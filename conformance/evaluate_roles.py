#!/usr/bin/env python3
"""Independently evaluate final ROLE_CASES Documents/Artifacts, never a model.

Original values remain exact comparisons. Native socket/time/anchor-subject
representations are recorded as explicit adapter differences, not erased. Typed
use proves a binding, not its prose meaning or a rendered video's fidelity.
"""
import argparse
from collections import Counter
from fractions import Fraction
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
VERSION = 'contextir-role-evaluator/v4'
UPSTREAM_SHA = 'fd031e136ae3d89147324c0d1b2aa65e838c21f5'
ROLE_WIRE = {
    'subject': 'identity', 'environment': 'scene', 'style': 'style',
    'structure': 'structure', 'storyboard': 'storyboard',
    'frame_anchor_first': 'first_frame', 'frame_anchor_last': 'last_frame',
    'edit_source': 'edit_source', 'continuation_source': 'continuation_source',
    'voice_timbre': 'voice', 'bgm': 'music', 'music_style': 'music_style',
    'beat_reference': 'beat_reference', 'sfx': 'sfx',
    'placed_subject': 'placed_subject', 'replacement_subject': 'replacement_subject',
}
REFERENCE_RETENTION = {
    'style': 'weak_reference', 'structure': 'weak_reference', 'storyboard': 'weak_reference',
    'frame_anchor_first': 'fully_preserved', 'frame_anchor_last': 'fully_preserved',
    'edit_source': 'fully_preserved', 'continuation_source': 'partially_preserved',
    'voice_timbre': 'reference', 'bgm': 'partially_copy', 'music_style': 'reference',
    'beat_reference': 'reference', 'sfx': 'reference',
    'placed_subject': 'fully_preserved', 'replacement_subject': 'fully_preserved',
}
NON_ACTOR_ROLES = {'style', 'structure', 'storyboard', 'voice_timbre', 'bgm', 'music_style', 'beat_reference', 'sfx'}
BASE_MODES = {'i2va', 'l2va', 'fl2va'}


def sequence(value):
    return value or []


def add(row, field, expected, actual, scope, category='original_contract', adapter=None):
    check = dict(field=field, expected=expected, actual=actual, matched=expected == actual, scope=scope, category=category)
    row['checks'].append(check)
    if not check['matched'] and adapter:
        # Keeping matched=False is intentional: adapter differences never become
        # an assertion that the original field was equal.
        check['explicit_adapter_difference'] = adapter
        row['adapter_differences'].append(dict(field=field, original=expected, native=actual, **adapter))
    return check['matched']


def adapter(kind, explanation):
    return dict(kind=kind, explanation=explanation)


def actual_uses(document):
    """Only typed narrative references count; never keyword-match literal prose."""
    subject_shots, reference_shots = {}, {}
    speakers = {x['id']: x for x in sequence(document.get('speakers'))}
    lines = {x['id']: x for x in sequence(document.get('dialogue'))}
    for shot in sequence(document.get('shots')):
        texts = [(sequence(shot.get('description')), True)]
        for event in sequence(shot.get('events')):
            texts.append((sequence(event.get('description')), event.get('kind') in {'visual', 'action', 'camera', 'visible_text'}))
            part = event.get('dialogue')
            if part:
                for sid in sequence(lines.get(part.get('line_id'), {}).get('speaker_ids')):
                    speaker = speakers.get(sid, {})
                    line = lines.get(part.get('line_id'), {})
                    visible = speaker.get('onscreen') is True or speaker.get('onscreen') is None and not line.get('voiceover', False)
                    if speaker.get('subject_id') and visible:
                        subject_shots.setdefault(speaker['subject_id'], set()).add(shot['id'])
            cue = event.get('audio_cue')
            if cue:
                reference_shots.setdefault(cue['reference_id'], set()).add(shot['id'])
        for text, visual in texts:
            for span in text:
                if span.get('subject_id') and visual:
                    subject_shots.setdefault(span['subject_id'], set()).add(shot['id'])
                if span.get('reference_id'):
                    reference_shots.setdefault(span['reference_id'], set()).add(shot['id'])
    return subject_shots, reference_shots


