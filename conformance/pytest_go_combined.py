"""One unchanged original run with real mode/scope/licence tapes through Go.

Supplied-content policy is explicit. The unchanged source renderer, visual
analysis collaborator and orchestration are still outside this native claim.
"""
import pytest_go_core as core

pytest_collection_modifyitems=core.pytest_collection_modifyitems
pytest_runtest_logstart=core.pytest_runtest_logstart
pytest_runtest_logfinish=core.pytest_runtest_logfinish
pytest_runtest_logreport=core.pytest_runtest_logreport

def pytest_sessionfinish(session,exitstatus):
    core.pytest_sessionfinish(session,exitstatus)
    import json
    import licence_bridge
    import os
    from pathlib import Path
    path=Path(os.environ['CONTEXTIR_CAPTURE_PATH'])
    data=json.loads(path.read_text())
    data['semantic_missing_actual_inputs']=licence_bridge.MISSING_INPUTS
    path.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')

def pytest_configure(config):
    core.configure(config)
    import scope_bridge
    scope_bridge.install(core.native_operation,core.cap.plain,conflict_policy='supplied_content_wins')
    import licence_bridge
    licence_bridge.install(core.native_operation,core.cap.plain)
