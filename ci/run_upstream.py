#!/usr/bin/env python3
"""CI gates for pinned original assertions and archived native replay, never live AI."""
import argparse
import hashlib
import importlib.metadata
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
PIN = "fd031e136ae3d89147324c0d1b2aa65e838c21f5"


def dump(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def clean_source(source):
    sha = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
    changed = subprocess.check_output(["git", "-C", str(source), "status", "--porcelain", "--untracked-files=all"], text=True)
    if sha != PIN or changed:
        raise RuntimeError("upstream must be a clean checkout at " + PIN)


def network_namespace():
    if sys.platform != "linux":
        raise RuntimeError("CI isolation requires a Linux network namespace")
    links = json.loads(subprocess.check_output(["ip", "-json", "link", "show"], text=True))
    if len(links) != 1 or links[0]["ifname"] != "lo" or "UP" not in links[0]["flags"]:
        raise RuntimeError("CI network namespace must contain only an enabled loopback interface")


def locked_packages():
    versions = {}
    for line in (ROOT / "conformance/requirements.lock").read_text().splitlines():
        if not line or line.startswith("#"):
            continue
        name, expected = line.split("==")
        actual = importlib.metadata.version(name)
        if actual != expected:
            raise RuntimeError(f"locked dependency differs: {name} expected {expected}, got {actual}")
        versions[name] = actual
    return versions


def child_environment(output, source):
    # Deliberately build an allowlist; no provider credentials, dotenv locations,
    # GitHub token, proxy settings or user plugin configuration are inherited.
    home, temporary = output / "home", output / "tmp"
    home.mkdir(); temporary.mkdir()
    return {"PATH": os.environ.get("PATH", os.defpath), "HOME": str(home),
            "TMPDIR": str(temporary), "LANG": "C.UTF-8", "LC_ALL": "C.UTF-8",
            "PYTHONPATH": os.pathsep.join(str(p) for p in (ROOT / "ci", ROOT / "conformance", source)),
            "PYTEST_DISABLE_PLUGIN_AUTOLOAD": "1", "PYTHONDONTWRITEBYTECODE": "1",
            "GOTOOLCHAIN": "local", "GOENV": "off", "GOPROXY": "off", "GOSUMDB": "off",
            "GOCACHE": str(output / "go-cache")}


def run_pytest(source, binary, output, env, name, plugin):
    report = output / (name + ".json")
    state = output / (name + "-state")
    child = {**env, "CONTEXTIR_CAPTURE_PATH": str(report),
             "CONTEXTIR_GO_VALIDATOR": str(binary), "H3IR_STATE_DIR": str(state)}
    command = [sys.executable, "-m", "pytest", "-q", "-p", "pytest_ci_network", "-p", plugin]
    result = subprocess.run(command, cwd=source, env=child, capture_output=True, text=True)
    (output / (name + ".txt")).write_text(result.stdout + result.stderr, encoding="utf-8")
    if not report.is_file():
        return {"returncode": result.returncode, "error": "pytest report missing; inspect " + name + ".txt"}, None
    data = json.loads(report.read_text())
    return {"returncode": result.returncode, "counts": data.get("counts"),
            "native_go_assertion_counts": data.get("native_go_assertion_counts"),
            "native_go_not_covered_count": data.get("native_go_not_covered_count")}, data


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--media-binary", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--allow-host-network", action="store_true",
                        help="local diagnostics only; CI always requires isolated Linux loopback")
    args = parser.parse_args()
    source, binary, media_binary, output = (p.resolve() for p in (args.source, args.binary, args.media_binary, args.output))
    if output.exists():
        raise RuntimeError("refusing to overwrite CI evidence; use a fresh --output directory")
    output.mkdir(parents=True)
    failures, summary = [], {"source_commit": PIN, "network": "isolated-loopback"}
    try:
        if args.allow_host_network:
            summary["network"] = "local diagnostic: Python loopback guard only"
        else:
            network_namespace()
        clean_source(source)
        if not binary.is_file() or not media_binary.is_file():
            raise RuntimeError("build both the native bridge and media test binary first")
        env = child_environment(output, source)
        tools = {}
        for tool in ("ffmpeg", "ffprobe"):
            if shutil.which(tool, path=env["PATH"]) is None:
                raise RuntimeError(tool + " is required; media tests may not silently skip")
            tools[tool] = subprocess.check_output([tool, "-version"], env=env, text=True).splitlines()[0]
        dump(output / "environment.json", {"python": platform.python_version(), "platform": platform.platform(),
             "packages": locked_packages(), "tools": tools, "source_commit": PIN,
             "go_bridge_sha256": hashlib.sha256(binary.read_bytes()).hexdigest()})
        media = subprocess.run(["go", "tool", "test2json", "-p", "github.com/ly95/contextir/media",
                                str(media_binary), "-test.v", "-test.count=1"], cwd=ROOT / "media", env=env, capture_output=True, text=True)
        (output / "go-media.jsonl").write_text(media.stdout, encoding="utf-8")
        (output / "go-media-stderr.txt").write_text(media.stderr, encoding="utf-8")
        events = [json.loads(line) for line in media.stdout.splitlines() if line]
        skipped = [e.get("Test", "package") for e in events if e.get("Action") == "skip"]
        passed = [e["Test"] for e in events if e.get("Action") == "pass" and e.get("Test")]
        summary["media"] = {"returncode": media.returncode, "passed_tests": len(passed), "skipped": skipped}
        if media.returncode or skipped or not passed:
            failures.append("native media execution failed, skipped tests, or ran no tests")
        expected = json.loads((ROOT / "conformance/reports/upstream-pytest.json").read_text())
        expected_nodes = {c["nodeid"] for c in expected["collected"]}
        if len(expected_nodes) != 991:
            failures.append("archived original collection is not the pinned 991-node baseline")
        reports = {}
        for name, plugin in (("upstream-baseline", "pytest_capture"), ("go-combined", "pytest_go_combined")):
            info, data = run_pytest(source, binary, output, env, name, plugin)
            summary[name] = info
            if data is None:
                failures.append(name + ": report missing")
                continue
            reports[name] = data
            nodes = {c["nodeid"] for c in data["collected"]}
            if info["returncode"] or data.get("counts") != {"passed": 991} or nodes != expected_nodes:
                failures.append(name + ": all 991 unchanged original assertions must pass with identical nodeids and zero skips")
            clean_source(source)
        combined = reports.get("go-combined")
        if combined:
            if combined.get("go_bridge_sha256") != hashlib.sha256(binary.read_bytes()).hexdigest():
                failures.append("native bridge identity changed during replay")
            actual_nodes = {c["nodeid"] for c in combined["collected"] if c.get("native_go_touched")}
            reference_pointer = ROOT / "conformance/reports/combined_latest.json"
            if not reference_pointer.is_file():
                failures.append("combined_latest.json coverage reference is missing")
            else:
                reference = (ROOT / json.loads(reference_pointer.read_text())["report"]).resolve()
                if not reference.is_relative_to(ROOT / "conformance/reports"):
                    raise RuntimeError("combined coverage reference must remain in conformance/reports")
                known = json.loads(reference.read_text())
                known_nodes = {c["nodeid"] for c in known["collected"] if c.get("native_go_touched")}
                if not known_nodes or not known_nodes <= actual_nodes:
                    failures.append("native assertion coverage shrank relative to the archived combined run")
            if not actual_nodes or not combined.get("go_validator_calls") or not combined.get("go_core_calls"):
                failures.append("combined run did not actually execute both native component and validator assertions")
            summary["native_go_touched"] = len(actual_nodes)
            summary["native_go_not_covered"] = len(expected_nodes - actual_nodes)
    except Exception as error:
        failures.append(str(error))
    summary["failures"] = failures
    summary["passed"] = not failures
    dump(output / "summary.json", summary)
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
