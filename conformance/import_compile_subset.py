#!/usr/bin/env python3
"""Archive an actual selected Compile rerun; preserve the complete prior batch."""
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
    p.add_argument('--reference',type=Path,required=True,help='Immutable complete prior run containing the exact original input hashes.')
    a=p.parse_args()
    if Path(a.label).name!=a.label or a.label in ('','.','..'):raise SystemExit('label must be one filename component')
    source=a.run.resolve();report=json.loads((source/'report.json').read_text());reference=json.loads(a.reference.read_text())
    if report['Kind']not in ('suite','roles')or report['Source']!=reference['Source']or report['Kind']!=reference['Kind']:raise SystemExit('Unexpected pinned source or Compile kind')
    expected={x['case_id']:x['input_sha256']for x in reference['results']};ids=[x['case_id']for x in report['results']]
    if not ids or len(ids)!=len(set(ids))or any(expected.get(x['case_id'])!=x['input_sha256']for x in report['results']):raise SystemExit('Selected actual inputs do not match original recorded inputs exactly')
    target=ROOT/f"conformance/reports/live-{report['Kind']}-{a.label}"
    if target.exists():raise SystemExit('Refusing to overwrite an archived run')
    target.mkdir(parents=True);shutil.copyfile(source/'report.json',target/'report.json');files=[]
    for path in sorted(list(source.glob('*/call-*'))+list(source.glob('*/result.json'))):
        destination=target/path.relative_to(source);destination.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(path,destination)
        files.append({'path':str(destination.relative_to(ROOT)),'sha256':hashlib.sha256(path.read_bytes()).hexdigest()})
    hash_available=all(x['input_sha256']for x in report['results'])
    output={'source_commit':report['Source'],'kind':report['Kind'],'executable_sha256':report['executable_sha256'],'original_input_hash_reference':str(a.reference.resolve().relative_to(ROOT)),'original_input_hash_reference_sha256':hashlib.sha256(a.reference.read_bytes()).hexdigest(),'original_input_hashes_available_and_checked':hash_available,'archived_report':str((target/'report.json').relative_to(ROOT)),'selected_cases':[{'case_id':x['case_id'],'input_sha256':x['input_sha256'],'status':x['status']}for x in report['results']],'files':files,'scope':'Independent selected actual production Compile rerun. Available nonempty original input hashes are checked; empty runner hashes are explicitly unavailable and establish case-ID correspondence only. The prior complete batch and its failures remain unchanged. Compiled is not a prose-quality or role-semantics verdict. Original ROLE_CASES Brief/cards omit their separate deterministic_draft mode argument; production Compile chooses its own mode.'}
    (target/'summary.json').write_text(json.dumps(output,ensure_ascii=False,indent=2)+'\n');supplement(source,target)
    print(json.dumps({'archive':str(target.relative_to(ROOT)),'cases':output['selected_cases'],'trace_files':len(files)}))


if __name__=='__main__':main()
