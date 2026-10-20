"""Replay actual paid model outputs through production Go, then original asserts.

This reads model-safe original inputs and actual response transcripts only.
Held-out expectations are intentionally not opened by this adapter.
"""
import functools
import copy
import hashlib
import importlib
import inspect
import json
from pathlib import Path
import sys
import tempfile

ROOT=Path(__file__).resolve().parents[1]


def install(native_operation,plain):
    inputs=json.loads((ROOT/'conformance/fixtures/semantic_cases.json').read_text())
    actual=json.loads((ROOT/'conformance/fixtures/semantic_resolutions.json').read_text())
    resolutions={c['case_id']:c for c in actual['cases']}
    cases={}
    canonical=lambda value:json.dumps(value,ensure_ascii=False,sort_keys=True,separators=(',',':'))
    for item in inputs['cases']:
        if item['input']['kind']!='mode':continue
        key=canonical({'brief':item['original_arguments']['brief'],'cards':item['original_arguments'].get('cards')})
        cases[key]=item
    module=importlib.import_module('h3ir.mode')
    from h3ir.models import Mode,ModeDecision
    original=module.infer_mode;signature=inspect.signature(original)
    @functools.wraps(original)
    def infer(*args,**kwargs):
        bound=signature.bind(*args,**kwargs);bound.apply_defaults();arguments=bound.arguments
        key=canonical({'brief':plain(arguments['brief']),'cards':plain(arguments.get('cards'))})
        item=cases.get(key)
        relocations=[]
        if item is None:
            current={'brief':plain(arguments['brief']),'cards':plain(arguments.get('cards'))}
            def without_temporary_paths(value,*,recorded_fixture=False):
                result=copy.deepcopy(value)
                for asset in result['brief']['assets']:
                    path=asset.get('path')
                    # Recorded pinned-fixture paths may come from another OS or
                    # TMPDIR. Current requests must still be inside this run's
                    # actual temp root; only pytest-owned locators can relocate.
                    if path and (recorded_fixture or Path(path).resolve().is_relative_to(Path(tempfile.gettempdir()).resolve())) and any(part.startswith('pytest-') and part[7:].isdigit() for part in Path(path).parts):asset['path']='<original-test-temporary-file>'
                return result
            same=[]
            for candidate in cases.values():
                original_args={'brief':candidate['original_arguments']['brief'],'cards':candidate['original_arguments'].get('cards')}
                if canonical(without_temporary_paths(original_args,recorded_fixture=True))==canonical(without_temporary_paths(current)):same.append(candidate)
            if len(same)==1:
                item=same[0]
                for old,new in zip(item['original_arguments']['brief']['assets'],current['brief']['assets']):
                    if old.get('path')!=new.get('path'):
                        path=Path(new['path'])
                        if not path.is_file():raise RuntimeError('Relocated original test media is unavailable')
                        relocations.append({'original':old['path'],'current':new['path'],'current_file_sha256':hashlib.sha256(path.read_bytes()).hexdigest(),
                            'scope':'Only the original pytest temporary locator changes; all remaining Brief/cards values match exactly.'})
        if item is None:raise RuntimeError('No actual live mode transcript exists for this exact original input')
        resolution=resolutions[item['case_id']]
        if resolution['input_sha256']!=item['input_sha256']:raise RuntimeError('Actual mode transcript input hash differs')
        visited=set()
        while resolution.get('reused_from'):
            if resolution['case_id'] in visited:raise RuntimeError('Recorded mode reuse contains a cycle')
            visited.add(resolution['case_id']);resolution=resolutions[resolution['reused_from']]
        responses=[]
        trace_dir=ROOT/Path(resolution['run_report']).parent/resolution['case_id']
        for index,call in enumerate(resolution.get('calls')or[],1):
            if call['stage'] not in ('mode','mode_conflict'):raise RuntimeError('An unrelated model stage cannot be supplied to the mode replay')
            path=trace_dir/f'call-{index:02d}-output.json'
            responses.append({'stage':call['stage'],'response':{'json':json.loads(path.read_text()),'model':call['model'],
                'usage':call['usage'],'usage_invalid':not call['usage_valid']}})
        result=native_operation('h3ir.mode.infer_mode',{'brief':arguments['brief'],'cards':arguments.get('cards'),
            'responses':responses,'actual_case_id':item['case_id'],'actual_report':resolution['run_report'],'fixture_path_relocations':relocations})
        decision=result['decision']
        value=ModeDecision(mode=Mode(decision['mode']),confidence=decision['confidence'],
            rule_fired=decision['rule'],signals=decision['signals'],alternatives=decision.get('alternatives',[]))
        questions=decision.get('questions') or []
        if len(questions)>1:raise RuntimeError('The original single-question DTO cannot represent multiple native questions')
        value._native_clarification=questions[0] if questions else None
        return value
    for n,loaded in list(sys.modules.items()):
        if n.startswith('h3ir') and loaded is not None:
            for key,value in list(vars(loaded).items()):
                if value is original:setattr(loaded,key,infer)
    module.infer_mode=infer
    original_clarification=module.needs_clarification
    @functools.wraps(original_clarification)
    def clarification(decision):
        if hasattr(decision,'_native_clarification'):return decision._native_clarification
        # This source helper remains Python for manually constructed source-only
        # decisions. Those paths do not acquire native mode coverage from here.
        return original_clarification(decision)
    for n,loaded in list(sys.modules.items()):
        if n.startswith('h3ir') and loaded is not None:
            for key,value in list(vars(loaded).items()):
                if value is original_clarification:setattr(loaded,key,clarification)
    module.needs_clarification=clarification
