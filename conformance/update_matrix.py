#!/usr/bin/env python3
"""Merge evidence without turning Python-only passes into native Go coverage."""
import argparse
import collections
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]

def read(path):return json.loads(path.read_text(encoding='utf-8'))
def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--core-report',type=Path,required=True)
    parser.add_argument('--validator-report',type=Path,default=ROOT/'conformance/reports/go-validator-pytest-round3.json')
    parser.add_argument('--licence-report',type=Path,default=ROOT/'conformance/reports/go-licence-result-pytest-round1.json')
    parser.add_argument('--scope-report',type=Path,default=ROOT/'conformance/reports/go-scope-pytest-round1.json')
    parser.add_argument('--plan-report',type=Path,default=ROOT/'conformance/reports/go-plan-pytest-round3.json')
    parser.add_argument('--scope-legacy-report',type=Path,default=ROOT/'conformance/reports/go-scope-legacy-pytest-round1.json')
    parser.add_argument('--combined-report',type=Path,default=ROOT/'conformance/reports/go-combined-pytest-ca28-frozen.json')
    parser.add_argument('--draft-report',type=Path,help='Strict complete native Draft original-assert run; reported separately from component coverage.')
    args=parser.parse_args()
    baseline=read(ROOT/'conformance/reports/upstream-pytest.json')
    core_path=args.core_report.resolve();core=read(core_path)
    validator=read(args.validator_report.resolve())
    licence=read(args.licence_report.resolve()) if args.licence_report.is_file() else None
    by_licence={t['nodeid']:t for t in licence['collected']} if licence else {}
    licence_calls=collections.Counter(c['nodeid'] for c in licence['licence_result_calls']) if licence else {}
    scope=read(args.scope_report.resolve()) if args.scope_report.is_file() else None
    scope_calls=collections.Counter(c['nodeid'] for c in scope['go_core_calls'] if c['function']=='h3ir.creativity.build') if scope else {}
    by_scope={t['nodeid']:t for t in scope['collected']} if scope else {}
    legacy=read(args.scope_legacy_report.resolve()) if args.scope_legacy_report.is_file() else None
    legacy_calls=collections.Counter(c['nodeid'] for c in legacy['go_core_calls'] if c['function']=='h3ir.creativity.build') if legacy else {}
    by_legacy={t['nodeid']:t for t in legacy['collected']} if legacy else {}
    fullplan=read(args.plan_report.resolve()) if args.plan_report.is_file() else None
    fullplan_calls=collections.Counter(c['nodeid'] for c in fullplan['go_core_calls'] if c['function']=='h3ir.plan.build_plan') if fullplan else {}
    by_fullplan={t['nodeid']:t for t in fullplan['collected']} if fullplan else {}
    combined=read(args.combined_report.resolve()) if args.combined_report.is_file() else None
    by_combined={t['nodeid']:t for t in combined['collected']} if combined else {}
    combined_calls=collections.defaultdict(collections.Counter)
    for call in combined['go_core_calls'] if combined else []:combined_calls[call['nodeid']][call['function']]+=1
    draft=read(args.draft_report.resolve()) if args.draft_report else None
    by_draft={t['nodeid']:t for t in draft['collected']} if draft else {}
    draft_calls=collections.defaultdict(collections.Counter)
    for call in draft['go_core_calls'] if draft else []:draft_calls[call['nodeid']][call['function']]+=1
    plan_path=ROOT/'conformance/reports/go-plan-corpus.json'
    plan=read(plan_path) if plan_path.exists() else {'calls':[]}
    by_core={t['nodeid']:t for t in core['collected']}
    by_validator={t['nodeid']:t for t in validator['collected']}
    core_calls=collections.defaultdict(collections.Counter)
    for c in core['go_core_calls']:core_calls[c['nodeid']][c['function']]+=1
    validator_calls=collections.Counter(c['nodeid'] for c in validator['validator_calls'])
    plan_calls=collections.defaultdict(collections.Counter)
    for c in plan['calls']:plan_calls[c['nodeid']][c['status']]+=1
    tests=[]
    for original in baseline['collected']:
        item=dict(original);node=item['nodeid'];vc=by_validator.get(node);cc=by_core.get(node)
        item['go_validator']={'status':vc['go_status'] if vc else 'not_run','calls':validator_calls[node],
                              'report':str(args.validator_report.resolve().relative_to(ROOT))}
        item['go_core_assertions']={'status':cc['upstream_status'] if cc and cc['native_go_touched'] else 'not_covered',
                                   'functions':dict(core_calls[node]),'modules':cc['native_go_modules'] if cc else [],
                                   'report':str(core_path.relative_to(ROOT)),
                                   'scope':core['implementation_under_test']}
        item['go_plan_corpus']=dict(plan_calls[node])
        dr=by_draft.get(node)
        item['go_whole_draft_run']={'status':dr['upstream_status'] if dr and dr['native_go_touched'] else 'not_covered',
            'functions':dict(draft_calls[node]),'full_native_draft_calls':dr.get('full_native_draft_calls',0) if dr else 0,
            'native_draft_artifacts':dr.get('full_native_draft_artifacts',0) if dr else 0,
            'native_draft_render_calls':dr.get('native_draft_artifact_render_calls',0) if dr else 0,
            'native_plan_only':dr.get('native_plan_only',False) if dr else False,
            'report':str(args.draft_report.resolve().relative_to(ROOT)) if draft else None,
            'scope':'Original complete deterministic_draft entries execute public Go Compiler.Draft; their renderer executes native h3.Compile on the same Document. Per-entry calls distinguish those boundaries from other native components and Python collaborators. Failed native calls are not counted as complete Draft passes.'}
        co=by_combined.get(node)
        item['go_combined_assertions']={'status':co['upstream_status'] if co and co['native_go_touched'] else 'not_covered',
            'functions':dict(combined_calls[node]),'report':str(args.combined_report.resolve().relative_to(ROOT)) if combined else None,
            'scope':'Combined original assertions with actual mode/scope/licence model tapes through native components. Failed assertions and unimplemented Python collaborators remain explicit.'}
        pc=by_fullplan.get(node)
        item['go_full_plan_assertions']={'status':pc['upstream_status'] if pc and fullplan_calls[node] else 'not_covered',
            'calls':fullplan_calls[node],'report':str(args.plan_report.resolve().relative_to(ROOT)) if fullplan else None,
            'scope':'Original build_plan calls execute native plan.Build; remaining source collaborators are Python.'}
        sc=by_scope.get(node)
        item['go_scope_assertions']={'status':sc['upstream_status'] if sc and scope_calls[node] else 'not_covered',
            'calls':scope_calls[node],'report':str(args.scope_report.resolve().relative_to(ROOT)) if scope else None,
            'scope':'Original creativity.build uses actual paid model tapes through native DecideScopeInput; native Scope formatter helpers are recorded separately.'}
        leg=by_legacy.get(node)
        item['go_scope_explicit_legacy_policy_assertions']={'status':leg['upstream_status'] if leg and legacy_calls[node] else 'not_covered',
            'calls':legacy_calls[node],'report':str(args.scope_legacy_report.resolve().relative_to(ROOT)) if legacy else None,
            'scope':'Separately selected public supplied_content_wins policy; default-reject evidence is retained independently. Model outputs are unchanged.'}
        lc=by_licence.get(node)
        item['recorded_go_licence_results']={'status':lc['upstream_status'] if lc and lc['recorded_native_licence_result_touched'] else 'not_covered',
            'calls':licence_calls.get(node,0),'report':str(args.licence_report.resolve().relative_to(ROOT)) if licence else None,
            'scope':'Original assertions over actual archived native licence/style outputs. This is separate from current-binary component coverage and does not establish full compile parity.'}
        # Whole-feature status is intentionally not upgraded by a partial bridge.
        item['go_status']='pending_whole_feature_native_verification'
        tests.append(item)
    out={'source_commit':baseline['source_commit'],'tests':tests,
         'native_go_assertion_counts':core['native_go_assertion_counts'],
         'native_go_not_covered_count':core['native_go_not_covered_count'],
         'native_plan_corpus_counts':plan.get('counts',{}),
         'recorded_licence_original_assertion_counts':licence['recorded_native_licence_assertion_counts'] if licence else {},
         'scope_original_assertion_counts':dict(collections.Counter(t['go_scope_assertions']['status'] for t in tests if t['go_scope_assertions']['status']!='not_covered')),
         'full_plan_original_assertion_counts':dict(collections.Counter(t['go_full_plan_assertions']['status'] for t in tests if t['go_full_plan_assertions']['status']!='not_covered')),
         'scope_explicit_legacy_policy_original_assertion_counts':dict(collections.Counter(t['go_scope_explicit_legacy_policy_assertions']['status'] for t in tests if t['go_scope_explicit_legacy_policy_assertions']['status']!='not_covered')),
         'whole_feature_go_tests_counted_as_passed':0,
         'combined_native_assertion_counts':combined['native_go_assertion_counts'] if combined else {},
         'combined_python_only_count':combined['native_go_not_covered_count'] if combined else None,
         'whole_draft_run_counts':draft['counts'] if draft else None,
         'whole_draft_run_native_counts':draft['native_go_assertion_counts'] if draft else None,
         'whole_draft_run_python_only_count':draft['native_go_not_covered_count'] if draft else None,
         'note':'Native module assertions and corpus comparisons are separate from whole-feature end-to-end native implementation.'}
    path=ROOT/'conformance/matrix.json';path.write_text(json.dumps(out,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    rows=[]
    for source in sorted({i['source'] for i in tests}):
        cases=[i for i in tests if i['source']==source]
        covered=sum(i['go_core_assertions']['status']=='passed' for i in cases)
        failed=sum(i['go_core_assertions']['status']=='failed' for i in cases)
        skipped=sum(i['go_core_assertions']['status']=='skipped' for i in cases)
        python_only=sum(i['go_core_assertions']['status']=='not_covered' for i in cases)
        rows.append({'source':source,'upstream_tests':len(cases),'upstream_passed':sum(i['upstream_status']=='passed' for i in cases),
                     'native_module_assertions_passed':covered,'native_module_assertions_failed':failed,
                     'native_module_assertions_skipped':skipped,'python_only':python_only,
                     'combined_native_assertions_passed':sum(i['go_combined_assertions']['status']=='passed' for i in cases),
                     'combined_native_assertions_failed':sum(i['go_combined_assertions']['status']=='failed' for i in cases),
                     'combined_python_only':sum(i['go_combined_assertions']['status']=='not_covered' for i in cases),
                     'whole_draft_run_native_assertions_passed':sum(i['go_whole_draft_run']['status']=='passed' for i in cases),
                     'whole_draft_run_native_assertions_failed':sum(i['go_whole_draft_run']['status']=='failed' for i in cases),
                     'whole_draft_run_python_only':sum(i['go_whole_draft_run']['status']=='not_covered' for i in cases),
                     'full_native_draft_nodeids':sum(i['go_whole_draft_run']['full_native_draft_calls']>0 for i in cases),
                     'native_whole_feature':'pending'})
    (ROOT/'conformance/reports/coverage_by_file.json').write_text(json.dumps({'files':rows},indent=2)+'\n')
    print(json.dumps({k:out[k] for k in ('native_go_assertion_counts','native_go_not_covered_count','native_plan_corpus_counts')}))
if __name__=='__main__':main()
