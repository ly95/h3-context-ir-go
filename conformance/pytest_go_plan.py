"""Original assertions over native plan construction; semantic inputs are real tapes."""
import pytest_go_core as core
import pytest_capture as cap

pytest_collection_modifyitems=core.pytest_collection_modifyitems
pytest_runtest_logstart=core.pytest_runtest_logstart
pytest_runtest_logfinish=core.pytest_runtest_logfinish
pytest_runtest_logreport=core.pytest_runtest_logreport
pytest_sessionfinish=core.pytest_sessionfinish

def pytest_configure(config):
    core.configure(config,mode_replay=False)
