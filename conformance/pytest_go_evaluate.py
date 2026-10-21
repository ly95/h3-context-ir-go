"""Original full suite with only offline evaluation operations replaced by Go."""
import atexit
import json
import os
from pathlib import Path
import subprocess

import pytest_go_core as core
import pytest_go_validate as transport

pytest_collection_modifyitems = core.pytest_collection_modifyitems
pytest_runtest_logstart = core.pytest_runtest_logstart
pytest_runtest_logfinish = core.pytest_runtest_logfinish
pytest_runtest_logreport = core.pytest_runtest_logreport


def pytest_configure(config):
    # Start the JSON-lines transport without replacing the source validator or
    # any other source function. Only evaluate_bridge installs native helpers.
    transport.BINARY = Path(os.environ['CONTEXTIR_GO_VALIDATOR']).resolve()
    transport.PROCESS = subprocess.Popen([str(transport.BINARY)], stdin=subprocess.PIPE,
                                         stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                         text=True, encoding='utf-8', bufsize=1)
    atexit.register(transport.stop)
    import evaluate_bridge
    evaluate_bridge.install(core.native_operation)


def pytest_sessionfinish(session, exitstatus):
    core.pytest_sessionfinish(session, exitstatus)
    path = Path(os.environ['CONTEXTIR_CAPTURE_PATH'])
    report = json.loads(path.read_text())
    report['implementation_under_test'] = 'Native Go evaluate helpers only; original assertion bodies unchanged. All other source functions remain Python. No provider calls.'
    report['adapter_differences'] = []
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
