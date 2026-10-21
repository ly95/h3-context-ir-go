#!/usr/bin/env python3
"""Build the paid acceptance command solely from an immutable source archive.

This command never reads provider credentials or calls a model. Every run has a
new label, archive and build receipt; earlier rounds are never overwritten.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

from run_go_adapted import source_files

ROOT = Path(__file__).resolve().parents[1]


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def runtime_sources(directory, env):
    output = subprocess.check_output(
        ['go', 'list', '-deps', '-json', './cmd/contextir-live-eval'],
        cwd=directory, env=env, text=True,
    )
    decoder = json.JSONDecoder()
    offset = 0
    names = {'go.mod'}
    if (directory / 'go.sum').exists():
        names.add('go.sum')
    while offset < len(output):
        while offset < len(output) and output[offset].isspace():
            offset += 1
        if offset == len(output):
            break
        package, offset = decoder.raw_decode(output, offset)
        if not package.get('ImportPath', '').startswith('github.com/ly95/contextir'):
            continue
        base = Path(package['Dir']).resolve()
        if not base.is_relative_to(directory):
            raise SystemExit('runtime package resolves outside frozen source')
        for name in package.get('GoFiles', []) + package.get('EmbedFiles', []):
            path = (base / name).resolve()
            if not path.is_relative_to(directory):
                raise SystemExit('runtime input resolves outside frozen source')
            names.add(str(path.relative_to(directory)))
    return {name: digest(directory / name) for name in sorted(names)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--label', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*', args.label):
        parser.error('label must be one filename component')
    base = ROOT / 'conformance/reports/builds'
    base.mkdir(parents=True, exist_ok=True)
    report = base / (args.label + '-live-build.json')
    archive = base / (args.label + '-live-sources.tar.gz')
    if report.exists() or archive.exists():
        parser.error('refusing to overwrite earlier source/build evidence')
    sources = source_files()
    if 'cmd/contextir-live-eval/main.go' not in sources:
        parser.error('native live acceptance command is missing')
    env = dict(os.environ, GOWORK='off', GOTOOLCHAIN='local', GOPROXY='off', GOSUMDB='off')
    for key in ('DEEPSEEK_API_KEY', 'OPENAI_API_KEY', 'ANTHROPIC_API_KEY'):
        env.pop(key, None)
    environment = subprocess.check_output(
        ['go', 'env', '-json', 'GOOS', 'GOARCH', 'GOVERSION', 'GOROOT', 'GOTOOLCHAIN', 'GOWORK'],
        cwd=ROOT, env=env, text=True,
    )
    with tarfile.open(archive, 'x:gz') as out:
        for name in sources:
            out.add(ROOT / name, arcname=name, recursive=False)
    directory = Path(tempfile.mkdtemp(prefix='contextir-live-build-' + args.label + '-')).resolve()
    with tarfile.open(archive) as saved:
        saved.extractall(directory, filter='data')
    for name, want in sources.items():
        if digest(directory / name) != want:
            raise SystemExit('archived source mismatch: ' + name)
    binary = directory / 'contextir-live-eval'
    command = ['go', 'build', '-trimpath', '-buildvcs=false', '-o', str(binary), './cmd/contextir-live-eval']
    run = subprocess.run(command, cwd=directory, env=env, capture_output=True, text=True)
    runtime_inputs = runtime_sources(directory, env) if run.returncode == 0 else {}
    stdout = base / (args.label + '-live-build.stdout.txt')
    stderr = base / (args.label + '-live-build.stderr.txt')
    with stdout.open('x') as out:
        out.write(run.stdout)
    with stderr.open('x') as out:
        out.write(run.stderr)
    archived_unchanged = all(digest(directory / name) == want for name, want in sources.items())
    working_unchanged = sources == source_files()
    runtime_unchanged = all((ROOT / name).is_file() and digest(ROOT / name) == want for name, want in runtime_inputs.items())
    value = {
        'schema': 'contextir-live-frozen-build/v1',
        'created_at': dt.datetime.now(dt.timezone.utc).isoformat(),
        'git_head': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
        'command': command,
        'exit_code': run.returncode,
        'passed': run.returncode == 0 and archived_unchanged and runtime_unchanged,
        'source_archive': str(archive.relative_to(ROOT)),
        'source_archive_sha256': digest(archive),
        'sources': sources,
        'runtime_sources': runtime_inputs,
        'verified_source_count': len(sources),
        'build_directory': str(directory),
        'binary': str(binary),
        'binary_sha256': digest(binary) if binary.exists() else None,
        'go_environment': json.loads(environment),
        'archived_sources_unchanged': archived_unchanged,
        'working_sources_unchanged': working_unchanged,
        'working_runtime_sources_unchanged': runtime_unchanged,
        'stdout': str(stdout.relative_to(ROOT)),
        'stdout_sha256': digest(stdout),
        'stderr': str(stderr.relative_to(ROOT)),
        'stderr_sha256': digest(stderr),
        'live_provider_calls': 0,
        'scope': 'The executable is built only from the verified archive and current runtime inputs must match it. Concurrent non-runtime documentation or consumer-tool edits are recorded separately by working_sources_unchanged. Paid run inputs, options and model calls have separate receipts.',
    }
    with report.open('x') as out:
        json.dump(value, out, ensure_ascii=False, indent=2)
        out.write('\n')
    print(json.dumps({'report': str(report.relative_to(ROOT)), 'passed': value['passed'], 'binary': str(binary), 'binary_sha256': value['binary_sha256']}))
    return 0 if value['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
