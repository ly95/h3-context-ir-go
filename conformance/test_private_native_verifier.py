"""Launcher controls only; no provider calls or original-corpus pass claims."""
import contextlib
import hashlib
import io
import json
from pathlib import Path
import shutil
import sys
import tempfile
import unittest
from unittest import mock
import zipfile

import verify_private_native as native


class PrivateNativeVerifierTests(unittest.TestCase):
    def fixture(self, root, extra=None):
        cache = root / "module-cache"
        version, commit = "v0.0.0-20261020000000-0123456789ab", "0123456789abcdef0123456789abcdef01234567"
        source = cache / (native.MODULE + "@" + version)
        source.mkdir(parents=True)
        files = {"go.mod": f"module {native.MODULE}\n\ngo 1.25\n".encode(),
                 "module.go": b"package contextir\n"}
        files.update(extra or {})
        artifacts = cache / "cache/download" / native.MODULE / "@v"
        artifacts.mkdir(parents=True)
        zipped = {}
        zip_path = artifacts / (version + ".zip")
        with zipfile.ZipFile(zip_path, "w") as archive:
            for relative, data in files.items():
                p = source / relative
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_bytes(data)
                name = native.MODULE + "@" + version + "/" + relative
                archive.writestr(name, data)
                zipped[name] = hashlib.sha256(data).hexdigest()
        module_sum = native.hash1(zipped)
        (artifacts / (version + ".ziphash")).write_text(module_sum)
        (artifacts / (version + ".mod")).write_bytes(files["go.mod"])
        (artifacts / (version + ".info")).write_text(json.dumps({"Version": version,
            "Origin": {"VCS": "git", "Hash": commit, "URL": "https://" + native.MODULE}}))
        report = {"schema": "contextir-private-consumer/v1", "passed": True, "module": native.MODULE,
                  "resolved_commit": commit, "requested_commit": commit, "resolved_version": version,
                  "isolated_module_cache": True, "workspace_disabled": True, "local_replace_allowed": False,
                  "consumer": {"module_version": version, "gate_ok": True,
                               "canonical_replay_identical": True, "artifact_replay_identical": True},
                  "import_source": {"path": native.MODULE, "version": version, "replacement": None,
                                    "module_cache": str(cache), "directory": str(source)},
                  "module_zip_bytes": zip_path.stat().st_size, "module_zip_sha256": native.sha256_file(zip_path),
                  "module_sum": module_sum, "go_mod_sum": native.hash1({"go.mod": hashlib.sha256(files["go.mod"]).hexdigest()})}
        path = root / "consumer.json"
        path.write_text(json.dumps(report))
        return path, source, report

    def test_valid_exact_zip_and_fail_closed_tampering(self):
        with tempfile.TemporaryDirectory(prefix="contextir-private-consumer-") as tmp:
            path, source, report = self.fixture(Path(tmp))
            checked = native.verify_consumer(path)
            self.assertEqual(checked["zip_files"], 2)
            self.assertEqual(checked["zip_reports_files"], 0)
            native.verify_unchanged(checked)
            (source / "module.go").write_text("package contextir // modified\n")
            with self.assertRaisesRegex(native.CheckFailure, "sources differ"):
                native.verify_consumer(path)
            with self.assertRaisesRegex(native.CheckFailure, "bytes changed"):
                native.verify_unchanged(checked)
            report["passed"] = False
            path.write_text(json.dumps(report))
            with self.assertRaisesRegex(native.CheckFailure, "did not pass"):
                native.verify_consumer(path)

    def test_origin_and_checksum_are_independent_required_evidence(self):
        for kind in ("commit", "zip_sha", "h1"):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(prefix="contextir-private-consumer-") as tmp:
                path, _, report = self.fixture(Path(tmp))
                if kind == "commit":
                    report["resolved_commit"] = "f" * 40
                    report["requested_commit"] = "f" * 40
                elif kind == "zip_sha":
                    report["module_zip_sha256"] = "0" * 64
                else:
                    report["module_sum"] = "h1:invalid"
                path.write_text(json.dumps(report))
                with self.assertRaises(native.CheckFailure):
                    native.verify_consumer(path)

    def test_reports_nested_modules_and_unshipped_files_are_rejected(self):
        for relative in ("conformance/reports/run.json", "nested/go.mod", "go.work"):
            with self.subTest(relative=relative), tempfile.TemporaryDirectory(prefix="contextir-private-consumer-") as tmp:
                path, _, _ = self.fixture(Path(tmp), {relative: b"not a runtime dependency"})
                with self.assertRaisesRegex(native.CheckFailure, "reports, nested modules or workspaces"):
                    native.verify_consumer(path)
        with tempfile.TemporaryDirectory(prefix="contextir-private-consumer-") as tmp:
            path, source, _ = self.fixture(Path(tmp))
            (source / "extra.go").write_text("package contextir\n")
            with self.assertRaisesRegex(native.CheckFailure, "sources differ"):
                native.verify_consumer(path)

    def test_credentials_and_cache_report_writes_are_not_forwarded(self):
        with tempfile.TemporaryDirectory(prefix="contextir-private-consumer-") as tmp:
            root = Path(tmp)
            path, source, _ = self.fixture(root)
            work = root / "native-work"
            work.mkdir()
            with mock.patch.dict("os.environ", {"DEEPSEEK_API_KEY": "must-not-propagate", "GIT_CONFIG_COUNT": "9",
                                                "GOWORK": "/worktree/go.work", "GOFLAGS": "-overlay=/worktree/overlay"}):
                env = native.sealed_environment(work, root / "module-cache")
            self.assertNotIn("DEEPSEEK_API_KEY", env)
            self.assertNotIn("GIT_CONFIG_COUNT", env)
            self.assertEqual(env["GOFLAGS"], "-mod=readonly")
            self.assertEqual(env["GOWORK"], "off")
            forbidden = source / "forbidden-report.json"
            with mock.patch.object(sys, "argv", ["verify", "--consumer-report", str(path), "--report", str(forbidden), "--dry-check"]):
                with contextlib.redirect_stderr(io.StringIO()):
                    self.assertEqual(native.main(), 2)
            self.assertFalse(forbidden.exists())

    @unittest.skipUnless(shutil.which("go"), "local Go is required for the actual declaration boundary")
    def test_real_go_rejects_replace_before_tests(self):
        with tempfile.TemporaryDirectory(prefix="contextir-private-consumer-") as tmp:
            root = Path(tmp)
            data = f"module {native.MODULE}\n\ngo 1.25\nreplace example.invalid/dep => /worktree/dep\n".encode()
            path, _, _ = self.fixture(root, {"go.mod": data})
            checked = native.verify_consumer(path)
            work = root / "native-work"
            work.mkdir()
            env = native.sealed_environment(work, checked["cache"])
            with self.assertRaisesRegex(native.CheckFailure, "replace directive"):
                native.verify_go(shutil.which("go"), checked, env)

    def test_json_event_leaf_counts_and_redacted_diagnostics(self):
        def event(action, test=None):
            out = {"Action": action, "Package": native.MODULE + "/compiler"}
            if test:
                out["Test"] = test
            return out
        events = [event("run", "TestRoot"), event("run", "TestRoot/leaf"), event("pass", "TestRoot/leaf"),
                  event("skip", "TestRoot/skipped"), event("fail", "TestRoot/bad"), event("fail", "TestRoot"), event("fail")]
        events.append({"Action": "output", "Output": "Authorization: Bearer abcdefg https://user:password@private.test/path?key=secret sk-abcdefghijklmnop"})
        stdout = "\n".join(json.dumps(e) for e in events).encode() + b"\nraw-secret-output\n"
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            out, err = root / "stdout.jsonl", root / "stderr.txt"
            result = native.retain_test_output(stdout, b"API_KEY=privatevalue https://private.test/?token=secret", out, err)
            self.assertEqual(result["counts"]["root"], {"total": 1, "pass": 0, "fail": 1, "skip": 0})
            self.assertEqual(result["counts"]["sub"], {"total": 3, "pass": 1, "fail": 1, "skip": 1})
            self.assertEqual(result["invalid_json_lines"], 1)
            self.assertEqual(result["unterminated_tests"], 0)
            retained = out.read_text() + err.read_text()
            for raw in ("abcdefg", "private.test", "privatevalue", "sk-abcdefghijklmnop", "raw-secret-output"):
                self.assertNotIn(raw, retained)


if __name__ == "__main__":
    unittest.main()
