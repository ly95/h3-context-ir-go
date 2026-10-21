"""Observe original licence/style calls without replacing any implementation."""
import functools
import hashlib
import importlib
import inspect
import json
import os
import subprocess
from pathlib import Path

import pytest_capture as cap

pytest_collection_modifyitems = cap.pytest_collection_modifyitems
pytest_runtest_logstart = cap.pytest_runtest_logstart
pytest_runtest_logfinish = cap.pytest_runtest_logfinish
pytest_runtest_logreport = cap.pytest_runtest_logreport
CALLS = []


def pytest_configure(config):
    source=Path(str(config.rootpath))
    pin='fd031e136ae3d89147324c0d1b2aa65e838c21f5'
    if subprocess.check_output(['git','rev-parse','HEAD'],cwd=source,text=True).strip()!=pin or subprocess.check_output(['git','status','--porcelain','--untracked-files=no'],cwd=source,text=True):
        raise RuntimeError('Licence capture requires the clean pinned original checkout')
    # Install before plan/compile import aliases. Every wrapped function still
    # executes the original source and every assertion remains untouched.
    for module_name, names in {
        'h3ir.licence': ('resolve_licence', 'transformation_intent', 'transform_target'),
        'h3ir.style': ('resolve_style', 'observed_style', 'classify_medium', 'style_terms_in', '_tidy'),
    }.items():
        module = importlib.import_module(module_name)
        for name in names:
            original = getattr(module, name)
            signature = inspect.signature(original)
            def wrap(original, signature, qualified):
                @functools.wraps(original)
                def invoke(*args, **kwargs):
                    bound = signature.bind(*args, **kwargs)
                    bound.apply_defaults()
                    item = {'id': f'licence-{len(CALLS)+1:05d}', 'nodeid': cap.CURRENT,
                            'function': qualified, 'arguments': cap.plain(dict(bound.arguments))}
                    CALLS.append(item)
                    try:
                        result = original(*args, **kwargs)
                    except Exception as error:
                        item['raised'] = type(error).__module__ + '.' + type(error).__name__
                        raise
                    item['result'] = cap.plain(result)
                    if qualified == 'h3ir.licence.resolve_licence':
                        item['properties'] = {'medium_transferred': result.medium_transferred, 'note': result.note()}
                    if qualified == 'h3ir.style.resolve_style':
                        item['properties'] = {'discrepancy': result.discrepancy, 'note': result.note()}
                    return result
                return invoke
            setattr(module, name, wrap(original, signature, module_name + '.' + name))


def pytest_sessionfinish(session, exitstatus):
    cap.pytest_sessionfinish(session, exitstatus)
    path = Path(os.environ['CONTEXTIR_CAPTURE_PATH'])
    report = json.loads(path.read_text(encoding='utf-8'))
    report['licence_calls'] = CALLS
    report['source_commit'] = 'fd031e136ae3d89147324c0d1b2aa65e838c21f5'
    report['implementation_under_test'] = 'Original Python only; observation does not prove native Go parity'
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    if exitstatus == 0 and len(report['collected']) == 991:
        fixture = path.parent.parent / 'fixtures' / 'licence_cases.json'
        fixture.write_text(json.dumps({'source_commit': report['source_commit'],
            'provenance': 'Actual calls from unmodified original pytest, observed without changing results',
            'calls': CALLS, 'tests': report['collected']}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        export_semantic_files(fixture.parent, report['source_commit'], CALLS)


def export_semantic_files(directory, commit, calls):
    inputs=[];expectations=[]
    for item in calls:
        if item['function'] not in ('h3ir.licence.resolve_licence','h3ir.style.resolve_style'):
            continue
        # resolve_style may receive a Python-computed licence as an argument;
        # that is an oracle result too. Semantic model inputs use Brief/cards
        # only and must independently decide precedence before rendering style.
        arguments={k:v for k,v in item['arguments'].items() if k in ('brief','cards')}
        canonical=json.dumps(arguments,ensure_ascii=False,sort_keys=True,separators=(',',':')).encode('utf-8')
        identity={'case_id':item['id'],'nodeid':item['nodeid'],'function':item['function'],
            'input_sha256':hashlib.sha256(canonical).hexdigest()}
        inputs.append({**identity,'input':arguments})
        expectations.append({**identity,'expected':item.get('result'),'properties':item.get('properties'),
            'expected_exception':item.get('raised')})
    for name,cases,warning in (
        ('licence_inputs.json',inputs,'Only Brief/cards inputs; no computed licence or oracle output. Safe for model input.'),
        ('licence_expectations.json',expectations,'Held-out original outputs. Never send to a model.'),
    ):
        (directory/name).write_text(json.dumps({'source_commit':commit,'warning':warning,'cases':cases},ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
