"""Replay actual model scope outputs through native Go; never read expectations."""
import functools
import importlib
import inspect
import json
from pathlib import Path
import sys

ROOT=Path(__file__).resolve().parents[1]
ELEMENT_FIELDS={'speech':'speech','music':'music','on-screen text':'onscreen_text'}

def install(native_operation,plain,*,conflict_policy=None):
    if conflict_policy not in (None,'supplied_content_wins'):raise ValueError('Unknown explicit source compatibility policy')
    inputs=json.loads((ROOT/'conformance/fixtures/scope_inputs.json').read_text())
    actual=json.loads((ROOT/'conformance/fixtures/scope_resolutions.json').read_text())
    canonical=lambda x:json.dumps(x,ensure_ascii=False,sort_keys=True,separators=(',',':'))
    cases={canonical(x['input']):x for x in inputs['cases']}
    results={x['case_id']:x for x in actual['cases']}
    module=importlib.import_module('h3ir.creativity')
    class NativeScope(module.Scope):
        @property
        def magnitude(self):return self._native_allowed['magnitude']
        @property
        def licensed(self):return frozenset(e for e,key in ELEMENT_FIELDS.items() if self._native_licensed[key])
        def permits(self,element):return bool(self._native_allowed.get(ELEMENT_FIELDS.get(element,''),False))
    original=module.build;signature=inspect.signature(original)
    @functools.wraps(original)
    def build(*args,**kwargs):
        bound=signature.bind(*args,**kwargs);bound.apply_defaults();arguments=plain(dict(bound.arguments))
        item=cases.get(canonical(arguments))
        if item is None:raise RuntimeError('No actual scope transcript exists for this exact original input')
        resolution=results[item['case_id']]
        if resolution['input_sha256']!=item['input_sha256']:raise RuntimeError('Original scope input hash differs')
        visited=set()
        while resolution.get('reused_from'):
            if resolution['case_id'] in visited:raise RuntimeError('Scope reuse contains a cycle')
            visited.add(resolution['case_id']);resolution=results[resolution['reused_from']]
        trace_dir=ROOT/Path(resolution['run_report']).parent/resolution['case_id'];responses=[]
        for i,call in enumerate(resolution.get('calls') or [],1):
            if call['stage']!='scope':raise RuntimeError('Unrelated model stage in scope transcript')
            responses.append({'stage':'scope','response':{'json':json.loads((trace_dir/f'call-{i:02d}-output.json').read_text()),
                'model':call['model'],'usage':call['usage'],'usage_invalid':not call['usage_valid']}})
        native_input=dict(arguments)
        if conflict_policy is not None:native_input['conflict_policy']=conflict_policy
        result=native_operation('h3ir.creativity.build',{'input':native_input,'responses':responses,
            'actual_case_id':item['case_id'],'actual_report':resolution['run_report'],
            'explicit_source_compatibility_policy':conflict_policy})
        d=result['decision'];value=NativeScope(level=module.Creativity(result['level']),
            requested=frozenset(e for e,key in ELEMENT_FIELDS.items() if d['requested'][key]),
            forbidden=frozenset(e for e,key in ELEMENT_FIELDS.items() if d['forbidden'][key]))
        object.__setattr__(value,'_native_allowed',d['allowed']);object.__setattr__(value,'_native_licensed',result['licensed'])
        return value
    for name,loaded in list(sys.modules.items()):
        if name.startswith('h3ir') and loaded is not None:
            for key,value in list(vars(loaded).items()):
                if value is original:setattr(loaded,key,build)
    module.build=build
