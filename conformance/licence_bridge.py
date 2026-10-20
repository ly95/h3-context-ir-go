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
import os

import pytest_capture as cap

ROOT=Path(__file__).resolve().parents[1]
MISSING_INPUTS=[]

def install(native_operation,plain):
    selected=os.environ.get('CONTEXTIR_LICENCE_RESOLUTIONS')
    pairs=[(None,selected)] if selected else [('licence_inputs.json','licence_resolutions.json')]
    if not selected and (ROOT/'conformance/fixtures/native_audio_licence_resolutions.json').is_file():
        pairs.append(('native_audio_licence_inputs.json','native_audio_licence_resolutions.json'))
    for path in ([] if selected else sorted((ROOT/'conformance/fixtures').glob('native_audio_licence_resolutions_round*.json'))):
        pairs.append((path.name.replace('_resolutions_','_inputs_'),path.name))
    for path in ([] if selected else sorted((ROOT/'conformance/fixtures').glob('licence_permission_resolutions_round*.json'))):
        pairs.append((path.name.replace('_resolutions_','_inputs_'),path.name))
    for path in ([] if selected else sorted((ROOT/'conformance/fixtures').glob('licence_environment_resolutions_round*.json'))):
        pairs.append((path.name.replace('_resolutions_','_inputs_'),path.name))
    def key(arguments,*,recorded_fixture=False):
        arguments=copy.deepcopy(arguments)
        for asset in arguments['brief']['assets']:
            path=asset.get('path')
            if path and (recorded_fixture or Path(path).resolve().is_relative_to(Path(tempfile.gettempdir()).resolve())) and any(p.startswith('pytest-') and p[7:].isdigit() for p in Path(path).parts):asset['path']='<original-test-temporary-file>'
        return json.dumps(arguments,ensure_ascii=False,sort_keys=True,separators=(',',':'))
    inputs={}
    report_provenance={}
    for input_file,result_file in pairs:
        index_path=Path(result_file) if selected else ROOT/'conformance/fixtures'/result_file
        if not index_path.is_absolute():index_path=ROOT/index_path
        actual=json.loads(index_path.read_text())
        if actual['source_commit']!='fd031e136ae3d89147324c0d1b2aa65e838c21f5':raise RuntimeError('Unpinned actual licence replay index')
        for recorded in actual.get('files',[]):
            path=ROOT/recorded['path']
            if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest()!=recorded['sha256']:raise RuntimeError('Recorded licence call bytes changed')
        corpus_path=ROOT/actual['input_fixture'] if actual.get('input_fixture') else ROOT/'conformance/fixtures'/input_file
        if not corpus_path.resolve().is_relative_to((ROOT/'conformance/fixtures').resolve()):raise RuntimeError('Actual licence input fixture must be inside the conformance fixture directory')
        if actual.get('input_fixture_sha256') and hashlib.sha256(corpus_path.read_bytes()).hexdigest()!=actual['input_fixture_sha256']:raise RuntimeError('Actual licence input fixture hash changed')
        corpus=json.loads(corpus_path.read_text())
        results={c['case_id']:c for c in actual['cases']}
        def provenance_for(result,actual=actual,index_path=index_path):
            report_path=ROOT/result.get('run_report',actual.get('archived_report',''))
            if report_path not in report_provenance:report_provenance[report_path]=(json.loads(report_path.read_text()),hashlib.sha256(report_path.read_bytes()).hexdigest())
            report,report_sha=report_provenance[report_path]
            if actual.get('executable_sha256') and report['executable_sha256']!=actual['executable_sha256']:raise RuntimeError('Licence recording binary differs from its explicit index')
            return {'actual_model_binary_sha256':report['executable_sha256'],'actual_model_report_sha256':report_sha,'actual_licence_index':str(index_path.relative_to(ROOT)),'actual_licence_index_sha256':hashlib.sha256(index_path.read_bytes()).hexdigest()}
        if selected:
            for recording in actual.get('recordings',[]):
                report_path=ROOT/recording['report']
                if hashlib.sha256(report_path.read_bytes()).hexdigest()!=recording['report_sha256']:raise RuntimeError('Actual licence report changed')
        for item in corpus['cases']:
            if item['case_id'] not in results:raise RuntimeError('Incomplete actual licence transcript corpus')
            inputs[key(item['input'],recorded_fixture=True)]=(item,results,provenance_for)
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
                item,results,provenance_for=match
                result=results[item['case_id']]
                if result['input_sha256']!=item['input_sha256']:raise RuntimeError('Original licence input hash differs')
                visited=set()
                while result.get('reused_from'):
                    if result['case_id'] in visited:raise RuntimeError('Actual licence reuse contains a cycle')
                    visited.add(result['case_id']);result=results[result['reused_from']]
                provenance=provenance_for(result)
                trace=ROOT/Path(result['run_report']).parent/result.get('trace_case_id',result['case_id']);responses=[]
                for i,call in enumerate(result.get('calls')or[],1):
                    if call['stage'] not in ('rendering_permission','medium','identity_requirements','licence'):raise RuntimeError('Unrelated model stage in licence transcript')
                    output_path=trace/f'call-{i:02d}-output.json'
                    candidate={'stage':call['stage'],'recorded_request':json.loads((trace/f'call-{i:02d}-request.json').read_text()),'response':{'json':json.loads(output_path.read_text()) if output_path.is_file() else None,'model':call['model'],'usage':call['usage'],'usage_invalid':not call['usage_valid']}}
                    if not output_path.is_file():
                        rejected=trace/f'call-{i:02d}-rejected.txt'
                        if not rejected.is_file() or not call.get('error','').startswith('contextir: invalid_model_output at deepseek.response ') or not call.get('diagnostics'):raise RuntimeError('Recorded output missing without an explicit provider rejection')
                        candidate['recorded_error']={'code':'invalid_model_output','stage':'deepseek.response','issues':call['diagnostics']}
                        candidate['rejected_output']=rejected.read_text()
                    responses.append(candidate)
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
