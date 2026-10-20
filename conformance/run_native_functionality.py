#!/usr/bin/env python3
"""Verify every pinned applicable source node against a real native boundary.

Two honest evidence forms coexist: an executed Go test, or unchanged original
assertions whose primary producer is the immutable Go bridge. A Python harness
asserting Go output is valid; mere helper reachability is not. Failed historical
source assertions are retained even when a separately authorized Go adaptation
passes. No live provider or Python product is called by this aggregator.
"""
import argparse
import collections
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile

from run_go_adapted import source_files, digest

ROOT=Path(__file__).resolve().parents[1]
PIN='fd031e136ae3d89147324c0d1b2aa65e838c21f5'
MODULE='github.com/ly95/contextir'
NA={
 'tests/test_supported_python_versions.py::test_no_backslash_inside_an_f_string_expression':'Python f-string grammar is not a Go runtime feature; native Go parsing/build and vet run separately.',
 'tests/test_supported_python_versions.py::test_every_module_parses':'Python module grammar is not a Go runtime feature; every native package is compiled and tested separately.',
}

def require(condition,message):
    if not condition:raise ValueError(message)

def current_runtime_sources():
    run=subprocess.run(['go','list','-deps','-json','./conformance/bridge'],cwd=ROOT,capture_output=True,text=True)
    require(run.returncode==0,'Cannot resolve native bridge runtime sources: '+run.stderr)
    decoder=json.JSONDecoder();text=run.stdout;at=0;names=set()
    while at<len(text):
        while at<len(text) and text[at].isspace():at+=1
        if at==len(text):break
        pkg,at=decoder.raw_decode(text,at)
        if pkg.get('ImportPath','')!=MODULE and not pkg.get('ImportPath','').startswith(MODULE+'/'):continue
        directory=Path(pkg['Dir']).resolve();require(directory.is_relative_to(ROOT),'Native runtime imports workspace-external source')
        for name in pkg.get('GoFiles',[])+pkg.get('CgoFiles',[])+pkg.get('EmbedFiles',[]):names.add(str((directory/name).relative_to(ROOT)))
    for name in ('go.mod','go.sum'):
        if (ROOT/name).is_file():names.add(name)
    return {name:digest(ROOT/name) for name in sorted(names)}

def load_primary_report(path,provenance_path,inventory):
    report=json.loads(path.read_text());provenance=json.loads(provenance_path.read_text())
    require(report.get('source_commit')==PIN and report.get('collection_matches_baseline') is True,'Original source collection is not pinned/unchanged')
    collected={x['nodeid']:x for x in report['collected']}
    require(len(report['collected'])==991 and set(collected)==inventory,'Fresh source report must contain every original nodeid exactly once')
    require(provenance.get('report_sha256')==digest(path),'Fresh source report hash differs from provenance')
    require(provenance.get('adapter_and_fixture_files_unchanged_during_run') is True,'Adapters or external replay fixtures changed during original assertions')
    require(provenance.get('source_snapshot_verified') is True,'Original bridge lacks an immutable verified source build')
    build=provenance.get('verified_bridge_build') or {};bridge_hash=report.get('go_bridge_sha256')
    require(bridge_hash and provenance.get('bridge_binary_sha256')==bridge_hash and build.get('bridge_binary_sha256')==bridge_hash,'Original report/actual bridge/build hashes disagree')
    binary=Path(build.get('bridge_binary',''));require(binary.is_file() and digest(binary)==bridge_hash,'Actual immutable bridge binary is unavailable or changed')
    manifest_path=Path(build['frozen_source_manifest']);require(manifest_path.is_file() and digest(manifest_path)==build['frozen_source_manifest_sha256'],'Immutable bridge source manifest changed')
    manifest=json.loads(manifest_path.read_text());archive=Path(build['source_archive'])
    require(archive.is_file() and digest(archive)==build['source_archive_sha256']==manifest['source_archive_sha256'],'Immutable source archive changed')
    source_hashes=manifest['sources'];seen=set()
    with tarfile.open(archive) as tar:
        for member in tar:
            if member.name not in source_hashes:continue
            require(member.isfile() and member.name not in seen,'Invalid or duplicate immutable source archive member')
            stream=tar.extractfile(member);require(stream is not None and hashlib.sha256(stream.read()).hexdigest()==source_hashes[member.name],'Immutable source member differs: '+member.name);seen.add(member.name)
    require(seen==set(source_hashes),'Immutable source archive lacks manifest files')
    runtime=current_runtime_sources()
    require(all(source_hashes.get(name)==sha for name,sha in runtime.items()),'Current native runtime/embedded sources differ from the immutable original-assertion bridge build')
    toolchain=subprocess.check_output(['go','env','GOOS','GOARCH','GOVERSION'],cwd=ROOT,text=True).splitlines()
    require(build.get('go_environment')==toolchain,'Original bridge and current Go tests use different toolchain/platform')
    for key in ('adapter_source_sha256','external_fixture_sha256','licence_recorded_file_sha256'):
        for name,sha in provenance.get(key,{}).items():require((ROOT/name).is_file() and digest(ROOT/name)==sha,'Current recorded adapter/fixture differs: '+name)
    require(provenance.get('adapter_source_sha256'),'Original Python adapter hashes are missing')
    adapter_archive=ROOT/provenance['adapter_source_archive'];require(adapter_archive.is_file() and digest(adapter_archive)==provenance['adapter_source_archive_sha256'],'Archived assertion adapters changed')
    if provenance.get('licence_resolutions'):
        index=Path(provenance['licence_resolutions']);require(index.is_file() and digest(index)==provenance['licence_resolutions_sha256'],'Explicit fresh licence replay index changed')
        for call in report.get('go_core_calls',[]):
            if call.get('function') not in ('h3ir.licence.resolve_licence','h3ir.style.resolve_style'):continue
            args=call['arguments'];require(args.get('actual_licence_index_sha256')==digest(index),'Original source used an unselected historical licence index')
            value=call.get('result') or call.get('failed_native_value') or {}
            require(value.get('actual_recorded_requests_verified')==value.get('recorded_outputs_consumed') and value.get('recorded_outputs_consumed',0)>0,'Licence call did not verify every actual recorded request/schema')
    return report,provenance,{'report':str(path),'report_sha256':digest(path),'provenance':str(provenance_path),'provenance_sha256':digest(provenance_path),'bridge_binary_sha256':bridge_hash,'runtime_source_sha256':runtime,'source_report_counts':report.get('counts'),'fresh_primary_sources_verified':True}

