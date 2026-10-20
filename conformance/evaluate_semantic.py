#!/usr/bin/env python3
"""Held-out mode fields and actual-map plan corpus checks, without model calls."""
import argparse
from collections import Counter
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--label',required=True)
    parser.add_argument('--resolutions',type=Path,default=ROOT/'conformance/fixtures/semantic_resolutions.json')
    parser.add_argument('--plan-report',type=Path,default=ROOT/'conformance/reports/go-plan-corpus.json')
    a=parser.parse_args()
    if Path(a.label).name!=a.label or a.label in ('','.','..'):raise SystemExit('label must be a filename component')
    destination=ROOT/f'conformance/reports/semantic-heldout-{a.label}.json'
    if destination.exists():raise SystemExit('Refusing to overwrite an earlier evaluation')
    actual=json.loads(a.resolutions.read_text());results={x['case_id']:x for x in actual['cases']}
    oracle=json.loads((ROOT/'conformance/fixtures/semantic_expectations.json').read_text())
    inputs=json.loads((ROOT/'conformance/fixtures/semantic_cases.json').read_text())
    expected={x['case_id']:x for x in oracle['cases']}
    plan=json.loads(a.plan_report.read_text());plan_calls={x['id']:x for x in plan['calls']}
    rows=[]
    for item in inputs['cases']:
        result=results[item['case_id']];wanted=expected[item['case_id']]
        if item['input_sha256']!=result['input_sha256'] or wanted['input_sha256']!=result['input_sha256']:raise SystemExit('Original input hash differs')
        row={k:item[k] for k in ('case_id','nodeid','input_sha256')};row['kind']=item['input']['kind'];row['checks']=[];rows.append(row)
        def compare(field,want,got):row['checks'].append({'field':field,'expected':want,'actual':got,'matched':want==got})
        if row['kind']=='mode':
            if result['status']!='mode_decided':row.update(status='native_decision_failed',error=result.get('error'));continue
            old=wanted['expected'];new=result['mode_decision']
            for before,after in (('mode','mode'),('confidence','confidence'),('rule_fired','rule')):compare(after,old[before],new[after])
            compare('alternatives',sorted(old['alternatives']),sorted(new.get('alternatives') or []))
            # This is a source audit-contract marker, not an intent detector.
            marker='overridden by the declared role'
            compare('declared_role_override_audit',any(marker in x for x in old['signals']),any(marker in x for x in new['signals']))
            row['status']='checked_mode_fields_match' if all(c['matched'] for c in row['checks']) else 'heldout_mode_difference'
        else:
            replay=plan_calls[item['case_id']];row['actual_plan_replay_status']=replay['status'];row['actual_model_status']=result['status']
            row['status']={'matched':'original_internal_plan_matches','matched_original_test_intake_refusal':'final_intake_refusal_separately_verified'}.get(replay['status'],'native_plan_replay_failed')
    data={'source_commit':inputs['source_commit'],'actual_run_report':actual['archived_report'],
        'plan_report':str(a.plan_report.resolve().relative_to(ROOT)),
        'scope':'Held-out mode fields/audit markers and original deterministic plan replay are separate from full semantic/compile parity. Exact natural-language signals and clarification are verified only by the original pytest replay, not this field score.',
        'counts':dict(Counter(x['status'] for x in rows)),'cases':rows}
    destination.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');print(json.dumps(data['counts']))

if __name__=='__main__':main()
