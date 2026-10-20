#!/usr/bin/env python3
"""Build the native assertion bridge from a verified immutable source snapshot."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--label', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*', args.label):
        raise SystemExit('label must be one filename component')
    record_path = ROOT / f'conformance/reports/builds/{args.label}-bridge.json'
    if record_path.exists():
        raise SystemExit('Refusing to overwrite a bridge build record')
    manifest_path = args.manifest.resolve()
    manifest = json.loads(manifest_path.read_text())
    archive_path = ROOT / manifest['source_archive']
    archive_hash = hashlib.sha256(archive_path.read_bytes()).hexdigest()
    if archive_hash != manifest['source_archive_sha256']:
        raise SystemExit('Source archive differs from the immutable manifest')
    environment = subprocess.check_output(['go', 'env', 'GOOS', 'GOARCH', 'GOVERSION'], text=True).splitlines()
    if environment != manifest['go_environment']:
        raise SystemExit('Recorded Go toolchain/OS/architecture required for this immutable build proof')
    directory = Path(tempfile.mkdtemp(prefix=f'contextir-frozen-bridge-{args.label}-'))
    with tarfile.open(archive_path) as archive:
        archive.extractall(directory, filter='data')
    for relative, want in manifest['sources'].items():
        path = directory / relative
        if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != want:
            raise SystemExit('Archived source mismatch: ' + relative)
    binary = directory / 'bridge'
    command = ['go', 'build', '-trimpath', '-buildvcs=false', '-o', str(binary), './conformance/bridge']
    subprocess.run(command, cwd=directory, check=True)
    record = {
        'frozen_eval_binary_sha256': manifest['binary_sha256'],
        'frozen_source_manifest': str(manifest_path), 'frozen_source_manifest_sha256': hashlib.sha256(manifest_path.read_bytes()).hexdigest(),
        'source_archive': str(archive_path), 'source_archive_sha256': archive_hash,
        'verified_source_count': len(manifest['sources']), 'build_command': command,
        'build_directory': str(directory), 'bridge_binary': str(binary),
        'bridge_binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'go_environment': environment,
        'scope': 'The bridge is built solely from all hash-verified files in the immutable source snapshot. Python assertion adapters and external replay fixtures are recorded separately by run_go_core.py.',
    }
    record_path.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'record': str(record_path.relative_to(ROOT)), 'binary': str(binary), 'sha256': record['bridge_binary_sha256']}))


if __name__ == '__main__':
    main()
