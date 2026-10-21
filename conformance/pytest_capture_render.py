"""Observe all original deterministic render/textnorm functions without replacing behavior."""
import functools
import importlib
import inspect
import json
import os
from pathlib import Path
import subprocess

import pytest_capture as cap

CALLS=[]
pytest_collection_modifyitems=cap.pytest_collection_modifyitems
pytest_runtest_logstart=cap.pytest_runtest_logstart
pytest_runtest_logfinish=cap.pytest_runtest_logfinish
pytest_runtest_logreport=cap.pytest_runtest_logreport

def pytest_configure(config):
    source=Path(str(config.rootpath));pin='fd031e136ae3d89147324c0d1b2aa65e838c21f5'
    if subprocess.check_output(['git','rev-parse','HEAD'],cwd=source,text=True).strip()!=pin or subprocess.check_output(['git','status','--porcelain','--untracked-files=no'],cwd=source,text=True):raise RuntimeError('Render observation requires clean pinned source')
    # textnorm first: render imports the wrapped functions; every original still executes.
    for module_name in ('h3ir.textnorm','h3ir.render'):
        module=importlib.import_module(module_name)
        for name,original in list(vars(module).items()):
            if not inspect.isfunction(original) or original.__module__!=module_name:continue
            signature=inspect.signature(original)
            def wrap(original,signature,qualified):
                @functools.wraps(original)
                def invoke(*args,**kwargs):
                    bound=signature.bind(*args,**kwargs);bound.apply_defaults()
                    record={'id':f'render-{len(CALLS)+1:05d}','nodeid':cap.CURRENT,'function':qualified,'arguments':cap.plain(dict(bound.arguments))};CALLS.append(record)
                    try:result=original(*args,**kwargs)
                    except Exception as error:record['raised']=type(error).__module__+'.'+type(error).__name__;raise
                    record['result']=cap.plain(result)
                    after=cap.plain(dict(bound.arguments))
                    if after!=record['arguments']:record['arguments_after']=after
                    return result
                return invoke
            setattr(module,name,wrap(original,signature,module_name+'.'+name))

def pytest_sessionfinish(session,exitstatus):
    cap.pytest_sessionfinish(session,exitstatus)
    path=Path(os.environ['CONTEXTIR_CAPTURE_PATH']);report=json.loads(path.read_text())
    report.update(source_commit='fd031e136ae3d89147324c0d1b2aa65e838c21f5',
        implementation_under_test='Original Python render/textnorm only; observation does not establish native Go parity',render_calls=CALLS)
    path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    if exitstatus!=0 or len(report['collected'])!=991:return
    destination=path.parent.parent/'fixtures/render_calls.json'
    if destination.exists():raise RuntimeError('Refusing to overwrite original render capture')
    destination.write_text(json.dumps({'source_commit':report['source_commit'],
        'provenance':'Actual calls and after-mutation states from unchanged original 991-test run.',
        'calls':CALLS},ensure_ascii=False,separators=(',',':'))+'\n')
