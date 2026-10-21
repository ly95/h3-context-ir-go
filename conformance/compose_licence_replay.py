#!/usr/bin/env python3
"""Compose explicitly ordered fresh licence recordings, preserving failed history.

Inputs are raw Brief/cards; no oracle is consulted. Reuse IDs are namespaced by
run, so a retry cannot accidentally refer to a case in a different recording.
Only an explicitly later run can replace the same exact source input key.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
PIN='fd031e136ae3d89147324c0d1b2aa65e838c21f5'

def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def relative(p):return str(p.resolve().relative_to(ROOT))
def source_key(value):
    value=copy.deepcopy(value)
    for asset in value['brief']['assets']:
        path=asset.get('path')
        if path and any(p.startswith('pytest-') and p[7:].isdigit() for p in Path(path).parts):asset['path']='<original-test-temporary-file>'
    return json.dumps(value,ensure_ascii=False,sort_keys=True,separators=(',',':'))

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--run',nargs=3,action='append',required=True,metavar=('NAME','INPUT_FIXTURE','REPORT'))
    p.add_argument('--inputs',type=Path,required=True)
    p.add_argument('--resolutions',type=Path,required=True)
    a=p.parse_args()
    if a.inputs.exists() or a.resolutions.exists():raise SystemExit('Refusing to overwrite an existing replay corpus')
    names=set();inputs=[];results=[];runs=[];selected={};by_id={};files=[];binary=None
    for name,input_name,report_name in a.run:
        if name in names or not name or ':' in name:raise SystemExit('Run names must be distinct plain namespaces')
        names.add(name);ip=Path(input_name).resolve();rp=Path(report_name).resolve();raw=json.loads(ip.read_text());report=json.loads(rp.read_text())
        if raw['source_commit']!=PIN or report['Source']!=PIN or report['Kind']!='licence':raise SystemExit('Unpinned or wrong-kind licence recording')
        if binary is None:binary=report['executable_sha256']
        if binary!=report['executable_sha256']:raise SystemExit('Fresh recordings use different native binaries')
        actual={x['case_id']:x for x in report['results']};expected={x['case_id']:x for x in raw['cases']}
        if len(actual)!=len(report['results']) or set(actual)!=set(expected):raise SystemExit('Recording input collection differs from its complete input fixture')
        failures=[]
        for original_id,item in expected.items():
            result=copy.deepcopy(actual[original_id])
            if result['input_sha256']!=item['input_sha256']:raise SystemExit('Recorded input hash differs: '+name+':'+original_id)
            qualified=name+':'+original_id;converted=copy.deepcopy(item);converted['case_id']=qualified;converted['source_case_id']=original_id;converted['source_run']=name
            result['case_id']=qualified;result['source_case_id']=original_id;result['trace_case_id']=original_id;result['run_report']=relative(rp)
            if result.get('reused_from'):result['reused_from']=name+':'+result['reused_from']
            inputs.append(converted);results.append(result);by_id[qualified]=result;selected[source_key(item['input'])]=qualified
            if result.get('status')!='licence_decided':failures.append(original_id)
        for path in sorted(rp.parent.glob('*/call-*')):
            files.append({'path':relative(path),'sha256':digest(path)})
        runs.append({'name':name,'input_fixture':relative(ip),'input_fixture_sha256':digest(ip),'report':relative(rp),'report_sha256':digest(rp),'executable_sha256':report['executable_sha256'],'entries':len(expected),'failed_entries_preserved':failures})
    def canonical(id):
        seen=set()
        while by_id[id].get('reused_from'):
            if id in seen:raise SystemExit('Reuse cycle in '+id)
            seen.add(id);id=by_id[id]['reused_from']
            if id not in by_id:raise SystemExit('Reuse leaves its recording namespace')
        return by_id[id]
    for id in selected.values():
        c=canonical(id)
        if c['status']!='licence_decided':raise SystemExit('Selected final recording is not a successful resolution: '+id)
        trace=ROOT/Path(c['run_report']).parent/c['trace_case_id']
        for i,call in enumerate(c.get('calls') or [],1):
            if call['stage']not in ('rendering_permission','medium','identity_requirements','licence'):raise SystemExit('Unexpected recorded licence stage')
            if not (trace/f'call-{i:02d}-request.json').is_file():raise SystemExit('Missing actual recorded request')
            if not (trace/f'call-{i:02d}-output.json').is_file():
                if not (trace/f'call-{i:02d}-rejected.txt').is_file() or not call.get('error','').startswith('contextir: invalid_model_output at deepseek.response ') or not call.get('diagnostics'):raise SystemExit('Missing actual successful output or explicit provider rejection: '+str(trace))
    fixture={'source_commit':PIN,'warning':'Exact original Brief/cards only; ordered replay selection is explicit. No expected decisions or source oracle is input to a model. All earlier failures remain preserved in raw run reports and the result index.','cases':inputs,'recordings':runs}
    a.inputs.write_text(json.dumps(fixture,ensure_ascii=False,indent=2)+'\n')
    index={'source_commit':PIN,'schema':'contextir-explicit-licence-replay/v1','input_fixture':relative(a.inputs),'input_fixture_sha256':digest(a.inputs),'executable_sha256':binary,'selection_policy':'last explicitly listed successful recording for the exact complete normalized source input; prior failures retained','recordings':runs,'cases':results,'files':files,'selected_input_count':len(selected),'scope':'Explicit fresh index only. The adapter must not append historical supplement indexes. Reuse chains stay namespaced; trace_case_id identifies unchanged raw call bytes in each actual run_report.'}
    a.resolutions.write_text(json.dumps(index,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'inputs':relative(a.inputs),'resolutions':relative(a.resolutions),'recorded_entries':len(inputs),'selected_unique_inputs':len(selected),'native_binary':binary,'preserved_failed_entries':sum(len(x['failed_entries_preserved'])for x in runs)}))
if __name__=='__main__':main()
