"""Observe unchanged original creativity permission calls and their effects."""
import functools
import hashlib
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
    if subprocess.check_output(['git','rev-parse','HEAD'],cwd=source,text=True).strip()!=pin or subprocess.check_output(['git','status','--porcelain','--untracked-files=no'],cwd=source,text=True):raise RuntimeError('Scope observation requires the clean pinned source')
    module=importlib.import_module('h3ir.creativity')
    for owner,names,prefix in ((module,('build','parse'),'h3ir.creativity.'),(module.Scope,('permits','why_not','note','brief_instruction'),'h3ir.creativity.Scope.')):
        for name in names:
            original=getattr(owner,name);signature=inspect.signature(original)
            def wrap(original,signature,qualified):
                @functools.wraps(original)
                def invoke(*args,**kwargs):
                    bound=signature.bind(*args,**kwargs);bound.apply_defaults()
                    record={'id':f'scope-{len(CALLS)+1:05d}','nodeid':cap.CURRENT,'function':qualified,'arguments':cap.plain(dict(bound.arguments))}
                    CALLS.append(record)
                    try:result=original(*args,**kwargs)
                    except Exception as error:record['raised']=type(error).__module__+'.'+type(error).__name__;raise
                    record['result']=cap.plain(result)
                    if qualified=='h3ir.creativity.build':record['supplementary_properties']={'magnitude':result.magnitude,'licensed':cap.plain(result.licensed)}
                    return result
                return invoke
            setattr(owner,name,wrap(original,signature,prefix+name))

def pytest_sessionfinish(session,exitstatus):
    cap.pytest_sessionfinish(session,exitstatus)
    path=Path(os.environ['CONTEXTIR_CAPTURE_PATH']);report=json.loads(path.read_text());report['scope_calls']=CALLS
    report['source_commit']='fd031e136ae3d89147324c0d1b2aa65e838c21f5';report['implementation_under_test']='Original Python only; call observation does not prove native Go parity'
    path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    if exitstatus!=0 or len(report['collected'])!=991:return
    directory=path.parent.parent/'fixtures'
    (directory/'scope_calls.json').write_text(json.dumps({'source_commit':report['source_commit'],'calls':CALLS,'tests':report['collected']},ensure_ascii=False,indent=2)+'\n')
    inputs=[];expectations=[]
    for item in CALLS:
        if item['function']!='h3ir.creativity.build':continue
        canonical=json.dumps(item['arguments'],ensure_ascii=False,sort_keys=True,separators=(',',':')).encode()
        identity={'case_id':item['id'],'nodeid':item['nodeid'],'input_sha256':hashlib.sha256(canonical).hexdigest()}
        inputs.append({**identity,'input':item['arguments']})
        expectations.append({**identity,'expected':item.get('result'),'supplementary_properties':item.get('supplementary_properties'),'expected_exception':item.get('raised')})
    for name,cases,warning in (('scope_inputs.json',inputs,'Original permission-input contract only; no expected values. Presence flags must never be turned into invented dialogue or lettering.'),('scope_expectations.json',expectations,'Held-out original outputs; never send to a model.')):
        (directory/name).write_text(json.dumps({'source_commit':report['source_commit'],'warning':warning,'cases':cases},ensure_ascii=False,indent=2)+'\n')
