#!/usr/bin/env python3
"""Exact per-node union of evidence, never a union of pass claims."""
import argparse
from collections import Counter
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--core',type=Path,required=True)
    p.add_argument('--scope',type=Path,default=ROOT/'conformance/reports/go-scope-pytest-round2.json')
    p.add_argument('--legacy-scope',type=Path,default=ROOT/'conformance/reports/go-scope-legacy-pytest-round1.json')
    p.add_argument('--plan',type=Path,default=ROOT/'conformance/reports/go-plan-pytest-round3.json')
    p.add_argument('--licence',type=Path,default=ROOT/'conformance/reports/go-licence-result-pytest-round2.json')
    p.add_argument('--combined',type=Path,default=ROOT/'conformance/reports/go-combined-pytest-round1.json')
    p.add_argument('--draft',type=Path,help='Strict original whole-Draft run, with all failures retained.')
    p.add_argument('--evaluate',type=Path,default=ROOT/'conformance/reports/go-evaluate-pytest-round2.json')
    a=p.parse_args()
    base=json.loads((ROOT/'conformance/reports/upstream-pytest.json').read_text());nodes={x['nodeid']:{'nodeid':x['nodeid'],'source':x['source'],'native_execution':[],'recorded_native_result_assertions':[]} for x in base['collected']}
    unattributed=[]
    for label,path in (('core',a.core),('scope_default',a.scope),('scope_explicit_legacy',a.legacy_scope),('full_plan',a.plan),('combined_including_failed_contract_replays',a.combined),('strict_whole_draft',a.draft),('evaluate_only',a.evaluate)):
        if path is None:continue
        if not path.is_file():continue
        r=json.loads(path.read_text());states={x['nodeid']:x['upstream_status'] for x in r['collected']};calls={}
        for c in r.get('go_core_calls',[]):calls.setdefault(c['nodeid'],Counter())[c['function']]+=1
        for c in r.get('validator_calls',[]):calls.setdefault(c['nodeid'],Counter())['h3ir.validate.validate']+=1
        for node,functions in calls.items():
            if node not in nodes:unattributed.append({'run':label,'nodeid':node,'functions':dict(functions)});continue
            execution={'run':label,'report':str(path.resolve().relative_to(ROOT)),'status':states[node],'functions':dict(functions),'binary_sha256':r.get('go_bridge_sha256')}
            if label=='strict_whole_draft':
                execution['complete_draft_calls']=functions.get('h3ir.draft.deterministic_draft',0)
                execution['native_draft_render_calls']=functions.get('h3ir.render.native_draft',0)
                execution['native_plan_only']=not execution['complete_draft_calls'] and bool(functions.get('h3ir.plan.build_plan'))
            nodes[node]['native_execution'].append(execution)
    r=json.loads(a.licence.read_text());states={x['nodeid']:x['upstream_status'] for x in r['collected']};calls={}
    for c in r['licence_result_calls']:calls.setdefault(c['nodeid'],Counter())[c['function']]+=1
    for node,functions in calls.items():nodes[node]['recorded_native_result_assertions'].append({'report':str(a.licence.resolve().relative_to(ROOT)),'status':states[node],'functions':dict(functions),
        'scope':'Actual prior Go outputs consumed by original assertions; no current Go execution in this replay.'})
    rows=list(nodes.values());native=[x for x in rows if x['native_execution']];either=[x for x in rows if x['native_execution'] or x['recorded_native_result_assertions']]
    untouched=[x for x in rows if not x['native_execution'] and not x['recorded_native_result_assertions']]
    language=[x['nodeid'] for x in untouched if x['source']=='tests/test_supported_python_versions.py']
    output={'source_commit':base['source_commit'],'collected':len(rows),'native_execution_union_nodeids':len(native),
        'native_execution_or_recorded_result_union_nodeids':len(either),'still_python_only_nodeids':len(untouched),
        'language_specific_python_packaging_not_applicable_to_go':language,
        'python_only_by_file':dict(Counter(x['source'] for x in untouched)),
        'unattributed_initialization_calls':unattributed,
        'note':'This is a coverage union, not a pass union or a whole-flow Go parity claim. All per-run failures remain. Recorded historical outputs are distinguished from current native execution.',
        'tests':rows,'still_python_only_exact_list':[x['nodeid'] for x in untouched]}
    (ROOT/'conformance/reports/coverage_union.json').write_text(json.dumps(output,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({k:v for k,v in output.items() if k not in ('tests','still_python_only_exact_list')},ensure_ascii=False))

if __name__=='__main__':main()
