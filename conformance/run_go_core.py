#!/usr/bin/env python3
"""Run immutable original assertions with explicit native component adapters."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile

ROOT=Path(__file__).resolve().parents[1]
PIN='fd031e136ae3d89147324c0d1b2aa65e838c21f5'

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source',type=Path,required=True)
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--label',required=True)
    parser.add_argument('--plugin',choices=('pytest_go_core','pytest_go_plan','pytest_go_scope','pytest_go_scope_legacy','pytest_go_combined','pytest_go_draft'),default='pytest_go_core')
    parser.add_argument('--licence-resolutions',type=Path,help='One explicit complete fresh licence replay index; historical supplement indexes are not loaded.')
    parser.add_argument('--bridge-provenance',type=Path,help='Verified immutable-source bridge build record; its binary hash must match --binary.')
    args=parser.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*',args.label):raise SystemExit('label must be one plain filename component')
    source=args.source.resolve();binary=args.binary.resolve()
    if subprocess.check_output(['git','rev-parse','HEAD'],cwd=source,text=True).strip()!=PIN or subprocess.check_output(['git','status','--porcelain','--untracked-files=no'],cwd=source,text=True):
        raise SystemExit('Expected a clean source checkout at the pinned SHA')
    category={'pytest_go_core':'core','pytest_go_plan':'plan','pytest_go_scope':'scope','pytest_go_scope_legacy':'scope-legacy','pytest_go_combined':'combined','pytest_go_draft':'draft'}[args.plugin]
    report=ROOT/f'conformance/reports/go-{category}-pytest-{args.label}.json'
    provenance=report.with_name(report.stem+'-provenance.json')
    adapter_archive=report.with_name(report.stem+'-adapters.tar.gz')
    if any(path.exists() for path in (report,report.with_suffix('.txt'),provenance,adapter_archive)):raise SystemExit('Refusing to overwrite an earlier test round')
    binary_hash=hashlib.sha256(binary.read_bytes()).hexdigest()
    bridge_build=None
    if args.bridge_provenance:
        bridge_build=json.loads(args.bridge_provenance.read_text())
        if bridge_build.get('bridge_binary_sha256')!=binary_hash:raise SystemExit('Immutable bridge build record does not match the actual bridge binary')
    adapters=sorted((ROOT/'conformance').glob('*.py'))
    adapter_hashes={str(path.relative_to(ROOT)):hashlib.sha256(path.read_bytes()).hexdigest() for path in adapters}
    fixture_hashes={str(path.relative_to(ROOT)):hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted((ROOT/'conformance/fixtures').glob('*.json'))}
    with tarfile.open(adapter_archive,'x:gz') as archive:
        for path in adapters:archive.add(path,arcname=str(path.relative_to(ROOT)))
    env=os.environ.copy()
    env.update(CONTEXTIR_GO_VALIDATOR=str(binary),CONTEXTIR_CAPTURE_PATH=str(report),
        PYTHONPATH=str(ROOT/'conformance')+os.pathsep+str(source),
        H3IR_STATE_DIR='/tmp/contextir-upstream-go-core-state')
    replay_hashes={}
    if args.licence_resolutions:
        selected=args.licence_resolutions.resolve()
        if not selected.is_file():raise SystemExit('Explicit licence replay index does not exist')
        if not selected.is_relative_to(ROOT):raise SystemExit('Replay index must be archived inside this repository')
        index=json.loads(selected.read_text())
        replay_hashes={x['path']:x['sha256'] for x in index.get('files',[])}
        replay_hashes.update({x['report']:x['report_sha256'] for x in index.get('recordings',[])})
        for name,sha in replay_hashes.items():
            if not (ROOT/name).is_file() or hashlib.sha256((ROOT/name).read_bytes()).hexdigest()!=sha:raise SystemExit('Explicit licence recorded bytes differ: '+name)
        env['CONTEXTIR_LICENCE_RESOLUTIONS']=str(selected)
    result=subprocess.run([sys.executable,'-m','pytest','-q','-p',args.plugin],cwd=source,env=env,capture_output=True,text=True)
    report.with_suffix('.txt').write_text(result.stdout+result.stderr,encoding='utf-8')
    if not report.exists():raise SystemExit('pytest did not produce the report; inspect the retained text output')
    data=json.loads(report.read_text(encoding='utf-8'))
    if data['go_bridge_sha256']!=binary_hash:raise SystemExit('Bridge binary changed during the run')
    data.update(label=args.label,command='python -m pytest -q -p '+args.plugin)
    baseline=json.loads((ROOT/'conformance/reports/upstream-pytest.json').read_text(encoding='utf-8'))
    data['collection_matches_baseline']={i['nodeid'] for i in data['collected']}=={i['nodeid'] for i in baseline['collected']}
    report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    unchanged=all(path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest()==digest for relative,digest in {**adapter_hashes,**fixture_hashes,**replay_hashes}.items() for path in (ROOT/relative,))
    provenance.write_text(json.dumps({'report':str(report.relative_to(ROOT)),'report_sha256':hashlib.sha256(report.read_bytes()).hexdigest(),
        'bridge_binary_sha256':binary_hash,'verified_bridge_build_record':str(args.bridge_provenance.resolve()) if args.bridge_provenance else None,
        'verified_bridge_build':bridge_build,'source_snapshot_verified':bridge_build is not None,
        'adapter_source_archive':str(adapter_archive.relative_to(ROOT)),'adapter_source_archive_sha256':hashlib.sha256(adapter_archive.read_bytes()).hexdigest(),
        'adapter_source_sha256':adapter_hashes,'external_fixture_sha256':fixture_hashes,'licence_recorded_file_sha256':replay_hashes,'adapter_and_fixture_files_unchanged_during_run':unchanged,
        'licence_resolutions':str(args.licence_resolutions.resolve()) if args.licence_resolutions else None,'licence_resolutions_sha256':hashlib.sha256(args.licence_resolutions.read_bytes()).hexdigest() if args.licence_resolutions else None,
        'scope':'Original source tests remain pinned and unchanged. Python adapter sources are archived; external fixture hashes and exact native binary are recorded. Unmatched source snapshots and failing assertions remain explicit.'},ensure_ascii=False,indent=2)+'\n')
    if not data['collection_matches_baseline']:raise SystemExit('Original nodeid collection changed; report retained but no parity claim is permitted')
    if not unchanged:raise SystemExit('Adapter or fixture changed during the run; all evidence retained but no stable-input acceptance claim is permitted')
    print(json.dumps({k:data[k] for k in ('counts','native_go_assertion_counts','native_go_not_covered_count')},ensure_ascii=False))
    raise SystemExit(result.returncode)

if __name__=='__main__':main()