def visual_reference_uses(document, reference_id):
    uses = []
    for span in sequence(document.get('summary')):
        if span.get('reference_id') == reference_id:
            uses.append('summary')
    for shot in sequence(document.get('shots')):
        for span in sequence(shot.get('description')):
            if span.get('reference_id') == reference_id:
                uses.append(shot['id'] + '.description')
        for n, event in enumerate(sequence(shot.get('events'))):
            if event.get('kind') == 'camera':
                continue
            for span in sequence(event.get('description')):
                if span.get('reference_id') == reference_id:
                    uses.append(f'{shot["id"]}.events[{n}].description')
    return uses


def rational_ms(target):
    value = Fraction(target['frames'] * 1000, target['fps'])
    return value.numerator // value.denominator, (2 * value.numerator + value.denominator) // (2 * value.denominator)


def evaluate_case(item, expectation, actual):
    oracle = expectation['expected']
    row = {k: item[k] for k in ('case_id', 'nodeid', 'input_sha256')}
    row.update(actual_status=actual.get('status'), checks=[], adapter_differences=[], human_review_required=True,
               semantic_status='not_determined_by_structural_evaluator',
               original_explicit_draft_mode=oracle['mode'],
               actual_input_brief_mode=item['input']['brief'].get('mode'))
    add(row, 'input.sha256', expectation['input_sha256'], actual.get('input_sha256'), 'Actual original request must match the independent held-out case.', 'native_integrity')
    if actual.get('status') != 'compiled':
        row.update(status='native_compile_failed', native_contract_status='failed', original_field_status='not_evaluable', error=actual.get('error'), issues=actual.get('issues'))
        return row
    native = actual.get('result') or {}
    d, a = native.get('document') or {}, native.get('artifact') or {}
    if not d.get('shots') or not a.get('prompt') or not a.get('sha256'):
        add(row, 'complete.final_artifact', True, False, 'A status string alone is not a completed artifact.', 'native_integrity')
        row.update(status='incomplete_final_artifact', native_contract_status='failed', original_field_status='not_evaluable')
        return row
    target = d.get('target') or {}
    add(row, 'result.status', 'compiled', native.get('status'), 'The inner lifecycle must also have completed.', 'native_integrity')
    add(row, 'document.locked_target', native.get('request', {}).get('target'), target, 'The writer did not alter the prepared request target.', 'native_integrity')
    add(row, 'document.locked_assets', sequence(native.get('request', {}).get('assets')), sequence(d.get('assets')), 'The writer did not substitute or drop original sources.', 'native_integrity')
    add(row, 'mode', oracle['mode'], d.get('mode'), 'Exact field comparison only: original deterministic_draft receives an explicit mode argument, while this additional Compile run receives the original Brief/cards. A difference is not a replayed original assertion failure and is never silently marked equal.')
    add(row, 'target.frames', oracle['target']['frames'], target.get('frame_count'), 'Exact original frame target.')
    add(row, 'target.canvas', oracle['target']['canvas'], [target.get('width'), target.get('height')], 'Exact original canvas target.')
    fps = target.get('fps') or {}
    actual_fps = Fraction(fps.get('numerator', 0), fps['denominator']) if fps.get('denominator', 0) > 0 else None
    add(row, 'target.fps', str(Fraction(oracle['target']['fps'])), str(actual_fps) if actual_fps is not None else None, 'Exact rational frame rate; a zero denominator is invalid.')
    floor_ms, nearest_ms = rational_ms(oracle['target'])
    ms_adapter = adapter('frame_duration_floor_to_nearest', 'Upstream endpoint truncates the frame-derived duration; native Time rounds to nearest millisecond. Both values are retained.') if target.get('duration_ms') == nearest_ms and floor_ms != nearest_ms else None
    add(row, 'target.duration_ms.original_floor', floor_ms, target.get('duration_ms'), 'Original frame-derived integer endpoint.', adapter=ms_adapter)
    add(row, 'target.duration_ms.native_nearest', nearest_ms, target.get('duration_ms'), 'The explicit native millisecond policy must hold exactly.', 'native_integrity')
    add(row, 'artifact.target', target, a.get('target'), 'Artifact and authoritative Document target must be identical.', 'native_integrity')
    add(row, 'artifact.mode', d.get('mode'), a.get('mode'), 'Artifact mode must preserve Document mode.', 'native_integrity')
    add(row, 'artifact.frames', oracle['target']['frames'], a.get('frames'), 'Final artifact frame count is not inferred from compiled status.')
    shots = sequence(d.get('shots'))
    shot_map = {x['id']: x for x in shots}
    shot_order = {x['id']: n + 1 for n, x in enumerate(shots)}
    add(row, 'timeline.starts_at_zero', 0, shots[0].get('start_ms'), 'Exact complete target timeline.', 'native_integrity')
    add(row, 'timeline.ends_at_target', target.get('duration_ms'), shots[-1].get('end_ms'), 'Exact complete target timeline.', 'native_integrity')
    for i in range(1, len(shots)):
        add(row, f'timeline.cut[{i}]', shots[i - 1].get('end_ms'), shots[i].get('start_ms'), 'No gap/overlap between final shots.', 'native_integrity')
    original_assets = item['input']['brief']['assets']
    native_assets = sequence(native.get('brief', {}).get('assets'))
    add(row, 'asset.count', len(original_assets), len(native_assets), 'No dropped/added source occurrence.', 'native_integrity')
    by_sha = {x['sha256']: native_assets[n]['id'] for n, x in enumerate(original_assets) if n < len(native_assets)}
    by_label = {x['label']: by_sha.get(x['sha256']) for x in oracle['references']}
    doc_assets = {x['id']: x for x in sequence(d.get('assets'))}
    refs = sequence(d.get('references'))
    subjects = sequence(d.get('subjects'))
    subject_map = {x['id']: x for x in subjects}
    media = sequence(a.get('manifest', {}).get('media'))
    roles_by_source = {}
    for subject in subjects:
        for source in sequence(subject.get('sources')):
            roles_by_source.setdefault(source['source']['asset_id'], set()).add(source['role'])
    for ref in refs:
        roles_by_source.setdefault(ref['source']['asset_id'], set()).update(sequence(ref.get('roles')))
    add(row, 'manifest.source_occurrences', sorted(by_sha.values()), sorted({x['source']['asset_id'] for x in media}), 'All original source IDs reach actual manifest media.', 'native_integrity')
    subject_uses, ref_uses = actual_uses(d)
    add(row, 'manifest.subject_ids', sorted(x['id'] for x in subjects), sorted(x['id'] for x in sequence(a.get('manifest', {}).get('subjects'))), 'Every target subject has exactly one manifest label.', 'native_integrity')
    add(row, 'manifest.reference_ids', sorted(x['id'] for x in refs), sorted(x['id'] for x in sequence(a.get('manifest', {}).get('references'))), 'Every Reference has exactly one manifest label.', 'native_integrity')
    known_refs = {x['id']: x for x in refs}
    for m in media:
        for use in sequence(m.get('target_uses')):
            ref = known_refs.get(use.get('reference_id'))
            add(row, 'manifest.use_owner.' + str(use.get('reference_id')), ref['source'] if ref else None, m['source'], 'No orphan or misrouted manifest target use.', 'native_integrity')
    for source in oracle['references']:
        sid, role = by_sha.get(source['sha256']), source['role']
        prefix = 'source.' + source['sha256']
        add(row, prefix + '.document_asset', True, sid in doc_assets, 'The output retains each supplied source occurrence.', 'native_integrity')
        add(row, prefix + '.typed_roles', [ROLE_WIRE[role]], sorted(roles_by_source.get(sid, set())), f'Explicit source role mapping: {role} -> {ROLE_WIRE[role]}; no unlicensed extra responsibility.')
        track = 'audio' if role in {'voice_timbre', 'bgm', 'music_style', 'beat_reference', 'sfx'} else 'visual'
        bindings = [x for x in media if x['source']['asset_id'] == sid and x['source'].get('track') == track]
        add(row, prefix + '.manifest_binding_count', 1, len(bindings), 'A physical source track has exactly one conditioning binding.', 'native_integrity')
        expected_socket = source['wiring']
        native_socket = expected_socket
        if oracle['mode'] in BASE_MODES and role in {'frame_anchor_first', 'frame_anchor_last'}:
            native_socket = 'first_frame' if role == 'frame_anchor_first' else 'last_frame'
        for binding in bindings:
            socket_adapter = adapter('base_checkpoint_socket_correction', 'The original manifest used ref_image_N for an explicit base frame; native base checkpoint requires first_frame/last_frame. Mode/role/retention/anchor checks remain mandatory.') if native_socket != expected_socket and binding.get('socket') == native_socket else None
            add(row, prefix + '.socket.original', expected_socket, binding.get('socket'), 'Exact retained original socket.', adapter=socket_adapter)
            add(row, prefix + '.socket.native', native_socket, binding.get('socket'), 'Explicit checkpoint socket policy.', 'native_integrity')
        source_refs = [x for x in refs if x['source']['asset_id'] == sid and x['source'].get('track') == track]
        actor_sources = [x['id'] for x in subjects if any(y['source']['asset_id'] == sid for y in sequence(x.get('sources')))]
        if role in NON_ACTOR_ROLES:
            add(row, prefix + '.observed_actor_sources', [], actor_sources, 'Property/structure/storyboard sources cannot supply observed target actor identities.', 'native_integrity')
        if role in REFERENCE_RETENTION:
            add(row, prefix + '.reference_retention', [REFERENCE_RETENTION[role]], [x.get('retention') for x in source_refs], 'Reference retention follows the independent role contract, including copy versus property.')
        if role in {'frame_anchor_first', 'frame_anchor_last'}:
            want_native = 0 if role == 'frame_anchor_first' else nearest_ms
            want_original = 0 if role == 'frame_anchor_first' else floor_ms
            for ref in source_refs:
                diff = ms_adapter if ref.get('anchor_ms') == want_native and want_original != want_native else None
                add(row, prefix + '.anchor_ms.original', want_original, ref.get('anchor_ms'), 'Original precise frame endpoint.', adapter=diff)
                add(row, prefix + '.anchor_ms.native', want_native, ref.get('anchor_ms'), 'Native anchor endpoint is exact.', 'native_integrity')
                owner = shots[0]['id'] if role == 'frame_anchor_first' else shots[-1]['id']
                add(row, prefix + '.anchor_shot', [owner], sequence(ref.get('shot_ids')), 'The actual first/last frame is bound to the containing shot.', 'native_integrity')
        if role in {'structure', 'storyboard'}:
            for ref in source_refs:
                add(row, prefix + '.visual_scene_reference_uses', [], visual_reference_uses(d, ref['id']), 'A camera/storyboard ID cannot stand in for source setting or actor in narrative spans.', 'native_integrity')
        row.setdefault('source_uses', []).append(dict(source_sha=source['sha256'], native_asset_id=sid, original_role=role, native_role=ROLE_WIRE[role], references=source_refs, observed_subject_ids=actor_sources, actual_reference_narrative_shots={x['id']: sorted(ref_uses.get(x['id'], set()), key=shot_order.get) for x in source_refs}))
    # Each reference's artifact use must preserve its complete typed scope and
    # relationship, independently of a declaration that it exists in Document.
    for ref in refs:
        bindings = [m for m in media if m['source'] == ref['source']]
        uses = [u for m in bindings for u in sequence(m.get('target_uses')) if u.get('reference_id') == ref['id']]
        add(row, 'artifact.use.' + ref['id'] + '.count', 1, len(uses), 'Every final Reference has a real manifest target-use record.', 'native_integrity')
        for use in uses:
            for field in ('roles', 'retention', 'anchor_ms', 'speaker_id', 'replaces', 'paired_video_id'):
                default = [] if field == 'roles' else '' if field in {'speaker_id', 'replaces', 'paired_video_id'} else None
                add(row, 'artifact.use.' + ref['id'] + '.' + field, ref.get(field, default), use.get(field, default), 'Artifact preserves the final Document reference exactly.', 'native_integrity')
            declared = sequence(ref.get('shot_ids'))
            for s in sequence(use.get('shots')):
                actual_shot = shot_map.get(s['id'])
                add(row, 'artifact.use.' + ref['id'] + '.' + s['id'] + '.interval', {'start_ms': actual_shot['start_ms'], 'end_ms': actual_shot['end_ms']} if actual_shot else None, s.get('interval'), 'Actual target shot interval, not merely a claimed label.', 'native_integrity')
            if declared:
                add(row, 'artifact.use.' + ref['id'] + '.shot_ids', declared, [x['id'] for x in sequence(use.get('shots'))], 'Declared use scope is materially represented.', 'native_integrity')
        if 'voice' in sequence(ref.get('roles')):
            speaker = next((s for s in sequence(d.get('speakers')) if s['id'] == ref.get('speaker_id')), None)
            add(row, 'voice.' + ref['id'] + '.reciprocal', ref['id'], speaker.get('voice_reference_id') if speaker else None, 'Voice references require an actual reciprocal target-speaker binding.', 'native_integrity')
    labels = {x['label']: x['id'] for x in sequence(a.get('manifest', {}).get('subjects'))}
    native_to_label = {v: k for k, v in labels.items()}
    for expected in oracle['subjects']:
        actual_subject = subject_map.get(labels.get(expected['label']))
        prefix = 'subject.' + expected['label']
        source_ids = [by_label[x] for x in expected['sources']]
        anchor_representation = (oracle['mode'] in BASE_MODES and d.get('mode') == oracle['mode'] and actual_subject is None and all(any(r['source']['asset_id'] == sid and r.get('retention') == 'fully_preserved' and r.get('anchor_ms') is not None for r in refs) for sid in source_ids))
        rep_adapter = adapter('base_identity_carried_by_actual_frame', 'The base Document carries identity/pose through the fully preserved actual image anchor instead of a separate subject definition. No literal-prose identity equivalence or rendered fidelity is inferred.') if anchor_representation else None
        add(row, prefix + '.exists', True, actual_subject is not None, 'Original stable subject occurrence.', adapter=rep_adapter)
        if actual_subject is None:
            row.setdefault('human_review_notes', []).append(prefix + ': no separate subject; exact image/body identity remains a human review requirement.')
            continue
        for field in ('kind', 'descriptor', 'retention'):
            add(row, prefix + '.' + field, expected[field], actual_subject.get(field, ''), 'Exact original structured subject field.')
        for field in ('attributes', 'pose'):
            add(row, prefix + '.' + field, expected[field], sequence(actual_subject.get(field)), 'No identity/pose facts are silently discarded.')
        add(row, prefix + '.pose_licensed', expected['pose_licensed'], actual_subject.get('pose_licensed', False), 'Pose authority is a structured role property.')
        actual_sources = sorted({x['source']['asset_id'] for x in sequence(actual_subject.get('sources'))})
        add(row, prefix + '.source_ids', sorted(source_ids), actual_sources, 'Subject provenance refers to the original source occurrence.', 'native_integrity')
        add(row, prefix + '.taken_over_by', expected['taken_over_by'], native_to_label.get(actual_subject.get('taken_over_by'), ''), 'Replacement retains the same source/target identity relationship.')
        actual_shots = sorted(subject_uses.get(actual_subject['id'], set()), key=shot_order.get)
        declared = sequence(actual_subject.get('shot_ids'))
        original_shots = sequence(expectation.get('expected_original_plan', {}).get('shots'))
        if set(expected['appears_in']) == set(range(1, len(original_shots) + 1)):
            expected_native_scope = [s['id'] for s in shots]
        else:
            expected_native_scope = [shots[n - 1]['id'] for n in expected['appears_in'] if 0 < n <= len(shots)]
        add(row, prefix + '.declared_scope', expected_native_scope, declared, 'Exact comparison to original automatic plan scope, not a claim that every preserved actor must appear in every new unpinned composition.', 'original_contract')
        if expected['taken_over_by']:
            add(row, prefix + '.removed_actor_actual_uses', [], actual_shots, 'A replaced actor contributes relation/transfer metadata, not its old visible appearance.', 'native_integrity')
        else:
            # Keep the old auto-plan scope comparison above unequal when it
            # differs. Independently compare the actual final scope to actual
            # typed uses; moving an unpinned cut does not create an obligation
            # to show every source actor in every resulting shot.
            add(row, prefix + '.actual_uses_cover_declared_shots', sorted(declared or [s['id'] for s in shots], key=shot_order.get), actual_shots, 'Final scope metadata must agree with actual target description/event/speaker uses; original automatic shot allocation is compared separately.', 'native_integrity')
            add(row, prefix + '.actual_use_exists', True, bool(actual_shots), 'Placed/preserved subjects must occur in an actual scene body, not only definitions.', 'native_integrity')
        row.setdefault('subject_uses', []).append(dict(original_label=expected['label'], native_id=actual_subject['id'], original_shot_indices=expected['appears_in'], declared_native_shots=declared, actual_native_shots=actual_shots, removed=bool(expected['taken_over_by'])))
    native_ok = all(x['matched'] for x in row['checks'] if not x.get('explicit_adapter_difference'))
    original_ok = all(x['matched'] for x in row['checks'])
    row.update(native_contract_status='passed' if native_ok else 'failed', original_field_status='all_equal' if original_ok else 'difference',
               status='all_checked_fields_match' if original_ok else 'contract_checks_passed_with_adapter_differences' if native_ok else 'heldout_field_difference',
               original_task_types=oracle['task_types'], final_prompt=a['prompt'])
    return row


