"""Receipt-shape compatibility cannot substitute a different Go build target."""
import unittest

from build_frozen_bridge import environment_triplet, require_environment


class FrozenBridgeEnvironmentTests(unittest.TestCase):
    def test_legacy_list_and_live_receipt_map_identify_the_same_toolchain(self):
        expected = ['darwin', 'arm64', 'go1.27.2']
        recorded = {'GOOS': 'darwin', 'GOARCH': 'arm64', 'GOVERSION': 'go1.27.2',
                    'GOROOT': '/explicit/toolchain', 'GOTOOLCHAIN': 'local', 'GOWORK': 'off'}
        self.assertEqual(environment_triplet(recorded), expected)
        self.assertEqual(environment_triplet(expected), expected)
        require_environment(recorded, expected)
        require_environment(expected, recorded)

    def test_platform_or_toolchain_drift_is_rejected(self):
        expected = ['darwin', 'arm64', 'go1.27.2']
        for name, replacement in [('GOOS', 'linux'), ('GOARCH', 'amd64'), ('GOVERSION', 'go1.25.0')]:
            with self.subTest(name=name):
                recorded = dict(zip(['GOOS', 'GOARCH', 'GOVERSION'], expected))
                recorded[name] = replacement
                with self.assertRaises(ValueError):
                    require_environment(recorded, expected)

    def test_missing_or_malformed_fields_are_not_accepted_as_unknown(self):
        for value in [None, 'go1.27.2', [], ['darwin', 'arm64'],
                      ['darwin', 'arm64', 'go1.27.2', 'extra'],
                      ['darwin', 'arm64', None], {'GOOS': 'darwin', 'GOARCH': 'arm64'},
                      {'GOOS': 'darwin', 'GOARCH': 'arm64', 'GOVERSION': ''}]:
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    environment_triplet(value)


if __name__ == '__main__':
    unittest.main()
