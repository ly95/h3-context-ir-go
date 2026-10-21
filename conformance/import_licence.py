#!/usr/bin/env python3
"""Archive an actual complete licence run without opening held-out expectations."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
from supplement_archive import supplement

ROOT=Path(__file__).resolve().parents[1]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--run',type=Path,required=True);p.add_argument('--label',required=True)
    p.add_argument('--inputs',type=Path,default=ROOT/'conformance/fixtures/licence_inputs.json')
    p.add_argument('--resolutions',type=Path,default=ROOT/'conformance/fixtures/licence_resolutions.json')
    a=p.parse_args()
    if Path(a.label).name!=a.label or a.label in ('','.','..'):raise SystemExit('label must be a filename component')
    inputs=json.loads(a.inputs.read_text())
    expected_ids={x['case_id']:x['input_sha256'] for x in inputs['cases']}
    source=a.run.resolve();report=json.loads((source/'report.json').read_text())
    if report['Source']!=inputs['source_commit'] or report['Kind']!='licence':raise SystemExit('Unexpected source revision or run kind')
    if len(report['results'])!=len(expected_ids) or {x['case_id'] for x in report['results']}!=set(expected_ids):raise SystemExit('The actual run does not contain the complete selected licence input corpus exactly once')
    for x in report['results']:
        if x['input_sha256']!=expected_ids[x['case_id']]:raise SystemExit('An actual run input hash differs from the original corpus')
    target=ROOT/f'conformance/reports/live-licence-{a.label}'
    if target.exists():raise SystemExit('Refusing to overwrite an archived run')
    target.mkdir(parents=True)
    shutil.copyfile(source/'report.json',target/'report.json')
    files=[]
    for path in sorted(list(source.glob('*/call-*'))+list(source.glob('*/result.json'))):
        relative=path.relative_to(source);destination=target/relative;destination.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(path,destination)
        files.append({'path':str(destination.relative_to(ROOT)),'sha256':hashlib.sha256(path.read_bytes()).hexdigest()})
    cases=[]
    for x in report['results']:
        cases.append({k:x.get(k) for k in ('case_id','input_sha256','status','reused_from','licence_decision','calls','issues','error')})
        cases[-1]['run_report']=str((target/'report.json').relative_to(ROOT))
    data={'source_commit':report['Source'],'model':report['Model'],'reasoning':report['Reasoning'],
        'executable_sha256':report['executable_sha256'],
        'scope':'Actual production DecideLicence results and model tapes only; held-out expectations were not read.',
        'input_fixture':str(a.inputs.resolve().relative_to(ROOT)),
        'input_fixture_sha256':hashlib.sha256(a.inputs.read_bytes()).hexdigest(),
        'files':files,'cases':cases,'archived_report':str((target/'report.json').relative_to(ROOT))}
    text=json.dumps(data,ensure_ascii=False,indent=2)+'\n'
    (target/'resolutions.json').write_text(text)
    a.resolutions.write_text(text)
    supplement(source,target)
    print(f'Archived {len(cases)} actual original cases and {len(files)} trace/result files; oracle unchanged')

if __name__=='__main__':main()
