"""Original assertions replay actual recorded native Go licence/style decisions.

Other functions remain original Python. This compares the earlier native run's
outputs, not the current Go binary, and never reads an expected value to answer.
"""
import copy
import functools
import importlib
import inspect
import json
import os
from pathlib import Path
import sys
import tempfile

import pytest_capture as cap

ROOT=Path(__file__).resolve().parents[1]
CALLS=[]
pytest_collection_modifyitems=cap.pytest_collection_modifyitems
pytest_runtest_logstart=cap.pytest_runtest_logstart
pytest_runtest_logfinish=cap.pytest_runtest_logfinish
pytest_runtest_logreport=cap.pytest_runtest_logreport

def pytest_configure(config):
    corpus=json.loads((ROOT/'conformance/fixtures/licence_inputs.json').read_text())
    actual=json.loads((ROOT/'conformance/fixtures/licence_resolutions.json').read_text())
    results={c['input_sha256']:c for c in actual['cases']}
    def key(arguments,*,recorded_fixture=False):
        arguments=copy.deepcopy(arguments)
        for asset in arguments['brief']['assets']:
            path=asset.get('path')
            if path and (recorded_fixture or Path(path).resolve().is_relative_to(Path(tempfile.gettempdir()).resolve())) and any(p.startswith('pytest-') and p[7:].isdigit() for p in Path(path).parts):asset['path']='<original-test-temporary-file>'
        return json.dumps(arguments,ensure_ascii=False,sort_keys=True,separators=(',',':'))
    inputs={key(x['input'],recorded_fixture=True):x for x in corpus['cases']}
    licence=importlib.import_module('h3ir.licence');style=importlib.import_module('h3ir.style')
    class RecordedStyle(style.StyleDecision):
        @property
        def discrepancy(self):return self._recorded_discrepancy
        def note(self):return self._recorded_note or None
    for module,name in ((licence,'resolve_licence'),(style,'resolve_style')):
        original=getattr(module,name);signature=inspect.signature(original)
        def wrap(name,original,signature):
            @functools.wraps(original)
            def invoke(*args,**kwargs):
                bound=signature.bind(*args,**kwargs);bound.apply_defaults()
                arguments={k:cap.plain(bound.arguments[k]) for k in ('brief','cards')}
                item=inputs.get(key(arguments))
                record={'nodeid':cap.CURRENT,'function':'h3ir.'+('licence' if name=='resolve_licence' else 'style')+'.'+name,'arguments':arguments}
                CALLS.append(record)
                if item is None:record['status']='no_original_input';raise RuntimeError('No actual native licence run exists for this exact original Brief/cards input')
                result=results[item['input_sha256']]
                record.update(actual_case_id=result['case_id'],input_sha256=item['input_sha256'],run_report=result['run_report'],status=result['status'])
                if result['status']!='licence_decided':record['error']=result.get('error');raise RuntimeError('Actual native licence decision failed: '+str(result.get('error')))
                decision=result['licence_decision']
                if name=='resolve_licence':
                    value=licence.Licence(governs=decision['governs'],transform_phrase=decision['transform_phrase'],reasons=decision['reasons'])
                else:
                    d=decision['style'];value=RecordedStyle(phrase=d['phrase'],source=d['source'],requested_medium=d['requested_medium'] or None,
                        observed_medium=d['reference_medium'] or None,observed_phrase=d.get('observed_phrase',''))
                    value._recorded_discrepancy=d['discrepancy'];value._recorded_note=d['note']
                record['result']=cap.plain(value)
                return value
            return invoke
        wrapper=wrap(name,original,signature)
        for n,loaded in list(sys.modules.items()):
            if n.startswith('h3ir') and loaded is not None:
                for k,v in list(vars(loaded).items()):
                    if v is original:setattr(loaded,k,wrapper)
        setattr(module,name,wrapper)

def pytest_sessionfinish(session,exitstatus):
    cap.pytest_sessionfinish(session,exitstatus)
    path=Path(os.environ['CONTEXTIR_CAPTURE_PATH']);report=json.loads(path.read_text())
    touched={x['nodeid'] for x in CALLS};counts={}
    for item in report['collected']:
        item['recorded_native_licence_result_touched']=item['nodeid'] in touched
        if item['nodeid'] in touched:counts[item['upstream_status']]=counts.get(item['upstream_status'],0)+1
    report.update(implementation_under_test='Actual archived native Go DecideLicence results converted to original source DTOs; all other code is original Python. This is recorded-result original-assert replay, not a current-Go-binary run.',
        licence_result_calls=CALLS,recorded_native_licence_assertion_counts=counts,not_touched=len(report['collected'])-sum(counts.values()),
        adapter_differences=['Original StyleDecision.observed_phrase is empty when absent from native recorded result; no source observation algorithm fills the missing native field. Native discrepancy/note values are preserved through a DTO subclass. The source Licence.note formatter remains Python.'])
    path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