def primary_result(case,report):
    node=case['upstream_nodeid'];item=next((x for x in report['collected'] if x['nodeid']==node),None)
    phase=next((x for x in item.get('phases',[]) if x.get('phase')=='call'),{}) if item else {}
    operations=collections.Counter(x['function'] for x in report.get('go_core_calls',[]) if x.get('nodeid')==node)
    validators=[x for x in report.get('validator_calls',[]) if x.get('nodeid')==node]
    missing=[x for x in case.get('required_primary_operations',[]) if not operations[x]]
    passed=bool(item and item.get('upstream_status')=='passed' and phase.get('outcome')=='passed' and not missing and (validators or not case.get('require_native_validator_calls')))
    return {'status':'passed' if passed else 'failed','original_call_phase':phase.get('outcome'),'actual_primary_operation_counts':dict(operations),'required_operations_missing':missing,'actual_native_validator_calls':len(validators),'evidence_kind':'unchanged original assertions consume actual immutable native API output; no independent Go leaf is claimed'}

def go_result(case,tests):
    package=case['go_package'];pkg=MODULE+('/'+package[2:] if package!='.' else '')
    name=case['go_test'];selected={t:s for (pk,t),s in tests.items() if pk==pkg and (t==name or t.startswith(name+'/'))}
    return {'status':'passed' if selected.get(name)=='pass' and all(x=='pass' for x in selected.values()) else 'failed','actual_go_events':selected,'evidence_kind':'executed native Go test'}

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--label',required=True);p.add_argument('--require-all',action='store_true')
    p.add_argument('--original-report',type=Path,help='Fresh full pinned pytest_go_draft report for original_go_primary mappings')
    p.add_argument('--original-provenance',type=Path,help='Defaults to <original-report stem>-provenance.json')
    a=p.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*',a.label):p.error('label must be a plain filename component')
    matrix=json.loads((ROOT/'conformance/matrix.json').read_text());require(matrix['source_commit']==PIN and len(matrix['tests'])==991,'Original pinned inventory changed')
    inventory={x['nodeid'] for x in matrix['tests']};require(set(NA)<=inventory,'Syntax exclusions differ from original inventory')
    manifests={};by_node={}
    for path in sorted((ROOT/'conformance/adaptations').glob('remaining_*.json')):
        data=json.loads(path.read_text());require(data['source_commit']==PIN,'Unpinned adaptation '+str(path));manifests[str(path.relative_to(ROOT))]=digest(path)
        for case in data['cases']:
            node=case['upstream_nodeid'];kind=case.get('verification_kind','go_test')
            require(node in inventory and node not in NA,'Invalid adapted nodeid '+node);require(node not in by_node,'Duplicate native adaptation '+node)
            require(kind in ('go_test','original_go_primary'),'Unknown evidence kind '+node)
            if kind=='go_test':require(case.get('go_test') and case.get('go_package'),'Missing actual Go test '+node)
            else:require(case.get('primary_go_boundaries') and (case.get('required_primary_operations') or case.get('require_native_validator_calls')),'Missing actual primary API producer '+node)
            by_node[node]={**case,'verification_kind':kind,'manifest':str(path.relative_to(ROOT))}
    missing_all=sorted(inventory-set(NA)-set(by_node));gaps=json.loads((ROOT/'conformance/reports/coverage_gaps.json').read_text());missing_gap=sorted({x['nodeid'] for x in gaps['tests']}-set(NA)-set(by_node))
    require(not missing_gap,'Unmapped original coverage gaps: '+json.dumps(missing_gap));require(not a.require_all or not missing_all,'Missing primary-assertion mappings: '+json.dumps(missing_all))
    primary=None;primary_provenance=None;primary_evidence=None
    if any(x['verification_kind']=='original_go_primary' for x in by_node.values()):
        require(a.original_report is not None,'--original-report is required for original_go_primary cases')
        path=a.original_report.resolve();prov=a.original_provenance.resolve() if a.original_provenance else path.with_name(path.stem+'-provenance.json')
        primary,primary_provenance,primary_evidence=load_primary_report(path,prov,inventory)
    out=ROOT/'conformance/reports'/('native-functionality-'+a.label);paths=[out.with_suffix(x) for x in ('.json','.jsonl','.sources.tar.gz','.stderr.txt')];require(not any(x.exists() for x in paths),'Refusing to overwrite a prior run')
    hashes=source_files()
    with tarfile.open(paths[2],'x:gz') as tar:
        for name in hashes:tar.add(ROOT/name,arcname=name,recursive=False)
    env=dict(os.environ,GOWORK='off')
    for key in ('DEEPSEEK_API_KEY','OPENAI_API_KEY','ANTHROPIC_API_KEY'):env.pop(key,None)
    command=['go','test','-json','-count=1','-timeout=240s','./...'];started=dt.datetime.now(dt.timezone.utc).isoformat();run=subprocess.run(command,cwd=ROOT,env=env,capture_output=True,text=True)
    paths[1].write_text(run.stdout);paths[3].write_text(run.stderr);events=[json.loads(x) for x in run.stdout.splitlines()];tests={};packages={}
    for e in events:
        if e.get('Action') not in ('run','pass','skip','fail'):continue
        if e.get('Test'):tests[(e['Package'],e['Test'])]=e['Action']
        else:packages[e['Package']]=e['Action']
    results=[]
    for node,case in sorted(by_node.items()):
        value=primary_result(case,primary) if case['verification_kind']=='original_go_primary' else go_result(case,tests)
        supplements=[]
        for check in case.get('supplemental_go_checks',[]):supplements.append({**check,**go_result(check,tests)})
        if any(x['status']!='passed' for x in supplements):value['status']='failed'
        results.append({**case,**value,'supplemental_go_results':supplements})
    unchanged=hashes==source_files();passed=run.returncode==0 and unchanged and all(x['status']=='passed' for x in results) and all(v=='pass' for v in tests.values())
    report={'schema':'contextir-native-functionality/v2','source_commit':PIN,'started_at':started,'finished_at':dt.datetime.now(dt.timezone.utc).isoformat(),'command':command,'exit_code':run.returncode,'passed':passed,'counts':{'original_nodeids':len(inventory),'mapped_applicable_nodeids':len(results),'mapped_passed':sum(x['status']=='passed' for x in results),'original_go_primary_mappings':sum(x['verification_kind']=='original_go_primary' for x in results),'independent_go_mappings':sum(x['verification_kind']=='go_test' for x in results),'not_applicable_python_syntax':len(NA),'unmapped_original_nodeids':len(missing_all),'go_root_tests':sum('/' not in t for _,t in tests),'go_subtests':sum('/' in t for _,t in tests),'failed_go_events':sum(x=='fail' for x in tests.values()),'skipped_go_events':sum(x=='skip' for x in tests.values())},'cases':results,'not_applicable':[{'upstream_nodeid':n,'reason':s} for n,s in sorted(NA.items())],'unmapped_original_nodeids':missing_all,'original_primary_evidence':primary_evidence,'live_provider_calls':0,'model_execution':'Controlled typed doubles and hash-verified actual model recordings; deterministic native assertions need no provider credentials. This is not a video-model quality test.','scope':'Executed native Go adaptations plus unchanged source assertions consuming actual native primary output; helper reachability alone is not accepted. Original full pytest pass/fail counts remain separately reported and unmodified.','source_files_unchanged_during_run':unchanged,'source_sha256':hashes,'source_archive':str(paths[2].relative_to(ROOT)),'source_archive_sha256':digest(paths[2]),'raw_go_events':str(paths[1].relative_to(ROOT)),'raw_go_events_sha256':digest(paths[1]),'manifests_sha256':manifests,'git_head':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),'go_version':subprocess.check_output(['go','version'],cwd=ROOT,text=True).strip()}
    paths[0].write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({'report':str(paths[0].relative_to(ROOT)),'passed':passed,'counts':report['counts']},ensure_ascii=False));return 0 if passed else 1

if __name__=='__main__':
    try:sys.exit(main())
    except ValueError as e:raise SystemExit(str(e))
