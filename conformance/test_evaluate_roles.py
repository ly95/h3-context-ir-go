"""Meaningful evaluator regressions using the unchanged archived ROLE_CASES run."""
import copy
import json
import unittest

from evaluate_roles import ROOT, evaluate, evaluate_case


class RoleEvaluatorTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.inputs = json.loads((ROOT / 'conformance/fixtures/role_briefs.json').read_text())
        cls.expected = json.loads((ROOT / 'conformance/fixtures/role_expectations.json').read_text())
        cls.archived_run = json.loads((ROOT / 'conformance/reports/live-roles-round1/report.json').read_text())
        cls.by_input = {x['case_id']: x for x in cls.inputs['cases']}
        cls.by_expected = {x['case_id']: x for x in cls.expected['cases']}
        cls.by_result = {x['case_id']: x for x in cls.archived_run['results']}

    def case(self, case_id, actual=None):
        return evaluate_case(self.by_input[case_id], self.by_expected[case_id], actual or self.by_result[case_id])

    def test_complete_archived_run_is_not_compiled_equals_pass(self):
        report = evaluate(self.inputs, self.expected, self.archived_run)
        self.assertEqual(len(report['cases']), 16)
        self.assertFalse(report['exact_original_fields_all_equal'])
        self.assertFalse(report['native_contract_checks_all_passed'])
        self.assertEqual(report['counts']['native_compile_failed'], 2)
        self.assertFalse(report['comparison_entrypoints']['same_entrypoint'])
        self.assertEqual(report['comparison_entrypoints']['omitted_original_call_argument'], 'mode')
        subject = next(x for x in report['cases'] if x['case_id'] == 'compiler-00521')
        self.assertEqual(subject['original_explicit_draft_mode'], 'ref2va')
        self.assertIsNone(subject['actual_input_brief_mode'])
        self.assertTrue(all(x['semantic_status'] == 'not_determined_by_structural_evaluator' for x in report['cases']))

    def test_socket_correction_is_explicit_and_cannot_waive_a_mode_change(self):
        explicit = self.case('compiler-00581')
        diffs = [x for x in explicit['checks'] if x['field'].endswith('.socket.original')]
        self.assertEqual(diffs[0]['matched'], False)
        self.assertEqual(diffs[0]['explicit_adapter_difference']['kind'], 'base_checkpoint_socket_correction')
        changed = self.case('compiler-00521')
        wrong = [x for x in changed['checks'] if x['field'].endswith('.socket.original')]
        self.assertFalse(wrong[0]['matched'])
        self.assertNotIn('explicit_adapter_difference', wrong[0])
        self.assertEqual(changed['native_contract_status'], 'failed')

    def test_endpoint_policy_is_exact_not_one_millisecond_tolerance(self):
        result = copy.deepcopy(self.by_result['compiler-00593'])
        ref = result['result']['document']['references'][0]
        ref['anchor_ms'] = 5166
        row = self.case('compiler-00593', result)
        native = [x for x in row['checks'] if x['field'].endswith('.anchor_ms.native')][0]
        self.assertFalse(native['matched'])
        self.assertNotIn('explicit_adapter_difference', native)
        self.assertEqual(row['native_contract_status'], 'failed')

    def test_subject_scope_and_name_in_literal_text_are_not_actual_typed_uses(self):
        result = copy.deepcopy(self.by_result['compiler-00689'])
        result['result']['document']['shots'][0]['description'].append({'text': 'the driver in red appears here; a red jacket'})
        row = self.case('compiler-00689', result)
        missing = [x for x in row['checks'] if x['field'] == 'subject.<Subject 1>.actual_use_exists'][0]
        self.assertFalse(missing['matched'])
        self.assertEqual(row['native_contract_status'], 'failed')
        result['result']['document']['shots'][0]['events'].append({'kind': 'sound', 'description': [{'subject_id': 'subject-1'}, {'text': ' is heard offscreen.'}]})
        row = self.case('compiler-00689', result)
        self.assertFalse(next(x for x in row['checks'] if x['field'] == 'subject.<Subject 1>.actual_use_exists')['matched'])
        result['result']['document']['shots'][0]['description'].append({'subject_id': 'subject-1'})
        row = self.case('compiler-00689', result)
        real = [x for x in row['checks'] if x['field'] == 'subject.<Subject 1>.actual_use_exists'][0]
        self.assertTrue(real['matched'])

    def test_copy_property_and_manifest_relationships_are_separate_checks(self):
        result = copy.deepcopy(self.by_result['compiler-00641'])
        result['result']['document']['references'][0]['retention'] = 'partially_copy'
        row = self.case('compiler-00641', result)
        marker = [x for x in row['checks'] if x['field'].endswith('.reference_retention')][0]
        self.assertTrue(marker['matched'])
        artifact = [x for x in row['checks'] if x['field'].startswith('artifact.use.') and x['field'].endswith('.retention')][0]
        self.assertFalse(artifact['matched'])
        self.assertEqual(row['native_contract_status'], 'failed')

    def test_structure_cannot_be_a_typed_scene_even_with_no_actor_subjects(self):
        row = self.case('compiler-00557')
        scene = [x for x in row['checks'] if x['field'].endswith('.visual_scene_reference_uses')][0]
        self.assertFalse(scene['matched'])
        self.assertEqual(scene['actual'], ['shot-1.description'])

    def test_unknown_manifest_use_and_invalid_fps_fail_closed(self):
        result = copy.deepcopy(self.by_result['compiler-00665'])
        result['result']['artifact']['manifest']['media'][0]['target_uses'].append({'reference_id': 'missing', 'roles': ['beat_reference']})
        result['result']['document']['target']['fps']['denominator'] = 0
        row = self.case('compiler-00665', result)
        orphan = [x for x in row['checks'] if x['field'] == 'manifest.use_owner.missing'][0]
        self.assertFalse(orphan['matched'])
        self.assertFalse(next(x for x in row['checks'] if x['field'] == 'target.fps')['matched'])

    def test_changed_fixture_bytes_do_not_keep_a_stale_input_hash(self):
        inputs = copy.deepcopy(self.inputs)
        inputs['cases'][0]['input']['brief']['intent'] = 'a different request'
        with self.assertRaises(ValueError):
            evaluate(inputs, self.expected, self.archived_run)

    def test_missing_or_duplicated_original_case_rejects_the_run(self):
        result = copy.deepcopy(self.archived_run)
        result['results'].append(result['results'][0])
        with self.assertRaises(ValueError):
            evaluate(self.inputs, self.expected, result)

    def test_original_auto_cut_scope_is_separate_from_final_internal_scope(self):
        actual = json.loads((ROOT / 'conformance/reports/live-roles-round4/compiler-00689/result.json').read_text())
        row = self.case('compiler-00689', actual)
        original = next(x for x in row['checks'] if x['field'] == 'subject.<Subject 2>.declared_scope')
        final = next(x for x in row['checks'] if x['field'] == 'subject.<Subject 2>.actual_uses_cover_declared_shots')
        self.assertFalse(original['matched'])
        self.assertEqual(original['category'], 'original_contract')
        self.assertTrue(final['matched'])
        self.assertEqual(row['native_contract_status'], 'failed')  # original difference stays explicit


if __name__ == '__main__':
    unittest.main()
