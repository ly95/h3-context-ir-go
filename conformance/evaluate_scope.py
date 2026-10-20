#!/usr/bin/env python3
"""Independently check native permission fields against the held-out source corpus."""
import argparse
from collections import Counter
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
FIELDS={'speech':'speech','music':'music','on-screen text':'onscreen_text'}

def elements(value):return sorted(e for e,key in FIELDS.items() if value[key])

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run',type=Path,required=True);parser.add_argument('--label',required=True)
    a=parser.parse_args()
    if Path(a.label).name!=a.label or a.label in ('','.','..'):raise SystemExit('label must be a filename component')
    destination=ROOT/f'conformance/reports/scope-heldout-{a.label}.json'
    if destination.exists():raise SystemExit('Refusing to overwrite an earlier evaluation')
    inputs=json.loads((ROOT/'conformance/fixtures/scope_inputs.json').read_text())
    expected={x['case_id']:x for x in json.loads((ROOT/'conformance/fixtures/scope_expectations.json').read_text())['cases']}
    report=json.loads((a.run/'report.json').read_text())
    if report['Source']!=inputs['source_commit'] or report['Kind']!='scope':raise SystemExit('Unexpected source or run kind')
    actual={x['case_id']:x for x in report['results']};rows=[]
    for item in inputs['cases']:
        result=actual[item['case_id']];oracle=expected[item['case_id']]
        if result['input_sha256']!=item['input_sha256']:raise SystemExit('Original scope input differs')
        row={k:item[k] for k in ('case_id','nodeid','input_sha256')};row['checks']=[];rows.append(row)
        if result['status']!='scope_decided':row.update(status='native_decision_failed',error=result.get('error'));continue
        decision=result['scope_decision'];row['actual']=decision
        def compare(field,want,got):row['checks'].append({'field':field,'expected':want,'actual':got,'matched':want==got})
        compare('requested',sorted(oracle['expected']['requested']),elements(decision['requested']))
        compare('forbidden',sorted(oracle['expected']['forbidden']),elements(decision['forbidden']))
        compare('magnitude',oracle['supplementary_properties']['magnitude'],decision['allowed']['magnitude'])
        wanted=set(oracle['expected']['requested'])|set(oracle['supplementary_properties']['licensed'])
        wanted-=set(oracle['expected']['forbidden'])
        compare('allowed',sorted(wanted),elements(decision['allowed']))
        row['status']='all_checked_fields_match' if all(c['matched'] for c in row['checks']) else 'heldout_field_difference'
    summary={'source_commit':report['Source'],'actual_run':str(a.run.resolve()),
        'executable_sha256':report.get('executable_sha256'),'go_version':report.get('go_version'),
        'scope':'Held-out structured permission/effect checks only; original pytest assertions are a separate native replay.',
        'counts':dict(Counter(x['status'] for x in rows)),'cases':rows}
    destination.write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(summary['counts']))

if __name__=='__main__':main()
