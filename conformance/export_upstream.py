#!/usr/bin/env python3
"""Export pinned upstream controls/suite and run every upstream pytest unchanged.

Run with the isolated Python containing the upstream editable package and pytest:
 /tmp/contextir-upstream-venv/bin/python conformance/export_upstream.py --source /tmp/open-h3-ir-review-fd031e13
No model calls are made. The pytest suite stubs provider calls itself.
"""
from __future__ import annotations
import argparse
import dataclasses
import hashlib
import importlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time

PIN = "fd031e136ae3d89147324c0d1b2aa65e838c21f5"
ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "conformance"))
from pytest_capture import plain


def dump(relative, value):
    path = ROOT / relative
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    temporary.replace(path)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--skip-pytest", action="store_true", help="Export static fixtures only; no pass claim")
    args = parser.parse_args()
    source = args.source.resolve()
    sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip()
    if sha != PIN:
        raise SystemExit(f"Refusing unpinned source: expected {PIN}, got {sha}")
    tracked_changes = subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=no"], cwd=source, text=True)
    if tracked_changes:
        raise SystemExit("Refusing modified upstream tracked files; no tests may be weakened")
    sys.path.insert(0, str(source))
    import h3ir
    if Path(h3ir.__file__).resolve().parent.parent != source:
        raise SystemExit("Installed h3ir differs from --source")
    from h3ir.evalloop import controls, suite
    vm = importlib.import_module("h3ir.validate")
    original = vm.validate
    calls = []
    def observe(text, ctx=None, **kw):
        findings = original(text, ctx, **kw)
        calls.append({"text": text, "context": plain(ctx or vm.Context(**kw)), "findings": plain(findings)})
        return findings
    controls.validate = observe
    ok, results = controls.run_controls()
    controls.validate = original
    # The official example generates two distinct assertions over the same validator call.
    if len(calls) != 22 or len(results) != 23:
        raise SystemExit(f"Pinned control shape drift: {len(calls)} calls, {len(results)} controls")
    case_calls = [calls[0], calls[0], *calls[1:]]
    exported = []
    for index, (result, call) in enumerate(zip(results, case_calls), 1):
        exported.append({"id": f"control-{index:02d}", "name": result.name,
                         "upstream_passed": result.passed, "upstream_detail": result.detail,
                         "go_status": "pending",
                         "expectation": (
                             {"allowed_error_rules": ["P5-camera-no-motion-type"], "allowed_warn_rules": ["P5-camera-no-motion-type"]} if index == 1 else
                             {"required_error_rules": ["P5-camera-no-motion-type"]} if index == 2 else
                             {"required_error_rules": [controls.MUTANTS[index - 3][2]]} if 3 <= index <= 18 else
                             {"allowed_error_rules": []}), **call})
    dump("conformance/fixtures/controls.json", {"source_commit": sha, "upstream_passed": ok, "cases": exported})
    briefs = []
    for name, brief in suite.suite_briefs().items():
        raw = plain(brief)
        for asset in raw["assets"]:
            # Path relocation only; exact original asset bytes/sha are recorded below.
            asset["path"] = "third_party/open-h3-ir/testdata/golden/assets/" + Path(asset["path"]).name
        briefs.append({"id": name, "brief": raw, "go_status": "pending", "live_status": "not_run"})
    dump("conformance/fixtures/suite_briefs.json", {"source_commit": sha, "cases": briefs})
    inventory = []
    copied = []
    tracked = subprocess.check_output(["git", "ls-files", "-z"], cwd=source).split(b"\0")
    for name_bytes in tracked:
        if not name_bytes:
            continue
        relative = os.fsdecode(name_bytes)
        path = source / relative
        if not path.is_file():
            continue
        entry = {"path": relative, "sha256": digest(path), "bytes": path.stat().st_size,
                 "source_url": f"https://github.com/ruashots/open-h3-ir/blob/{sha}/{relative}"}
        inventory.append(entry)
        if relative.startswith("h3ir/golden/"):
            target = ROOT / "third_party/open-h3-ir/testdata/golden" / path.relative_to(source / "h3ir/golden")
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(path, target)
            copied.append({**entry, "local_path": str(target.relative_to(ROOT))})
    dump("conformance/source_manifest.json", {"repository": "https://github.com/ruashots/open-h3-ir",
        "commit": sha, "license": "Apache-2.0", "files": inventory, "copied_test_assets": copied})
    # Regeneration metadata intentionally contains no environment values or credentials.
    if not args.skip_pytest:
        capture = ROOT / f"conformance/reports/upstream-capture-candidate-{time.time_ns()}.json"
        env = os.environ.copy()
        env["PYTHONPATH"] = str(ROOT / "conformance") + os.pathsep + str(source)
        env["CONTEXTIR_CAPTURE_PATH"] = str(capture)
        # Keep caches generated by upstream tests outside the user's project.
        env["H3IR_STATE_DIR"] = "/tmp/contextir-upstream-test-state"
        command = [sys.executable, "-m", "pytest", "-q", "-p", "pytest_capture"]
        result = subprocess.run(command, cwd=source, env=env, capture_output=True, text=True)
        capture.with_suffix(".txt").write_text(result.stdout + result.stderr, encoding="utf-8")
        if not capture.exists():
            print(result.stdout + result.stderr)
            raise SystemExit("pytest did not produce a report")
        report = json.loads(capture.read_text(encoding="utf-8"))
        report["source_commit"] = sha
        report["command"] = "python -m pytest -q -p pytest_capture"
        # Never promote a failing/incomplete capture into canonical source fixtures.
        # The raw candidate and stdout remain available to diagnose the rejected run.
        expected = {"validator_calls": 386, "compiler_calls": 1952, "budget_calls": 4900,
                    "lora_calls": 108}
        actual = {key: len(report.get(key, [])) for key in expected}
        if result.returncode or report.get("counts") != {"passed": 991} or actual != expected:
            print(json.dumps({"rejected_capture": str(capture.relative_to(ROOT)),
                              "counts": report.get("counts"), "actual": actual,
                              "expected": expected}))
            raise SystemExit("Rejected capture: original suite or capture coverage changed; canonical fixtures preserved")
        validator_calls = report.pop("validator_calls")
        compiler_calls = report.pop("compiler_calls")
        budget_calls = report.pop("budget_calls")
        shots_calls = report.pop("shots_calls")
        lora_calls = report.pop("lora_calls")
        comfy_calls = report.pop("comfy_calls")
        dump("conformance/fixtures/validator_calls.json", {"source_commit": sha,
            "implementation_under_test": "upstream Python (not Go)", "calls": validator_calls})
        dump("conformance/fixtures/compiler_calls.json", {"source_commit": sha,
            "implementation_under_test": "upstream Python (not Go)", "calls": compiler_calls})
        dump("conformance/fixtures/budget_calls.json", {"source_commit": sha,
            "implementation_under_test": "upstream Python (not Go)", "calls": budget_calls})
        dump("conformance/fixtures/shots_calls.json", {"source_commit": sha,
            "implementation_under_test": "upstream Python (not Go)", "calls": shots_calls})
        dump("conformance/fixtures/lora_calls.json", {"source_commit": sha,
            "implementation_under_test": "upstream Python (not Go)", "calls": lora_calls})
        dump("conformance/fixtures/comfy_calls.json", {"source_commit": sha,
            "implementation_under_test": "upstream Python (not Go)", "calls": comfy_calls})
        dump("conformance/reports/upstream-pytest.json", report)
        dump("conformance/matrix.json", {"source_commit": sha, "tests": report["collected"],
             "go_counted_as_passed": 0,
             "note": "Python baseline results only. Every Go implementation status starts pending and needs an actual Go run."})
        shutil.copyfile(capture.with_suffix(".txt"), ROOT / "conformance/reports/upstream-pytest.txt")
        capture.unlink()
        capture.with_suffix(".txt").unlink()
        print(json.dumps({"upstream_counts": report["counts"], "validator_calls": len(validator_calls),
                          "compiler_calls": len(compiler_calls), "budget_calls": len(budget_calls), "pytest_exit_status": result.returncode}))
        if result.returncode:
            raise SystemExit(result.returncode)
    print(f"Exported {len(exported)} controls, {len(briefs)} suite briefs, {len(copied)} byte-exact golden files from {sha}")

if __name__ == "__main__":
    main()
