#!/usr/bin/env python3
"""Run the five Go-adapted behavior contracts separately from original pytest.

The pinned Python inputs/assertions and earlier failures are never modified.
Every report label is immutable. No live provider or dotenv is used.
"""

import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile


ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "conformance/go_adapted_cases.json"
PIN = "fd031e136ae3d89147324c0d1b2aa65e838c21f5"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def source_files():
    names = subprocess.check_output(
        ["git", "ls-files", "-c", "-o", "--exclude-standard", "-z"], cwd=ROOT
    ).split(b"\0")
    result = {}
    for raw in names:
        if not raw:
            continue
        relative = raw.decode("utf-8")
        if relative.startswith("conformance/reports/"):
            continue
        path = ROOT / relative
        if path.is_symlink():
            raise SystemExit("Source snapshots require regular files: " + relative)
        if path.is_file():
            result[relative] = digest(path)
    return dict(sorted(result.items()))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--label", required=True, help="New immutable report label")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_-]*", args.label):
        parser.error("label must be one plain filename component")
    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    if manifest["source_commit"] != PIN or len(manifest["cases"]) != 5:
        raise SystemExit("Expected the pinned five-case adaptation manifest")
    prefixes = [case["go_test_prefix"] for case in manifest["cases"]]
    if len(set(prefixes)) != 5:
        raise SystemExit("Each contract requires a distinct Go test prefix")
    output = ROOT / "conformance/reports" / ("go-adapted-" + args.label)
    report_path = output.with_suffix(".json")
    raw_path = output.with_suffix(".jsonl")
    archive_path = output.with_suffix(".sources.tar.gz")
    if any(p.exists() for p in (report_path, raw_path, archive_path)):
        raise SystemExit("Refusing to overwrite an earlier round")
    report_path.parent.mkdir(parents=True, exist_ok=True)
    hashes = source_files()
    with tarfile.open(archive_path, "x:gz") as archive:
        for relative in hashes:
            archive.add(ROOT / relative, arcname=relative, recursive=False)
    env = os.environ.copy()
    env["GOWORK"] = "off"
    # These tests use explicit in-process doubles and never need credentials.
    for name in ("DEEPSEEK_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"):
        env.pop(name, None)
    command = [
        "go", "test", "-json", "-count=1", "-timeout=180s",
        "-run=^TestGoConformance", "./compiler",
    ]
    started = datetime.datetime.now(datetime.timezone.utc).isoformat()
    process = subprocess.run(command, cwd=ROOT, env=env, capture_output=True, text=True)
    raw_path.write_text(process.stdout, encoding="utf-8")
    events = []
    parse_errors = []
    for number, line in enumerate(process.stdout.splitlines(), 1):
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            parse_errors.append(number)
    tests = {}
    for event in events:
        name = event.get("Test")
        action = event.get("Action")
        if name and action in ("run", "pass", "fail", "skip"):
            tests[name] = action
    roots = {name: action for name, action in tests.items() if "/" not in name}
    unmatched = [name for name in roots if not any(name.startswith(p) for p in prefixes)]
    results = []
    for case in manifest["cases"]:
        selected = {name: action for name, action in tests.items()
                    if name.split("/", 1)[0].startswith(case["go_test_prefix"])}
        passed = bool(selected) and all(action == "pass" for action in selected.values())
        results.append({**case, "status": "passed" if passed else "failed", "go_tests": selected})
    unchanged = hashes == source_files()
    passed = (process.returncode == 0 and not parse_errors and not unmatched and unchanged
              and all(row["status"] == "passed" for row in results))
    report = {
        "schema": "contextir-go-adapted/v1", "source_commit": PIN,
        "scope": "Five native Go behavior contracts with explicit interface/input adaptations. Separate from unchanged original pytest; no original failures are waived or added to its pass count.",
        "started_at": started,
        "finished_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "git_head": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
        "go_version": subprocess.check_output(["go", "version"], cwd=ROOT, text=True).strip(),
        "command": command, "exit_code": process.returncode, "stderr": process.stderr,
        "live_provider_calls": 0,
        "model_execution": "In-process test doubles and archived real responses; no remote provider adapter is configured by these tests.",
        "passed": passed,
        "counts": {
            "contract_cases": len(results),
            "contract_cases_passed": sum(row["status"] == "passed" for row in results),
            "root_tests": len(roots), "subtests": len(tests) - len(roots),
            "skipped": sum(action == "skip" for action in tests.values()),
            "failed_tests": sum(action == "fail" for action in tests.values()),
        },
        "cases": results, "unmatched_root_tests": unmatched,
        "unparsed_output_lines": parse_errors,
        "source_files_unchanged_during_run": unchanged,
        "source_sha256": hashes,
        "source_archive": str(archive_path.relative_to(ROOT)),
        "source_archive_sha256": digest(archive_path),
        "raw_go_events": str(raw_path.relative_to(ROOT)),
        "raw_go_events_sha256": digest(raw_path),
        "manifest_sha256": digest(MANIFEST),
        "original_upstream_run": manifest["original_upstream_run"],
    }
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"report": str(report_path.relative_to(ROOT)), "passed": passed,
                      "counts": report["counts"]}, ensure_ascii=False))
    return 0 if passed else 1


if __name__ == "__main__":
    sys.exit(main())
