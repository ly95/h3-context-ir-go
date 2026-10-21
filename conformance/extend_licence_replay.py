#!/usr/bin/env python3
"""Add explicit fresh original-API recordings to a historical strict replay index.

This tool never calls a provider or reads expected decisions. Earlier inputs,
results, failures and trace bytes remain unchanged. Each recording names its own
binary and verified source build; a mixed index makes no same-binary claim.
"""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import tarfile

from compose_licence_replay import source_key

ROOT = Path(__file__).resolve().parents[1]
PIN = 'fd031e136ae3d89147324c0d1b2aa65e838c21f5'


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def relative(path):
    return str(path.resolve().relative_to(ROOT))


def require(ok, message):
    if not ok:
        raise ValueError(message)


def checked(path, expected):
    path = Path(path)
    path = path if path.is_absolute() else ROOT / path
    require(path.is_file() and digest(path) == expected, 'Evidence bytes differ: ' + str(path))
    return path


def verify_build(path):
    build = json.loads(path.read_text())
    binary = checked(build['binary'], build['binary_sha256'])
    archive = checked(build['source_archive'], build['source_archive_sha256'])
    # The new helper receipt points to the complete immutable live source list;
    # historical receipts already carry their complete source list directly.
    sources = build.get('sources')
    if sources is None:
        source_receipt = checked(build['main_live_build_receipt'], build['main_live_build_receipt_sha256'])
        frozen = json.loads(source_receipt.read_text())
        require(build.get('shared_library_runtime_exactly_equal') is True, 'Helper did not verify the main library runtime')
        require(build.get('frozen_sources_unchanged_after_build') is True, 'Frozen helper sources changed during build')
        require(build['shared_library_sources'] == build['main_shared_library_sources'], 'Helper and main library sources differ')
        require(build['shared_library_sources'] == {name: sha for name, sha in frozen['runtime_sources'].items() if not name.startswith('cmd/')}, 'Helper shared sources do not match the main receipt')
        require(frozen['source_archive_sha256'] == build['source_archive_sha256'], 'Helper source archive differs from the main snapshot')
        sources = frozen['sources']
    seen = set()
    with tarfile.open(archive) as saved:
        for member in saved:
            if member.name not in sources:
                continue
            require(member.isfile() and member.name not in seen, 'Invalid/duplicate archived source: ' + member.name)
            stream = saved.extractfile(member)
            require(stream is not None and hashlib.sha256(stream.read()).hexdigest() == sources[member.name], 'Archived source differs: ' + member.name)
            seen.add(member.name)
    require(seen == set(sources), 'Source archive is incomplete')
    return {'build_receipt': relative(path), 'build_receipt_sha256': digest(path),
            'binary_sha256': digest(binary), 'source_archive': relative(archive),
            'source_archive_sha256': digest(archive), 'verified_source_count': len(sources)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-index', 'fixture', 'report', 'helper-build', 'historical-build', 'inputs', 'resolutions'):
        parser.add_argument('--' + name, type=Path, required=True)
    parser.add_argument('--name', required=True)
    args = parser.parse_args()
    require(args.name and ':' not in args.name and Path(args.name).name == args.name, 'A plain recording namespace is required')
    require(not args.inputs.exists() and not args.resolutions.exists(), 'Refusing to overwrite existing evidence')
    require(args.inputs.resolve().is_relative_to(ROOT / 'conformance/fixtures'), 'Replay inputs must be in conformance/fixtures')
    require(args.resolutions.resolve().is_relative_to(ROOT / 'conformance/fixtures'), 'Replay index must be in conformance/fixtures')
    base = json.loads(args.base_index.read_text())
    require(base['source_commit'] == PIN, 'Historical replay index is not pinned')
    base_inputs = checked(base['input_fixture'], base['input_fixture_sha256'])
    corpus = json.loads(base_inputs.read_text())
    require(corpus['source_commit'] == PIN, 'Historical input corpus is not pinned')
    historical = verify_build(args.historical_build)
    helper = verify_build(args.helper_build)
    for item in base['files']:
        checked(item['path'], item['sha256'])
    recordings = copy.deepcopy(base['recordings'])
    for item in recordings:
        require(item['name'] != args.name, 'Duplicate recording namespace')
        checked(item['input_fixture'], item['input_fixture_sha256'])
        report = json.loads(checked(item['report'], item['report_sha256']).read_text())
        require(report['Source'] == PIN and report['Kind'] == 'licence', 'Unexpected historical recording kind/source')
        require(report['executable_sha256'] == item['executable_sha256'] == historical['binary_sha256'], 'Historical report/build binary mismatch')
        item['verified_source_build'] = historical
    fresh = json.loads(args.fixture.read_text())
    report = json.loads(args.report.read_text())
    require(fresh['source_commit'] == report['Source'] == PIN and report['Kind'] == 'licence', 'Fresh input/report kind or source differs')
    require(report['executable_sha256'] == helper['binary_sha256'], 'Fresh recording/build binary differs')
    helper_receipt = json.loads(args.helper_build.read_text())
    require(checked(helper_receipt['fixture_file'], helper_receipt['fixture_sha256']).resolve() == args.fixture.resolve(), 'Fresh report uses a different helper fixture')
    config = helper_receipt['provider_configuration']
    require(report['Model'] == config['model'] and report['Reasoning'] == config['reasoning_effort'], 'Fresh requested provider/model configuration differs')
    actual = {row['case_id']: row for row in report['results']}
    expected = {row['case_id']: row for row in fresh['cases']}
    require(len(actual) == len(report['results']) and len(expected) == len(fresh['cases']) and set(actual) == set(expected), 'Fresh recording inventory differs')
    inputs, results, files = copy.deepcopy(corpus['cases']), copy.deepcopy(base['cases']), copy.deepcopy(base['files'])
    for case_id, item in expected.items():
        result = copy.deepcopy(actual[case_id])
        require(item['input_sha256'] == result['input_sha256'], 'Fresh original input hash differs')
        exact = json.dumps(item['input'], ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()
        require(hashlib.sha256(exact).hexdigest() == item['input_sha256'], 'Fresh fixture does not contain its exact original input')
        require(result['status'] == 'licence_decided' and not result.get('error') and not result.get('reused_from'), 'Fresh decision failed or silently reused another case')
        require(bool(result.get('calls')), 'Fresh decision lacks actual model calls')
        qualified = args.name + ':' + case_id
        inputs.append({**copy.deepcopy(item), 'case_id': qualified, 'source_case_id': case_id, 'source_run': args.name})
        result.update(case_id=qualified, source_case_id=case_id, trace_case_id=case_id, run_report=relative(args.report))
        results.append(result)
        for index, call in enumerate(result['calls'], 1):
            require(call['stage'] in ('rendering_permission', 'medium', 'identity_requirements', 'licence') and call['usage_valid'] and call.get('model') and call['usage']['input_tokens'] >= 0 and 0 <= call['usage']['output_tokens'] <= config['max_output_tokens'], 'Fresh call stage/model/usage is invalid')
            trace = args.report.parent / case_id
            require((trace / f'call-{index:02d}-request.json').is_file(), 'Fresh request is missing')
            output = trace / f'call-{index:02d}-output.json'
            if not output.is_file():
                require((trace / f'call-{index:02d}-rejected.txt').is_file() and call.get('error', '').startswith('contextir: invalid_model_output at deepseek.response ') and call.get('diagnostics'), 'Fresh rejected candidate lacks its complete provider rejection')
        for path in sorted((args.report.parent / case_id).glob('*')):
            require(path.is_file(), 'Unexpected fresh trace directory')
            files.append({'path': relative(path), 'sha256': digest(path)})
    require(inputs[:len(corpus['cases'])] == corpus['cases'] and results[:len(base['cases'])] == base['cases'], 'Historical entries changed')
    recordings.append({'name': args.name, 'input_fixture': relative(args.fixture), 'input_fixture_sha256': digest(args.fixture),
                       'report': relative(args.report), 'report_sha256': digest(args.report), 'executable_sha256': helper['binary_sha256'],
                       'entries': len(expected), 'failed_entries_preserved': [], 'verified_source_build': helper})
    by_id = {row['case_id']: row for row in results}
    require(len(by_id) == len(results), 'Duplicate namespaced replay case')
    selected = {source_key(row['input']): row['case_id'] for row in inputs}
    for case_id in selected.values():
        visited = set()
        while by_id[case_id].get('reused_from'):
            require(case_id not in visited, 'Replay reuse cycle')
            visited.add(case_id)
            case_id = by_id[case_id]['reused_from']
            require(case_id in by_id, 'Replay reuse leaves this explicit index')
        require(by_id[case_id]['status'] == 'licence_decided', 'Final selected replay decision failed')
    new_inputs = {'source_commit': PIN, 'scope': 'Exact original inputs; historical entries retained and explicit later same-input recordings selected. No expected output is model input.', 'cases': inputs, 'recordings': recordings}
    args.inputs.write_text(json.dumps(new_inputs, ensure_ascii=False, indent=2) + '\n')
    index = {'source_commit': PIN, 'schema': 'contextir-explicit-licence-replay/v2',
             'input_fixture': relative(args.inputs), 'input_fixture_sha256': digest(args.inputs),
             'binary_policy': 'per-recording binary and verified source build; historical recordings are not relabelled as fresh current-binary calls',
             'selection_policy': 'last explicitly listed successful recording for the exact complete normalized source input; prior failures and reuse chains retained',
             'historical_base_index': relative(args.base_index), 'historical_base_index_sha256': digest(args.base_index),
             'recordings': recordings, 'cases': results, 'files': files, 'selected_input_count': len(selected),
             'scope': 'Deterministic strict replay only. Every actual request, schema and output must match before a recorded response is consumed. Fresh original-API supplement is separate from main full-Compile acceptance.'}
    args.resolutions.write_text(json.dumps(index, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'inputs': relative(args.inputs), 'resolutions': relative(args.resolutions), 'historical_entries_unchanged': len(base['cases']), 'new_unique_inputs': len(expected), 'selected_unique_inputs': len(selected)}))


if __name__ == '__main__':
    main()
