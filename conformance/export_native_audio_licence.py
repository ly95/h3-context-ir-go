#!/usr/bin/env python3
"""Export actual native audio adapter packets, never held-out expected outputs."""
from collections import defaultdict
import argparse
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def digest(value):
    return hashlib.sha256(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def differences(left, right, path=''):
    if isinstance(left, dict) and isinstance(right, dict):
        rows = []
        for key in sorted(left.keys() | right.keys()):
            field = f'{path}.{key}' if path else key
            if key not in left or key not in right:
                rows.append({'field': field, 'source_present': key in left, 'native_present': key in right, 'source': left.get(key), 'native': right.get(key)})
            else:
                rows.extend(differences(left[key], right[key], field))
        return rows
    if left != right:
        return [{'field': path, 'source': left, 'native': right}]
    return []


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--packets',type=Path,default=ROOT/'conformance/reports/go-ca28-native-audio-packets.json')
    parser.add_argument('--output',type=Path,default=ROOT/'conformance/fixtures/native_audio_licence_inputs.json')
    parser.add_argument('--case-prefix',default='native-audio')
    parser.add_argument('--expected-count',type=int,default=9)
    args=parser.parse_args()
    if Path(args.case_prefix).name!=args.case_prefix or args.expected_count<1:raise SystemExit('Choose a filename-component case prefix and a positive expected count')
    source_path = ROOT / 'conformance/fixtures/licence_inputs.json'
    packet_path = args.packets.resolve()
    source = json.loads(source_path.read_text())
    captured = json.loads(packet_path.read_text())
    grouped = defaultdict(list)
    for item in source['cases']:
        grouped[item['nodeid']].append(item)
    cases = []
    for number, packet in enumerate(captured['semantic_missing_actual_inputs'], 1):
        # Match the originating source call by unchanged Brief and all semantic
        # card fields. These are explicit DTO/provenance differences, not input
        # rewriting: the emitted input below retains the actual native values.
        matches = {}
        for item in grouped[packet['nodeid']]:
            changes = differences(item['input'], packet['arguments'])
            valid = True
            for change in changes:
                name = change['field'].rsplit('.', 1)[-1]
                if name in ('subjects', 'palette', 'visible_text'):
                    valid = change['source'] == [] and change['native'] is None
                elif name == 'composition':
                    valid = change['source'] == 'unknown' and change['native'] == ''
                elif name == 'analyzer_version':
                    valid = change['source'] == '5' and change['native'] == 'contextir-media/v1/caller-metadata'
                else:
                    valid = False
                if not valid:
                    break
            if valid:
                matches.setdefault(item['input_sha256'], []).append(item)
        if len(matches) != 1:
            raise SystemExit('Native packet does not identify one exact originating source input: ' + packet['nodeid'])
        source_hash, originals = next(iter(matches.items()))
        arguments = packet['arguments']
        cases.append({
            'case_id': f'{args.case_prefix}-{number:03d}', 'nodeid': packet['nodeid'], 'function': packet['function'],
            'source_case_ids': [x['case_id'] for x in originals], 'source_input_sha256': source_hash,
            'input_sha256': digest(arguments), 'input': arguments,
            'field_differences': differences(originals[0]['input'], arguments),
            'scope': 'Original unchanged test Brief and caller audio metadata after the actual deterministic Go audio adapter. Native values are retained exactly, including analyzer provenance. This input needs its own actual model call.',
        })
    if len(cases) != args.expected_count or len({x['input_sha256'] for x in cases}) != args.expected_count:
        raise SystemExit('Expected the declared number of distinct actual packets from the recorded original nodeids')
    output = {
        'source_commit': source['source_commit'], 'kind': 'licence',
        'source_fixture': str(source_path.relative_to(ROOT)), 'source_fixture_sha256': hashlib.sha256(source_path.read_bytes()).hexdigest(),
        'native_packet_report': str(packet_path.relative_to(ROOT)), 'native_packet_report_sha256': hashlib.sha256(packet_path.read_bytes()).hexdigest(),
        'go_bridge_sha256': captured['go_bridge_sha256'],
        'warning': 'Supplemental actual inputs only. No expected result, original decision, or held-out answer is included. The deterministic adapter provenance is not normalized away.',
        'cases': cases,
    }
    destination = args.output
    text = json.dumps(output, ensure_ascii=False, indent=2) + '\n'
    if destination.exists() and destination.read_text() != text:
        raise SystemExit('Refusing to overwrite an earlier native-input export')
    destination.write_text(text)
    print(f'Exported {len(cases)} distinct actual native audio packets with original hashes and exact field differences')


if __name__ == '__main__':
    main()
