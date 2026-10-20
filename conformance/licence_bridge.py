"""Actual licence model tapes through the current public Go resolver, no oracle."""
import copy
import functools
import importlib
import inspect
import json
import hashlib
from pathlib import Path
import sys
import tempfile

import pytest_capture as cap

ROOT=Path(__file__).resolve().parents[1]
MISSING_INPUTS=[]

def install(native_operation,plain):
    pairs=[('licence_inputs.json','licence_resolutions.json')]
    if (ROOT/'conformance/fixtures/native_audio_licence_resolutions.json').is_file():
        pairs.append(('native_audio_licence_inputs.json','native_audio_licence_resolutions.json'))
    for path in sorted((ROOT/'conformance/fixtures').glob('native_audio_licence_resolutions_round*.json')):
        pairs.append((path.name.replace('_resolutions_','_inputs_'),path.name))
    for path in sorted((ROOT/'conformance/fixtures').glob('licence_permission_resolutions_round*.json')):
        pairs.append((path.name.replace('_resolutions_','_inputs_'),path.name))
    for path in sorted((ROOT/'conformance/fixtures').glob('licence_environment_resolutions_round*.json')):
        pairs.append((path.name.replace('_resolutions_','_inputs_'),path.name))
    def key(arguments,*,recorded_fixture=False):
        arguments=copy.deepcopy(arguments)
        for asset in arguments['brief']['assets']:
            path=asset.get('path')
            if path and (recorded_fixture or Path(path).resolve().is_relative_to(Path(tempfile.gettempdir()).resolve())) and any(p.startswith('pytest-') and p[7:].isdigit() for p in Path(path).parts):asset['path']='<original-test-temporary-file>'
        return json.dumps(arguments,ensure_ascii=False,sort_keys=True,separators=(',',':'))
    inputs={}
    for input_file,result_file in pairs:
        actual=json.loads((ROOT/'conformance/fixtures'/result_file).read_text())
        corpus_path=ROOT/actual['input_fixture'] if actual.get('input_fixture') else ROOT/'conformance/fixtures'/input_file
        if not corpus_path.resolve().is_relative_to((ROOT/'conformance/fixtures').resolve()):raise RuntimeError('Actual licence input fixture must be inside the conformance fixture directory')
        if actual.get('input_fixture_sha256') and hashlib.sha256(corpus_path.read_bytes()).hexdigest()!=actual['input_fixture_sha256']:raise RuntimeError('Actual licence input fixture hash changed')
        corpus=json.loads(corpus_path.read_text())
        results={c['case_id']:c for c in actual['cases']}
        report_path=ROOT/actual['archived_report'];report=json.loads(report_path.read_text())
        provenance={'actual_model_binary_sha256':report['executable_sha256'],'actual_model_report_sha256':hashlib.sha256(report_path.read_bytes()).hexdigest()}
        for item in corpus['cases']:
            if item['case_id'] not in results:raise RuntimeError('Incomplete actual licence transcript corpus')
            inputs[key(item['input'],recorded_fixture=True)]=(item,results,provenance)
    licence=importlib.import_module('h3ir.licence');style=importlib.import_module('h3ir.style')
    class NativeStyle(style.StyleDecision):
        @property
        def discrepancy(self):return self._native_discrepancy
        def note(self):return self._native_note or None
    for module,name in ((licence,'resolve_licence'),(style,'resolve_style')):
        original=getattr(module,name);signature=inspect.signature(original)
        def wrap(name,original,signature):
            @functools.wraps(original)
            def invoke(*args,**kwargs):
                bound=signature.bind(*args,**kwargs);bound.apply_defaults();arguments={k:plain(bound.arguments[k]) for k in ('brief','cards')}
                match=inputs.get(key(arguments))
                if match is None:
                    MISSING_INPUTS.append({'nodeid':cap.CURRENT,'function':'h3ir.'+('licence' if name=='resolve_licence' else 'style')+'.'+name,'arguments':copy.deepcopy(arguments)})
                    raise RuntimeError('No actual licence transcript exists for this exact original input')
                item,results,provenance=match
                result=results[item['case_id']]
                if result['input_sha256']!=item['input_sha256']:raise RuntimeError('Original licence input hash differs')
                visited=set()
                while result.get('reused_from'):
                    if result['case_id'] in visited:raise RuntimeError('Actual licence reuse contains a cycle')
                    visited.add(result['case_id']);result=results[result['reused_from']]
                trace=ROOT/Path(result['run_report']).parent/result['case_id'];responses=[]
                for i,call in enumerate(result.get('calls')or[],1):
                    if call['stage'] not in ('rendering_permission','medium','identity_requirements','licence'):raise RuntimeError('Unrelated model stage in licence transcript')
                    responses.append({'stage':call['stage'],'recorded_request':json.loads((trace/f'call-{i:02d}-request.json').read_text()),'response':{'json':json.loads((trace/f'call-{i:02d}-output.json').read_text()),'model':call['model'],'usage':call['usage'],'usage_invalid':not call['usage_valid']}})
                function='h3ir.'+('licence' if name=='resolve_licence' else 'style')+'.'+name
                output=native_operation(function,{**arguments,'responses':responses,'actual_case_id':item['case_id'],'actual_report':result['run_report'],**provenance})['decision']
                if name=='resolve_licence':return licence.Licence(governs=output['governs'],transform_phrase=output['transform_phrase'],reasons=output['reasons'])
                d=output['style'];value=NativeStyle(phrase=d['phrase'],source=d['source'],requested_medium=d['requested_medium']or None,observed_medium=d['reference_medium']or None,observed_phrase=d['observed_phrase'])
                value._native_discrepancy=d['discrepancy'];value._native_note=d['note'];return value
            return invoke
        replacement=wrap(name,original,signature)
        for n,loaded in list(sys.modules.items()):
            if n.startswith('h3ir') and loaded is not None:
                for k,v in list(vars(loaded).items()):
                    if v is original:setattr(loaded,k,replacement)
        setattr(module,name,replacement)
