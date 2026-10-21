#!/usr/bin/env python3
"""Run one immutable, explicitly paid acceptance round with a local key source.

Only the explicit DeepSeek key is read from the named configuration file. It is
injected into the child environment, never copied to source or execution logs.
The production library and live Go command do not read configuration files.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--build-receipt', type=Path, required=True)
    parser.add_argument('--credential-file', type=Path, required=True)
    parser.add_argument('--label', required=True)
    parser.add_argument('--base-url', required=True)
    parser.add_argument('--model', required=True)
    parser.add_argument('--reasoning-effort', required=True)
    parser.add_argument('--max-output-tokens', type=int, required=True)
    parser.add_argument('--parallel', type=int, default=3)
    parser.add_argument('--max-structure-retries', type=int, choices=range(4), default=1)
    parser.add_argument('--max-planner-retries', type=int, choices=range(4), default=1)
    args = parser.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*', args.label):
        parser.error('label must be one filename component')
    if not 1 <= args.parallel <= 3 or args.max_output_tokens <= 0:
        parser.error('parallel must be 1..3 and output-token limit must be positive')
    build = json.loads(args.build_receipt.read_text())
    if not build.get('passed') or not build.get('working_runtime_sources_unchanged'):
        parser.error('a successful immutable runtime build receipt is required')
    directory = Path(build['build_directory']).resolve()
    binary = Path(build['binary']).resolve()
    if not binary.is_file() or digest(binary) != build['binary_sha256']:
        parser.error('the frozen executable is unavailable or changed')
    for name, want in build['runtime_sources'].items():
        if digest(directory / name) != want:
            parser.error('a frozen runtime input changed')
    fixture = directory / 'conformance/release_acceptance_inputs.json'
    assertions = directory / 'conformance/release_acceptance_assertions.json'
    for path in (fixture, assertions):
        name = str(path.relative_to(directory))
        if digest(path) != build['sources'].get(name):
            parser.error('frozen input/assertion evidence changed')
    config = {}
    for line in args.credential_file.read_text().splitlines():
        match = re.match(r'\s*(?:export\s+)?(DEEPSEEK_API_KEY|DEEPSEEK_BASE_URL)\s*=\s*(.*)', line)
        if not match:
            continue
        values = shlex.split(match.group(2), comments=True)
        if len(values) != 1:
            parser.error('explicit local DeepSeek configuration is malformed')
        config[match.group(1)] = values[0]
    if not config.get('DEEPSEEK_API_KEY') or config.get('DEEPSEEK_BASE_URL') != args.base_url:
        parser.error('key is missing or the explicit API endpoint differs from local configuration')
    base = ROOT / 'conformance/reports'
    output = base / args.label
    receipt = base / (args.label + '-execution.json')
    stdout = base / (args.label + '.stdout.txt')
    stderr = base / (args.label + '.stderr.txt')
    if any(p.exists() for p in (output, receipt, stdout, stderr)):
        parser.error('refusing to overwrite a previous paid round')
    command = [str(binary), '-input', str(fixture), '-assertions', str(assertions),
               '-out', str(output), '-base-url', args.base_url, '-model', args.model,
               '-api-key-env', 'DEEPSEEK_API_KEY', '-parallel', str(args.parallel),
               '-profile', 'official', '-reasoning-effort', args.reasoning_effort,
               '-max-output-tokens', str(args.max_output_tokens),
               '-max-structure-retries', str(args.max_structure_retries),
               '-max-planner-retries', str(args.max_planner_retries)]
    env = dict(os.environ, GOWORK='off')
    for key in ('DEEPSEEK_API_KEY', 'OPENAI_API_KEY', 'ANTHROPIC_API_KEY'):
        env.pop(key, None)
    env['DEEPSEEK_API_KEY'] = config['DEEPSEEK_API_KEY']
    config.clear()
    started = dt.datetime.now(dt.timezone.utc).isoformat()
    clock = time.monotonic()
    with stdout.open('x') as out, stderr.open('x') as err:
        process = subprocess.Popen(command, cwd=directory, env=env, stdout=subprocess.PIPE, stderr=err, text=True)
        for line in process.stdout:
            out.write(line)
            out.flush()
            print(line, end='', flush=True)
        code = process.wait()
    unchanged = all(digest(directory / name) == want for name, want in build['runtime_sources'].items())
    value = {
        'schema': 'contextir-paid-live-execution/v1',
        'started_at': started,
        'finished_at': dt.datetime.now(dt.timezone.utc).isoformat(),
        'elapsed_seconds': time.monotonic() - clock,
        'command': command,
        'exit_code': code,
        'runner_succeeded': code == 0,
        'build_receipt': str(args.build_receipt.resolve()),
        'build_receipt_sha256': digest(args.build_receipt),
        'binary_sha256': build['binary_sha256'],
        'input_sha256': digest(fixture),
        'assertions_sha256': digest(assertions),
        'requested_model': args.model,
        'reasoning_effort': args.reasoning_effort,
        'max_output_tokens': args.max_output_tokens,
        'max_structure_retries': args.max_structure_retries,
        'max_planner_retries': args.max_planner_retries,
        'base_url': args.base_url,
        'output_directory': str(output),
        'source_runtime_unchanged': unchanged,
        'stdout': str(stdout.relative_to(ROOT)),
        'stdout_sha256': digest(stdout),
        'stderr': str(stderr.relative_to(ROOT)),
        'stderr_sha256': digest(stderr),
        'credential_values_persisted': False,
        'key_environment': 'DEEPSEEK_API_KEY',
        'launcher_sha256': digest(Path(__file__)),
        'scope': 'Paid current-binary Compile/Refine run. Independent final-artifact and semantic verification is a separate report; successful execution alone is not acceptance.',
    }
    with receipt.open('x') as out:
        json.dump(value, out, ensure_ascii=False, indent=2)
        out.write('\n')
    print(json.dumps({'execution': str(receipt.relative_to(ROOT)), 'exit_code': code, 'runtime_unchanged': unchanged}), flush=True)
    return 0 if code == 0 and unchanged else 1


if __name__ == '__main__':
    raise SystemExit(main())
