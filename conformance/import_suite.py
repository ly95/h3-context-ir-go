#!/usr/bin/env python3
"""Archive all six actual original-suite outputs; compilation is not a quality verdict."""
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
    source=a.run.resolve();report=json.loads((source/'report.json').read_text());fixture=json.loads((ROOT/'conformance/fixtures/suite_briefs.json').read_text())
    if report['Source']!=fixture['source_commit'] or report['Kind']!='suite':raise SystemExit('Wrong source SHA or run kind')
    if {x['case_id'] for x in report['results']}!={x['id'] for x in fixture['cases']}:raise SystemExit('Missing original suite case')
    target=ROOT/f'conformance/reports/live-suite-{a.label}'
    if target.exists():raise SystemExit('Refusing to overwrite an archived run')
    target.mkdir(parents=True);shutil.copyfile(source/'report.json',target/'report.json')
    files=[]
    for path in sorted(list(source.glob('*/call-*'))+list(source.glob('*/result.json'))):
        relative=path.relative_to(source);destination=target/relative;destination.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(path,destination)
        files.append({'path':str(destination.relative_to(ROOT)),'sha256':hashlib.sha256(path.read_bytes()).hexdigest()})
    summary={'source_commit':report['Source'],'model':report['Model'],'reasoning':report['Reasoning'],
        'scope':'Actual original suite Compile run. A compiled result proves its recorded native validation path, not prose, perception, or rendered-video quality. No video rendering was performed.',
        'archived_report':str((target/'report.json').relative_to(ROOT)),'files':files,
        'cases':[{k:r.get(k) for k in ('case_id','status','error','issues','calls')} for r in report['results']]}
    (target/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
    supplement(source,target)
    print(json.dumps({'archive':str(target.relative_to(ROOT)),'cases':[(r['case_id'],r['status']) for r in report['results']],'trace_files':len(files)}))

if __name__=='__main__':main()
