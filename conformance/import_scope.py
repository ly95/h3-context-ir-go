#!/usr/bin/env python3
"""Archive the complete actual scope run without opening held-out expectations."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
from supplement_archive import supplement

ROOT=Path(__file__).resolve().parents[1]

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--run',type=Path,required=True);p.add_argument('--label',required=True)
    a=p.parse_args()
    if Path(a.label).name!=a.label or a.label in ('','.','..'):raise SystemExit('label must be a filename component')
    inputs=json.loads((ROOT/'conformance/fixtures/scope_inputs.json').read_text())
    ids={x['case_id']:x['input_sha256'] for x in inputs['cases']}
    source=a.run.resolve();raw=json.loads((source/'report.json').read_text())
    if raw['Source']!=inputs['source_commit'] or raw['Kind']!='scope':raise SystemExit('Unexpected source revision or run kind')
    if {x['case_id'] for x in raw['results']}!=set(ids):raise SystemExit('Run lacks complete original scope corpus')
    if any(x['input_sha256']!=ids[x['case_id']] for x in raw['results']):raise SystemExit('Original input hash differs')
    target=ROOT/f'conformance/reports/live-scope-{a.label}'
    if target.exists():raise SystemExit('Refusing to overwrite an archived run')
    target.mkdir(parents=True);shutil.copyfile(source/'report.json',target/'report.json')
    files=[]
    for path in sorted(list(source.glob('*/call-*'))+list(source.glob('*/result.json'))):
        dest=target/path.relative_to(source);dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(path,dest)
        files.append({'path':str(dest.relative_to(ROOT)),'sha256':hashlib.sha256(path.read_bytes()).hexdigest()})
    cases=[]
    for x in raw['results']:
        case={k:x.get(k) for k in ('case_id','input_sha256','status','reused_from','scope_decision','calls','issues','error')}
        case['run_report']=str((target/'report.json').relative_to(ROOT));cases.append(case)
    data={'source_commit':raw['Source'],'model':raw['Model'],'reasoning':raw['Reasoning'],
        'executable_sha256':raw.get('executable_sha256'),'go_version':raw.get('go_version'),
        'scope':'Actual native DecideScopeInput results/model outputs only; held-out expectations were not read.',
        'files':files,'cases':cases,'archived_report':str((target/'report.json').relative_to(ROOT))}
    out=json.dumps(data,ensure_ascii=False,indent=2)+'\n'
    (target/'resolutions.json').write_text(out);(ROOT/'conformance/fixtures/scope_resolutions.json').write_text(out)
    supplement(source,target)
    print(f'Archived {len(cases)} actual original cases, {len(files)} result/trace files')

if __name__=='__main__':main()
