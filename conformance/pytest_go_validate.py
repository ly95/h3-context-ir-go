"""Execute upstream's unchanged assertions with only validate() replaced by Go.

This is deliberately a mixed implementation run. Tests that never invoke the Go
validator are reported as not covered, even when their Python test passes.

CONTEXTIR_GO_VALIDATOR must name the bridge binary built from this checkout.
CONTEXTIR_CAPTURE_PATH must name a separate report; never overwrite the baseline.
"""
from __future__ import annotations
import atexit
import hashlib
import importlib
import json
import os
from pathlib import Path
import subprocess

import pytest_capture as cap

pytest_collection_modifyitems = cap.pytest_collection_modifyitems
pytest_runtest_logstart = cap.pytest_runtest_logstart
pytest_runtest_logfinish = cap.pytest_runtest_logfinish
pytest_runtest_logreport = cap.pytest_runtest_logreport
PROCESS = None
BINARY = None


def pytest_configure(config):
    global PROCESS, BINARY
    BINARY = Path(os.environ["CONTEXTIR_GO_VALIDATOR"]).resolve()
    if not BINARY.is_file():
        raise RuntimeError("CONTEXTIR_GO_VALIDATOR does not name a built Go bridge binary")
    PROCESS = subprocess.Popen([str(BINARY)], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, text=True, encoding="utf-8", bufsize=1)
    atexit.register(stop)
    vm = importlib.import_module("h3ir.validate")
    from h3ir.models import Finding

    def go_validate(text, ctx=None, **kw):
        context = cap.plain(ctx or vm.Context(**kw))
        request = {"text": text, "context": context}
        PROCESS.stdin.write(json.dumps(request, ensure_ascii=False) + "\n")
        PROCESS.stdin.flush()
        line = PROCESS.stdout.readline()
        if not line:
            raise RuntimeError("Go validator bridge stopped before responding")
        result = json.loads(line)
        if result.get("error"):
            raise RuntimeError(result["error"])
        findings = [Finding(**f) for f in result["findings"]]
        cap.STATE["validator_calls"].append({
            "id": f"go-validate-{len(cap.STATE['validator_calls']) + 1:05d}",
            "nodeid": cap.CURRENT, **request, "findings": cap.plain(findings)})
        return findings
    vm.validate = go_validate


def stop():
    global PROCESS
    if PROCESS is not None:
        if PROCESS.stdin:
            PROCESS.stdin.close()
        try:
            PROCESS.wait(timeout=5)
        except subprocess.TimeoutExpired:
            PROCESS.kill()
            PROCESS.wait()
        PROCESS = None


def pytest_sessionfinish(session, exitstatus):
    cap.pytest_sessionfinish(session, exitstatus)
    path = Path(os.environ["CONTEXTIR_CAPTURE_PATH"])
    report = json.loads(path.read_text(encoding="utf-8"))
    touched = {call["nodeid"] for call in report["validator_calls"]}
    counts = {}
    for item in report["collected"]:
        item["go_validator_touched"] = item["nodeid"] in touched
        # Passing a mixed test proves its unchanged assertions, not a Go port of
        # every Python collaborator the test exercised.
        item["go_status"] = "validator_assertions_" + item["upstream_status"] if item["go_validator_touched"] else "not_covered"
        if item["go_validator_touched"]:
            counts[item["upstream_status"]] = counts.get(item["upstream_status"], 0) + 1
    report["implementation_under_test"] = "Go h3check.validate; all other upstream code remains Python"
    report["source_commit"] = "fd031e136ae3d89147324c0d1b2aa65e838c21f5"
    report["go_validator_assertion_counts"] = counts
    report["go_validator_calls"] = len(report["validator_calls"])
    report["go_bridge_sha256"] = hashlib.sha256(BINARY.read_bytes()).hexdigest()
    report["go_not_covered_count"] = len(report["collected"]) - sum(counts.values())
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    stop()
