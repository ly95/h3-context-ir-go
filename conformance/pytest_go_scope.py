"""Original assertions with actual model scope tapes and production Go permissions."""
import pytest_go_core as core

pytest_collection_modifyitems=core.pytest_collection_modifyitems
pytest_runtest_logstart=core.pytest_runtest_logstart
pytest_runtest_logfinish=core.pytest_runtest_logfinish
pytest_runtest_logreport=core.pytest_runtest_logreport
pytest_sessionfinish=core.pytest_sessionfinish

def pytest_configure(config):
    core.configure(config,mode_replay=False)
    import scope_bridge
    scope_bridge.install(core.native_operation,core.cap.plain)
