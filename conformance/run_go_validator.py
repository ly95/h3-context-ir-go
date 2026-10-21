#!/usr/bin/env python3
"""Run unmodified upstream pytest with native Go validate(), preserving each round."""
from __future__ import annotations
import argparse
import collections
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

PIN = "fd031e136ae3d89147324c0d1b2aa65e838c21f5"
ROOT = Path(__file__).resolve().parents[1]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--label", required=True, help="A new result label, e.g. round2")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_-]*", args.label):
        raise SystemExit("label must be a plain filename component")
    source = args.source.resolve()
    sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip()
    if sha != PIN or subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=no"], cwd=source, text=True):
        raise SystemExit("Expected clean pinned upstream checkout")
    report = ROOT / f"conformance/reports/go-validator-pytest-{args.label}.json"
    if report.exists():
        raise SystemExit("Refusing to overwrite a previous round; choose a fresh label")
    env = os.environ.copy()
    env["CONTEXTIR_GO_VALIDATOR"] = str(args.binary.resolve())
    env["CONTEXTIR_CAPTURE_PATH"] = str(report)
    env["PYTHONPATH"] = str(ROOT / "conformance") + os.pathsep + str(source)
    env["H3IR_STATE_DIR"] = "/tmp/contextir-upstream-go-bridge-state"
    command = [sys.executable, "-m", "pytest", "-q", "-p", "pytest_go_validate"]
    result = subprocess.run(command, cwd=source, env=env, capture_output=True, text=True)
    report.with_suffix(".txt").write_text(result.stdout + result.stderr, encoding="utf-8")
    if not report.exists():
        print(result.stdout + result.stderr)
        raise SystemExit("pytest did not produce a Go bridge report")
    data = json.loads(report.read_text(encoding="utf-8"))
    data["command"] = "python -m pytest -q -p pytest_go_validate"
    data["label"] = args.label
    report.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    shutil.copyfile(report, ROOT / "conformance/reports/go-validator-pytest.json")
    matrix_path = ROOT / "conformance/matrix.json"
    matrix = json.loads(matrix_path.read_text(encoding="utf-8"))
    by_node = {item["nodeid"]: item for item in data["collected"]}
    calls = collections.Counter(item["nodeid"] for item in data["validator_calls"])
    for test in matrix["tests"]:
        actual = by_node.get(test["nodeid"])
        test["go_validator"] = {
            "status": actual["go_status"] if actual else "not_run",
            "calls": calls[test["nodeid"]],
            "report": str(report.relative_to(ROOT)),
            "scope": "Only h3check.validate is Go; all other upstream modules remain Python",
        }
    matrix_path.write_text(json.dumps(matrix, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"pytest_counts": data["counts"],
                      "actual_go_validator_assertion_counts": data["go_validator_assertion_counts"],
                      "go_validator_calls": data["go_validator_calls"],
                      "not_covered": data["go_not_covered_count"]}))
    raise SystemExit(result.returncode)

if __name__ == "__main__":
    main()