def evaluate(inputs, expected, report):
    expected_by_id = {x['case_id']: x for x in expected['cases']}
    results = {x['case_id']: x for x in report['results']}
    ids = {x['case_id'] for x in inputs['cases']}
    if inputs['source_commit'] != UPSTREAM_SHA or len(ids) != 16 or len(inputs['cases']) != 16 or len(expected['cases']) != 16 or report.get('Source') != inputs['source_commit'] or expected['source_commit'] != inputs['source_commit'] or report.get('Kind') != 'roles' or set(results) != ids or set(expected_by_id) != ids or len(report['results']) != len(ids):
        raise ValueError('Expected one complete, pinned original ROLE_CASES run without duplicated, missing or extra cases')
    for item in inputs['cases']:
        digest = hashlib.sha256(json.dumps(item['input'], ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
        if item['input_sha256'] != digest or expected_by_id[item['case_id']]['input_sha256'] != digest:
            raise ValueError('Original request bytes or independent input hash changed: ' + item['case_id'])
    rows = [evaluate_case(item, expected_by_id[item['case_id']], results[item['case_id']]) for item in inputs['cases']]
    return dict(evaluator_version=VERSION, source_commit=inputs['source_commit'], actual_executable_sha256=report['executable_sha256'],
                scope='Independent held-out structural comparisons, not semantic or rendered-video acceptance. Original differences remain explicit. Role Cards are synthetic upstream inputs, not fresh real video/audio observations.',
                comparison_entrypoints=dict(original='h3ir.draft.deterministic_draft(brief, mode, cards, opts)', actual='compiler.Compile(Brief, synthetic Cards)',
                                            same_entrypoint=False, omitted_original_call_argument='mode',
                                            interpretation='The fixture preserves the Brief/cards exactly, but not the original separate mode argument. These live runs exercise an additional production pipeline. Exact field differences remain recorded; neither equality nor difference counts as executing the original draft assertion.'),
                exact_original_fields_all_equal=all(x['original_field_status'] == 'all_equal' for x in rows), native_contract_checks_all_passed=all(x['native_contract_status'] == 'passed' for x in rows),
                counts=dict(Counter(x['status'] for x in rows)), native_contract_counts=dict(Counter(x['native_contract_status'] for x in rows)),
                adapter_difference_counts=dict(Counter(a['kind'] for x in rows for a in x['adapter_differences'])), cases=rows)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', type=Path, required=True)
    parser.add_argument('--label', required=True)
    parser.add_argument('--gate', choices=['original', 'native'], default='original', help='Default requires exact original values. Native explicitly permits only listed adapter representation differences, never role/retention/use differences.')
    args = parser.parse_args()
    if Path(args.label).name != args.label or args.label in ('', '.', '..'):
        raise SystemExit('label must be a filename component')
    destination = ROOT / f'conformance/reports/roles-heldout-{args.label}.json'
    if destination.exists():
        raise SystemExit('Refusing to overwrite an earlier evaluation')
    input_path, expected_path = (ROOT / 'conformance/fixtures' / name for name in ('role_briefs.json', 'role_expectations.json'))
    report_path = args.run / 'report.json' if args.run.is_dir() else args.run
    output = evaluate(json.loads(input_path.read_text()), json.loads(expected_path.read_text()), json.loads(report_path.read_text()))
    output.update(evaluator_source_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), actual_report=str(report_path.resolve()), actual_report_sha256=hashlib.sha256(report_path.read_bytes()).hexdigest(),
                  inputs_sha256=hashlib.sha256(input_path.read_bytes()).hexdigest(), expectations_sha256=hashlib.sha256(expected_path.read_bytes()).hexdigest(), gate=args.gate)
    output['gate_passed'] = output['exact_original_fields_all_equal'] if args.gate == 'original' else output['native_contract_checks_all_passed']
    destination.write_text(json.dumps(output, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'report': str(destination.relative_to(ROOT)), 'counts': output['counts'], 'native_contract_counts': output['native_contract_counts'], 'adapter_difference_counts': output['adapter_difference_counts'], 'gate': args.gate, 'gate_passed': output['gate_passed']}))
    return 0 if output['gate_passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
