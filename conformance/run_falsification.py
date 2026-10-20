#!/usr/bin/env python3
"""Run upstream's own 17 mutation checks in a disposable byte-exact checkout.

Only harness interpreter path is injected; upstream sources and test assertions are
unchanged. Results describe Python's guards, never Go's mutation resistance.
"""
from __future__ import annotations
import argparse
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

PIN = "fd031e136ae3d89147324c0d1b2aa65e838c21f5"
ROOT = Path(__file__).resolve().parents[1]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    args = parser.parse_args()
    source = args.source.resolve()
    sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip()
    if sha != PIN or subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=no"], cwd=source, text=True):
        raise SystemExit("Expected clean pinned upstream checkout")
    tracked = subprocess.check_output(["git", "ls-files", "-z"], cwd=source).split(b"\0")
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="contextir-upstream-falsification-") as directory:
        copy = Path(directory)
        for encoded in tracked:
            if not encoded:
                continue
            relative = os.fsdecode(encoded)
            target = copy / relative
            if (source / relative).is_file():
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(source / relative, target)
        spec = importlib.util.spec_from_file_location("upstream_falsification", copy / "research/contract_falsification.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        # The upstream script assumes .venv/bin/python in its tree. Inject our
        # already isolated interpreter rather than modifying that script's bytes.
        module.PY = sys.executable
        os.environ["PYTHONPATH"] = str(copy)
        os.environ["H3IR_STATE_DIR"] = str(copy / "test-state")
        before = {path: hashlib.sha256((copy / path).read_bytes()).hexdigest() for path in module.TOUCHES}
        stream = io.StringIO()
        with contextlib.redirect_stdout(stream):
            exit_status = module.main()
        after = {path: hashlib.sha256((copy / path).read_bytes()).hexdigest() for path in module.TOUCHES}
        log = stream.getvalue()
        cases = []
        for name, _defect, nodes in module.CASES:
            outcome = next((status for status in ("RED", "GREEN", "BROKEN") if f"{status:<8}{name}" in log), "not_run")
            cases.append({"name": name, "tests": nodes.split(), "upstream_outcome": outcome, "go_status": "pending"})
        report = {"source_commit": sha, "implementation_under_test": "upstream Python (not Go)",
                  "exit_status": exit_status, "duration_s": round(time.monotonic() - started, 3),
                  "source_restored": before == after, "interpreter_override_only": True,
                  "cases": cases, "touched_source_sha256": before}
        (ROOT / "conformance/reports/upstream-falsification.txt").write_text(log, encoding="utf-8")
        (ROOT / "conformance/reports/upstream-falsification.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(log)
        if before != after:
            raise SystemExit("Upstream mutation harness failed to restore disposable source bytes")
        raise SystemExit(exit_status)

if __name__ == "__main__":
    main()
