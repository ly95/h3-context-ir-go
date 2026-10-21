#!/usr/bin/env python3
"""Independently compare recorded native licence decisions with held-out source evidence.

This is an evaluator only: it reads oracle outputs after model work and never
calls a model. Field agreement is not mislabelled as full original-assert parity.
"""
import argparse
import collections
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--run',type=Path,required=True);p.add_argument('--label',required=True)
    a=p.parse_args()
    if Path(a.label).name!=a.label or a.label in ('','.','..'):raise SystemExit('label must be a filename component')
    destination=ROOT/f'conformance/reports/licence-heldout-{a.label}.json'
    if destination.exists():raise SystemExit('Refusing to overwrite an earlier evaluation')
    inputs=json.loads((ROOT/'conformance/fixtures/licence_inputs.json').read_text())
    expected={x['case_id']:x for x in json.loads((ROOT/'conformance/fixtures/licence_expectations.json').read_text())['cases']}
    source_tests={x['nodeid']:x for x in json.loads((ROOT/'conformance/reports/upstream-licence-capture.json').read_text())['collected']}
    paths=sorted(a.run.glob('*/result.json'))
    results=[json.loads(path.read_text()) for path in paths]
    if (a.run/'report.json').is_file():results=json.loads((a.run/'report.json').read_text())['results']
    byhash={x['input_sha256']:x for x in results}
    rows=[]
    for item in inputs['cases']:
        oracle=expected[item['case_id']];result=byhash.get(item['input_sha256'])
        row={'case_id':item['case_id'],'nodeid':item['nodeid'],'function':item['function'],'input_sha256':item['input_sha256'],
            'original_assertions':source_tests[item['nodeid']]['assertions'],'checks':[]}
        rows.append(row)
        if result is None:row['status']='not_recorded_yet';continue
        row['actual_case_id']=result['case_id'];row['actual_status']=result['status']
        if result['status']!='licence_decided':row['status']='native_decision_failed';row['error']=result.get('error');row['issues']=result.get('issues');continue
        decision=result['licence_decision'];row['actual']=decision
        def compare(field,want,got,scope):
            row['checks'].append({'field':field,'expected':want,'actual':got,'matched':want==got,'scope':scope})
        if item['function']=='h3ir.licence.resolve_licence':
            for attribute,want in oracle['expected']['governs'].items():compare('governs.'+attribute,want,decision['governs'].get(attribute),'Original structured precedence oracle; prose and final rendering need separate verification')
            compare('medium_transferred',oracle['properties']['medium_transferred'],decision['medium_transferred'],'A rendering-medium transformation never changes identity-retention marker semantics')
        else:
            old=oracle['expected'];style=decision['style']
            compare('style.source',old['source'],style['source'],'Which input governs the target medium')
            compare('style.discrepancy',oracle['properties']['discrepancy'],style['discrepancy'],'Caller-visible retained-medium conflict')
            compare('style.requested_medium',old['requested_medium'] or '',style['requested_medium'],'Original canonical medium taxonomy, consumed by transformation checks')
            compare('style.reference_medium',old['observed_medium'] or '',style['reference_medium'],'Original canonical medium taxonomy; free-form medium prose is not the same wire value')
            row['original_style_phrase']=old['phrase']
        # Evidence invariants are independently decidable without classifying
        # any user utterance or garment phrase in Python.
        subjects={s['label']:s for s in decision.get('subjects')or[]}
        for identity in decision.get('identity')or[]:
            subject=subjects.get(identity['label'])
            if subject is None:
                compare('identity.label.exists',True,False,'Typed identity must identify an actual observed subject');continue
            retained=identity.get('retained_attributes',[])
            compare('identity.retained_is_observed.'+identity['label'],True,set(retained)<=set(subject.get('attributes',[])),'No invented attribute is promoted to observation')
            compare('identity.pose_not_promoted.'+identity['label'],False,bool(set(retained)&(set(subject.get('pose',[]))-set(subject.get('attributes',[])))),'Transient pose does not become identity')
        row['status']='all_checked_fields_match' if all(x['matched'] for x in row['checks']) else 'heldout_field_difference'
    counts=dict(collections.Counter(x['status'] for x in rows))
    snapshot={'source_commit':inputs['source_commit'],'actual_run':str(a.run.resolve()),'complete_actual_run':(a.run/'report.json').is_file(),
        'scope':'Independent recorded-result field/evidence checks only. Original assert text is retained for effect review; this report does not claim those full tests passed.',
        'counts':counts,'observed_unique_inputs':len(byhash),'observed_result_files':len(paths),'cases':rows}
    destination.write_text(json.dumps(snapshot,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'report':str(destination.relative_to(ROOT)),'counts':counts,'observed_unique_inputs':len(byhash)}))

if __name__=='__main__':main()
