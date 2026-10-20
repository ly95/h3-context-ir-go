#!/usr/bin/env python3
"""Archive actual model runs and prepare replay-only resolutions; never alter expectations."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
from supplement_archive import supplement

ROOT=Path(__file__).resolve().parents[1]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--run',type=Path,required=True);p.add_argument('--label',required=True)
    a=p.parse_args()
    if Path(a.label).name!=a.label or a.label in ('','.','..'):raise SystemExit('label must be a filename component')
    source=a.run.resolve();raw=json.loads((source/'report.json').read_text())
    inputs={x['case_id']:x for x in json.loads((ROOT/'conformance/fixtures/semantic_cases.json').read_text())['cases']}
    if raw['Source']!='fd031e136ae3d89147324c0d1b2aa65e838c21f5' or raw['Kind']!='semantic':raise SystemExit('unexpected source revision or run kind')
    if len(raw['results'])!=len(inputs) or {x['case_id'] for x in raw['results']}!=set(inputs):raise SystemExit('expected each complete original semantic input exactly once')
    for result in raw['results']:
        if inputs[result['case_id']]['input_sha256']!=result['input_sha256']:raise SystemExit('actual semantic input hash differs from the corpus')
    target=ROOT/'conformance/reports'/f'live-semantic-{a.label}'
    if target.exists():raise SystemExit('cannot overwrite an archived run')
    target.mkdir(parents=True)
    shutil.copyfile(source/'report.json',target/'report.json')
    cases=[];files=[]
    for path in sorted(list(source.glob('*/call-*'))+list(source.glob('*/result.json'))):
        relative=path.relative_to(source);dest=target/relative;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(path,dest)
        files.append({'path':str(dest.relative_to(ROOT)),'sha256':hashlib.sha256(path.read_bytes()).hexdigest()})
    for result in raw['results']:
        id=result['case_id'];original=inputs[id]
        if original['input_sha256']!=result['input_sha256']:raise SystemExit(f'input hash mismatch for {id}')
        cases.append({'case_id':id,'input_sha256':result['input_sha256'],'kind':original['input']['kind'],
            'status':result['status'],'reused_from':result.get('reused_from'),
            'mode_decision':result.get('mode_decision'),
            'replacement_targets':((result.get('result')or{}).get('structure')or{}).get('replacement_targets'),
            'issues':result.get('issues',[]),'calls':result.get('calls') or [],'error':result.get('error'),
            'run_report':str((target/'report.json').relative_to(ROOT))})
    resolution={'source_commit':raw['Source'],'model':raw['Model'],'reasoning':raw['Reasoning'],
                'archived_report':str((target/'report.json').relative_to(ROOT)),
                'scope':'Actual production DecideMode/Propose results. No held-out expectations were read to construct these resolutions.',
                'files':files,'cases':cases}
    (target/'resolutions.json').write_text(json.dumps(resolution,ensure_ascii=False,indent=2)+'\n')
    (ROOT/'conformance/fixtures/semantic_resolutions.json').write_text(json.dumps(resolution,ensure_ascii=False,indent=2)+'\n')
    supplement(source,target)
    print(f'Archived {len(cases)} actual cases and {len(files)} request/output trace files; expectations unchanged')
if __name__=='__main__':main()
