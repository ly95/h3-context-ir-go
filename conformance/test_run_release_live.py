"""Exercise launcher receipts with synthetic configuration and no subprocess."""
import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import run_release_live as launcher


class RetryBudgetLauncherTests(unittest.TestCase):
    def test_explicit_retry_budgets_are_forwarded_and_recorded(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'conformance/reports').mkdir(parents=True)
            frozen = root / 'frozen'
            (frozen / 'conformance').mkdir(parents=True)
            binary = frozen / 'contextir-live-eval'
            binary.write_bytes(b'synthetic executable: subprocess is mocked')
            inputs = frozen / 'conformance/release_acceptance_inputs.json'
            assertions = frozen / 'conformance/release_acceptance_assertions.json'
            inputs.write_text('{}')
            assertions.write_text('{}')
            build = root / 'build.json'
            build.write_text(json.dumps({
                'passed': True, 'working_runtime_sources_unchanged': True,
                'build_directory': str(frozen), 'binary': str(binary),
                'binary_sha256': launcher.digest(binary), 'runtime_sources': {},
                'sources': {str(p.relative_to(frozen)): launcher.digest(p) for p in (inputs, assertions)},
            }))
            config = root / 'synthetic.env'
            config.write_text('DEEPSEEK_API_KEY=synthetic-non-secret-test-value\nDEEPSEEK_BASE_URL=http://127.0.0.1:12345\n')
            arguments = ['run_release_live.py', '--build-receipt', str(build),
                         '--credential-file', str(config), '--label', 'synthetic',
                         '--base-url', 'http://127.0.0.1:12345', '--model', 'deepseek-flash',
                         '--reasoning-effort', 'low', '--max-output-tokens', '16384',
                         '--max-structure-retries', '3', '--max-planner-retries', '0']
            process = mock.Mock()
            process.stdout = iter([])
            process.wait.return_value = 0
            with mock.patch.object(launcher, 'ROOT', root), mock.patch('sys.argv', arguments), \
                    mock.patch.object(launcher.subprocess, 'Popen', return_value=process) as spawn, \
                    contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(launcher.main(), 0)
            command = spawn.call_args.args[0]
            self.assertEqual(command[command.index('-max-structure-retries') + 1], '3')
            self.assertEqual(command[command.index('-max-planner-retries') + 1], '0')
            receipt = root / 'conformance/reports/synthetic-execution.json'
            value = json.loads(receipt.read_text())
            self.assertEqual(value['max_structure_retries'], 3)
            self.assertEqual(value['max_planner_retries'], 0)
            self.assertEqual(value['max_output_tokens'], 16384)
            self.assertEqual(value['command'], command)
            self.assertNotIn('synthetic-non-secret-test-value', receipt.read_text())

    def test_out_of_range_budgets_fail_before_reading_any_configuration(self):
        for flag, value in [('--max-structure-retries', '-1'), ('--max-structure-retries', '4'),
                            ('--max-planner-retries', '-1'), ('--max-planner-retries', '4')]:
            with self.subTest(flag=flag, value=value):
                arguments = ['run_release_live.py', '--build-receipt', '/absent/build',
                             '--credential-file', '/absent/config', '--label', 'invalid',
                             '--base-url', 'http://127.0.0.1:12345', '--model', 'deepseek-flash',
                             '--reasoning-effort', 'low', '--max-output-tokens', '16384', flag, value]
                with mock.patch('sys.argv', arguments), mock.patch.object(Path, 'read_text') as read, \
                        mock.patch.object(launcher.subprocess, 'Popen') as spawn, \
                        contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                    launcher.main()
                read.assert_not_called()
                spawn.assert_not_called()


if __name__ == '__main__':
    unittest.main()
