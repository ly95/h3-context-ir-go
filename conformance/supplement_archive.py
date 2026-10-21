#!/usr/bin/env python3
"""Add missing raw rejected outputs/build hashes without rewriting an archive."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil

ROOT=Path(__file__).resolve().parents[1]

def digest(path):return hashlib.sha256(path.read_bytes()).hexdigest()

def supplement(source,target):
    source=source.resolve();target=target.resolve()
    report=json.loads((source/'report.json').read_text())
    if digest(source/'report.json')!=digest(target/'report.json'):raise RuntimeError('Source report differs from immutable archived report')
    files=[]
    for path in sorted(source.glob('*/call-*')):
        if not path.is_file():continue
        dest=target/path.relative_to(source)
        if dest.exists():
            if digest(dest)!=digest(path):raise RuntimeError('An immutable archived trace differs from its source')
        else:dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(path,dest)
        if path.suffix!='.json':files.append({'path':str(dest.relative_to(ROOT)),'sha256':digest(dest),'kind':'raw_rejected_or_additional_trace_bytes'})
    executable=report.get('executable_sha256');build_status='not_recorded_by_this_run'
    if executable:
        candidate=Path('/tmp')/('contextir-upstream-eval-'+executable[:12]+'.sources.json')
        if candidate.is_file():
            manifest=json.loads(candidate.read_text())
            if manifest.get('executable_sha256',manifest.get('binary_sha256'))!=executable:raise RuntimeError('Build manifest identifies a different executable')
            dest=target/'build-sources.json'
            if dest.exists():
                if digest(dest)!=digest(candidate):raise RuntimeError('An immutable build manifest differs')
            else:shutil.copyfile(candidate,dest)
            files.append({'path':str(dest.relative_to(ROOT)),'sha256':digest(dest),'kind':'native_executable_source_hash_manifest'});build_status='retained'
        else:build_status='not_available_at_supplement_time'
    data={'scope':'Additive evidence only. The original report and existing archive manifests remain byte-identical. Raw rejected outputs are retained, not counted as successful model calls or valid paid usage.',
        'source_run':str(source),'original_report_sha256':digest(target/'report.json'),'executable_sha256':executable,'build_manifest_status':build_status,'files':files}
    path=target/'supplement-manifest-v1.json';out=json.dumps(data,ensure_ascii=False,indent=2)+'\n'
    if path.exists() and path.read_text()!=out:raise RuntimeError('Refusing to rewrite an existing additive supplement manifest')
    path.write_text(out);return data

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--source',type=Path,required=True);p.add_argument('--archive',type=Path,required=True);a=p.parse_args()
    data=supplement(a.source,a.archive);print(json.dumps({'files':len(data['files']),'build_manifest_status':data['build_manifest_status']}))

if __name__=='__main__':main()
