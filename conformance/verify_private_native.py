#!/usr/bin/env python3
"""Run shipped native Go tests from an already verified private module cache.

This does not download modules, edit the cache, or replace the existing consumer
check. A dry check verifies provenance and Go isolation only; it never claims that
the native suite passed. Test output is retained as redacted JSONL/stderr, while
the console prints a small result without raw subprocess diagnostics.
"""
from __future__ import annotations

import argparse
import base64
import datetime as dt
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import zipfile

MODULE = "github.com/ly95/contextir"
NATIVE_COMMAND = ["test", "-json", "-count=1", "-timeout=240s", "./..."]


class CheckFailure(Exception):
    def __init__(self, stage: str, reason: str):
        self.stage, self.reason = stage, reason
        super().__init__(reason)


def require(ok: bool, stage: str, reason: str) -> None:
    if not ok:
        raise CheckFailure(stage, reason)


def reject_duplicate_members(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON member")
        result[key] = value
    return result


def decode_json(data: str | bytes, stage: str):
    try:
        return json.loads(data, object_pairs_hook=reject_duplicate_members,
                          parse_constant=lambda _: (_ for _ in ()).throw(ValueError()))
    except (ValueError, UnicodeError, TypeError):
        raise CheckFailure(stage, "invalid or ambiguous machine-readable JSON") from None


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def hash1(files: dict[str, str]) -> str:
    # golang.org/x/mod/sumdb/dirhash.Hash1: filenames and full content hashes,
    # sorted and encoded exactly as Go's cached h1 module checksum.
    summary = "".join(f"{files[name]}  {name}\n" for name in sorted(files))
    return "h1:" + base64.b64encode(hashlib.sha256(summary.encode()).digest()).decode()


def source_files(directory: Path) -> dict[str, str]:
    files = {}
    for current, dirs, names in os.walk(directory, followlinks=False):
        here = Path(current)
        require(not any((here / name).is_symlink() for name in dirs + names),
                "source_verification", "module source contains a symbolic link")
        for name in names:
            path = here / name
            require(path.is_file(), "source_verification", "module source contains a nonregular file")
            files[path.relative_to(directory).as_posix()] = sha256_file(path)
    return files


def source_digest(files: dict[str, str]) -> str:
    return hashlib.sha256(json.dumps(files, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def absolute_directory(value, stage: str) -> Path:
    require(isinstance(value, str) and Path(value).is_absolute(), stage, "expected an absolute directory")
    path = Path(value).resolve()
    require(path.is_dir(), stage, "the attested external directory is unavailable")
    return path


def verify_consumer(path: Path) -> dict:
    consumer = decode_json(path.read_bytes(), "consumer_report")
    require(isinstance(consumer, dict) and consumer.get("schema") == "contextir-private-consumer/v1",
            "consumer_report", "unsupported consumer evidence schema")
    require(consumer.get("passed") is True and consumer.get("module") == MODULE,
            "consumer_report", "the matching private consumer check did not pass")
    commit = consumer.get("resolved_commit")
    require(isinstance(commit, str) and re.fullmatch(r"[0-9a-f]{40}", commit) is not None
            and consumer.get("requested_commit") == commit,
            "origin_verification", "the consumer did not resolve its exact requested commit")
    version = consumer.get("resolved_version")
    require(isinstance(version, str) and re.fullmatch(r"v[0-9][A-Za-z0-9.+-]*", version) is not None,
            "version_verification", "the consumer has no canonical module version")
    require(consumer.get("isolated_module_cache") is True and consumer.get("workspace_disabled") is True
            and consumer.get("local_replace_allowed") is False,
            "consumer_report", "consumer evidence did not establish isolated resolution")
    result = consumer.get("consumer", {})
    require(isinstance(result, dict) and result.get("module_version") == version
            and result.get("gate_ok") is True and result.get("canonical_replay_identical") is True
            and result.get("artifact_replay_identical") is True,
            "consumer_report", "consumer smoke/replay evidence is incomplete")
    imported = consumer.get("import_source", {})
    require(isinstance(imported, dict) and imported.get("path") == MODULE
            and imported.get("version") == version and imported.get("replacement") is None,
            "source_verification", "consumer dependency has a replacement or wrong module/version")
    cache = absolute_directory(imported.get("module_cache"), "cache_verification")
    require(cache.name == "module-cache" and cache.parent.name.startswith("contextir-private-consumer-"),
            "cache_verification", "the cache is not an isolated external consumer cache")
    source = absolute_directory(imported.get("directory"), "source_verification")
    require(source == cache / (MODULE + "@" + version) and not (source / ".git").exists(),
            "source_verification", "the source is not the exact downloaded module-cache directory")
    artifacts = cache / "cache" / "download" / MODULE / "@v"
    artifact_paths = {kind: artifacts / (version + "." + kind) for kind in ("info", "mod", "zip", "ziphash")}
    require(all(p.is_file() and not p.is_symlink() for p in artifact_paths.values()),
            "cache_verification", "download provenance artifacts are missing or indirect")
    info = decode_json(artifact_paths["info"].read_bytes(), "origin_verification")
    origin = info.get("Origin", {}) if isinstance(info, dict) else {}
    require(info.get("Version") == version and isinstance(origin, dict)
            and origin.get("VCS") == "git" and origin.get("Hash") == commit
            and origin.get("URL") in ("https://" + MODULE, "https://" + MODULE + ".git"),
            "origin_verification", "cached module origin does not attest the exact remote commit")
    zip_path = artifact_paths["zip"]
    require(zip_path.stat().st_size == consumer.get("module_zip_bytes")
            and sha256_file(zip_path) == consumer.get("module_zip_sha256"),
            "zip_verification", "cached module ZIP differs from verified consumer bytes")
    prefix = MODULE + "@" + version + "/"
    zipped, qualified = {}, {}
    with zipfile.ZipFile(zip_path) as archive:
        require(len(archive.infolist()) < 100000, "zip_verification", "module ZIP inventory exceeds verifier limits")
        for item in archive.infolist():
            require(item.filename.startswith(prefix), "zip_verification", "module ZIP contains another root")
            relative = item.filename[len(prefix):]
            if item.is_dir():
                continue
            parts = PurePosixPath(relative).parts
            require(relative and not relative.startswith("/") and ".." not in parts
                    and "\\" not in relative and relative not in zipped
                    and ((item.external_attr >> 16) & 0o170000) != 0o120000,
                    "zip_verification", "module ZIP contains an unsafe or duplicate source entry")
            require(item.file_size <= 128 * 1024 * 1024,
                    "zip_verification", "one module source exceeds verifier limits")
            digest = hashlib.sha256(archive.read(item)).hexdigest()
            zipped[relative] = qualified[item.filename] = digest
    module_sum = hash1(qualified)
    require(module_sum == consumer.get("module_sum")
            and artifact_paths["ziphash"].read_text().strip() == module_sum,
            "checksum_verification", "Go module checksum does not match actual ZIP content")
    mod_bytes = artifact_paths["mod"].read_bytes()
    require(hash1({"go.mod": hashlib.sha256(mod_bytes).hexdigest()}) == consumer.get("go_mod_sum"),
            "checksum_verification", "Go module declaration checksum is inconsistent")
    actual = source_files(source)
    require(actual == zipped and (source / "go.mod").read_bytes() == mod_bytes,
            "source_verification", "extracted module sources differ from the verified module ZIP")
    nested_modules = sorted(name for name in zipped if name.endswith("/go.mod"))
    reports = sorted(name for name in zipped if name == "conformance/reports"
                     or name.startswith("conformance/reports/"))
    require(not nested_modules and not reports and not (source / "conformance/reports").exists()
            and not any(name == "go.work" or name.endswith("/go.work") for name in zipped),
            "portable_source_verification", "downloaded module includes reports, nested modules or workspaces")
    return {"consumer": consumer, "commit": commit, "version": version, "cache": cache, "source": source,
            "files": actual, "source_digest": source_digest(actual), "module_sum": module_sum,
            "artifacts": artifact_paths, "artifact_hashes": {k: sha256_file(p) for k, p in artifact_paths.items()},
            "zip_files": len(zipped), "zip_reports_files": len(reports), "nested_modules": nested_modules}


def sealed_environment(work: Path, cache: Path) -> dict[str, str]:
    # An allowlist removes all inherited provider credentials, proxies, Git
    # authentication overrides, Go overlays, and language runtime injection.
    env = {key: os.environ[key] for key in ("PATH", "SYSTEMROOT", "WINDIR") if key in os.environ}
    for name in ("home", "tmp", "gopath", "build-cache"):
        (work / name).mkdir()
    env.update({"HOME": str(work / "home"), "USERPROFILE": str(work / "home"),
                "TMPDIR": str(work / "tmp"), "TMP": str(work / "tmp"), "TEMP": str(work / "tmp"),
                "LANG": "C.UTF-8", "LC_ALL": "C.UTF-8", "GOWORK": "off", "GOENV": "off",
                "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOAUTH": "off",
                "GOFLAGS": "-mod=readonly", "GOPATH": str(work / "gopath"), "GOMODCACHE": str(cache),
                "GOCACHE": str(work / "build-cache"), "GIT_TERMINAL_PROMPT": "0",
                "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull})
    return env


def run_command(command: list[str], source: Path, env: dict[str, str], timeout: int) -> tuple[int, bytes, bytes, bool]:
    process = subprocess.Popen(command, cwd=source, env=env, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, start_new_session=True)
    timed_out = False
    try:
        stdout, stderr = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        os.killpg(process.pid, signal.SIGKILL)
        stdout, stderr = process.communicate()
    return process.returncode, stdout, stderr, timed_out


def verify_go(go: str, checked: dict, env: dict[str, str]) -> dict:
    source, cache = checked["source"], checked["cache"]
    def machine(args, stage):
        code, stdout, _, timeout = run_command([go] + args, source, env, 60)
        require(code == 0 and not timeout, stage, "Go isolation verification failed; raw diagnostics withheld")
        return decode_json(stdout, stage)
    mod = machine(["mod", "edit", "-json"], "module_declaration")
    require(mod.get("Module", {}).get("Path") == MODULE and not mod.get("Replace"),
            "module_declaration", "downloaded module has a replace directive or incorrect declaration")
    dep = machine(["list", "-m", "-json"], "module_resolution")
    require(dep.get("Main") is True and dep.get("Path") == MODULE and not dep.get("Replace")
            and Path(dep.get("Dir", "")).resolve() == source,
            "module_resolution", "native test root is not the exact downloaded main module")
    goenv = machine(["env", "-json", "GOMOD", "GOWORK", "GOENV", "GOMODCACHE", "GOCACHE",
                     "GOPATH", "GOPROXY", "GOSUMDB", "GOTOOLCHAIN", "GOFLAGS"], "go_environment")
    require(Path(goenv.get("GOMOD", "")).resolve() == source / "go.mod"
            and Path(goenv.get("GOMODCACHE", "")).resolve() == cache
            and Path(goenv.get("GOCACHE", "")).resolve() == Path(env["GOCACHE"]).resolve()
            and Path(goenv.get("GOPATH", "")).resolve() == Path(env["GOPATH"]).resolve()
            # Go reports an empty GOENV filename when the input GOENV=off.
            and env["GOENV"] == "off" and goenv.get("GOENV") in ("", "off")
            and all(goenv.get(k) == v for k, v in {"GOWORK": "off", "GOPROXY": "off",
                                                    "GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOFLAGS": "-mod=readonly"}.items()),
            "go_environment", "Go did not retain the sealed workspace/cache/network settings")
    # Check every selected dependency too: a dependency module may itself declare
    # replacements, but no actual selected dependency may resolve to a workspace.
    code, stdout, _, timeout = run_command([go, "list", "-m", "-json", "all"], source, env, 60)
    require(code == 0 and not timeout, "dependency_resolution", "offline dependency verification failed")
    decoder, text, offset, count = json.JSONDecoder(object_pairs_hook=reject_duplicate_members), stdout.decode(), 0, 0
    while offset < len(text):
        if not text[offset:].strip():
            break
        offset += len(text[offset:]) - len(text[offset:].lstrip())
        item, end = decoder.raw_decode(text, offset)
        require(not item.get("Replace"), "dependency_resolution", "a selected dependency uses replacement resolution")
        if item.get("Dir"):
            require(Path(item["Dir"]).resolve().is_relative_to(cache),
                    "dependency_resolution", "a selected dependency resolves outside the isolated cache")
        count += 1
        offset = end
    return {"go_environment": goenv, "go_environment_input": {k: env[k] for k in
                ("GOWORK", "GOENV", "GOPROXY", "GOSUMDB", "GOTOOLCHAIN", "GOFLAGS", "GOAUTH")},
            "selected_modules": count, "replace_directives": 0,
            "main_module_directory": str(source)}


URL = re.compile(r"\b(?:https?|ssh|git)://[^\s\"'<>]+", re.IGNORECASE)
SECRET = re.compile(r"(?i)(\b[\w-]*(?:api[_-]?key|access[_-]?token|secret|password|credential|authorization)[\w-]*\s*[:=]\s*)(?:\"[^\"]*\"|'[^']*'|\S+)")
BEARER = re.compile(r"(?i)\b(?:bearer|basic)\s+[A-Za-z0-9+/_.=-]+")
KEY = re.compile(r"\b(?:sk-[A-Za-z0-9_-]{10,}|gh[pousr]_[A-Za-z0-9_]{10,}|github_pat_[A-Za-z0-9_]{10,})\b")


def redact(value):
    if isinstance(value, str):
        value = URL.sub("[URL REDACTED]", value)
        # Match the complete authentication scheme/value before the generic
        # assignment rule could redact only the word "Bearer" and leave its key.
        value = BEARER.sub("[AUTH_REDACTED]", value)
        value = SECRET.sub(r"\1[REDACTED]", value)
        return KEY.sub("[KEY REDACTED]", value)
    if isinstance(value, dict):
        return {k: redact(v) for k, v in value.items()}
    if isinstance(value, list):
        return [redact(v) for v in value]
    return value


def retain_test_output(stdout: bytes, stderr: bytes, stdout_path: Path, stderr_path: Path) -> dict:
    terminal, packages, started = {}, {}, set()
    invalid = 0
    with stdout_path.open("x", encoding="utf-8") as stream:
        for raw in stdout.splitlines():
            try:
                event = decode_json(raw, "test_output")
                require(isinstance(event, dict), "test_output", "test JSON event is not an object")
            except CheckFailure:
                invalid += 1
                event = {"Action": "withheld_non_json", "bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest()}
            stream.write(json.dumps(redact(event), ensure_ascii=False) + "\n")
            package, test, action = event.get("Package"), event.get("Test"), event.get("Action")
            if isinstance(package, str) and isinstance(test, str):
                if action == "run":
                    started.add((package, test))
                if action in ("pass", "fail", "skip"):
                    terminal[(package, test)] = action
            elif isinstance(package, str) and action in ("pass", "fail", "skip"):
                packages[package] = action
    with stderr_path.open("x", encoding="utf-8") as stream:
        stream.write(redact(stderr.decode("utf-8", errors="replace")))
    counts = {name: {"total": 0, "pass": 0, "fail": 0, "skip": 0} for name in ("root", "sub", "packages")}
    for (package, test), action in terminal.items():
        group = counts["sub" if "/" in test else "root"]
        group["total"] += 1
        group[action] += 1
    for action in packages.values():
        counts["packages"]["total"] += 1
        counts["packages"][action] += 1
    return {"counts": counts, "invalid_json_lines": invalid,
            "unterminated_tests": len(started - set(terminal)),
            "failures": [{"package": p, "test": t} for (p, t), action in sorted(terminal.items()) if action == "fail"],
            "skips": [{"package": p, "test": t} for (p, t), action in sorted(terminal.items()) if action == "skip"],
            "stdout_path": str(stdout_path), "stderr_path": str(stderr_path),
            "stdout_raw_sha256": hashlib.sha256(stdout).hexdigest(),
            "stderr_raw_sha256": hashlib.sha256(stderr).hexdigest(),
            "stdout_retained_sha256": sha256_file(stdout_path), "stderr_retained_sha256": sha256_file(stderr_path),
            "output_policy": "JSON event structure retained; URLs, credential assignments and token-shaped literals redacted; non-JSON stdout withheld with hash/size."}


def verify_unchanged(checked: dict) -> None:
    require(source_files(checked["source"]) == checked["files"]
            and all(sha256_file(path) == checked["artifact_hashes"][kind]
                    for kind, path in checked["artifacts"].items()),
            "cache_immutability", "verified source/download cache bytes changed during native verification")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--consumer-report", type=Path, required=True, help="Existing successful private consumer JSON report")
    parser.add_argument("--report", type=Path, required=True, help="New absolute JSON report path (never overwritten)")
    parser.add_argument("--dry-check", action="store_true", help="Verify provenance/isolation only; do not run any native test")
    args = parser.parse_args()
    if not args.consumer_report.is_absolute() or not args.consumer_report.is_file():
        parser.error("--consumer-report must be an existing absolute file")
    if not args.report.is_absolute() or args.report.exists() or not args.report.parent.is_dir():
        parser.error("--report must be a new absolute path in an existing directory")
    if "module-cache" in args.report.resolve().parts:
        # Reject before reading/validating evidence too, so an invalid consumer
        # report can never cause even an error report to be written to the cache.
        print("Reports cannot be written inside a module cache.", file=sys.stderr)
        return 2
    stdout_path = args.report.with_name(args.report.stem + ".go-test.stdout.jsonl")
    stderr_path = args.report.with_name(args.report.stem + ".go-test.stderr.txt")
    if stdout_path.exists() or stderr_path.exists():
        parser.error("derived output paths must also be new")
    work = Path(tempfile.mkdtemp(prefix="contextir-private-native-")).resolve()
    report = {"schema": "contextir-private-native/v1", "started_at": dt.datetime.now(dt.timezone.utc).isoformat(),
              "consumer_report": str(args.consumer_report.resolve()), "consumer_report_sha256": sha256_file(args.consumer_report),
              "report_path": str(args.report), "work_directory": str(work), "mode": "dry_check" if args.dry_check else "full_native_suite",
              "passed": False, "native_tests_passed": None, "full_suite_executed": False,
              "reports_independence_established": False,
              "credentials_policy": "inherited environment allowlist; isolated HOME; provider/Git credentials not forwarded",
              "network_policy": "Go module proxy and checksum service disabled; this is not an OS network firewall"}
    checked = None
    try:
        checked = verify_consumer(args.consumer_report)
        if args.report.resolve().is_relative_to(checked["cache"]):
            # Do not emit even a failure report to a prohibited cache path.
            print("Reports cannot be written inside the module cache.", file=sys.stderr)
            return 2
        go = shutil.which("go")
        require(go is not None, "go_executable", "local Go executable is unavailable")
        env = sealed_environment(work, checked["cache"])
        report.update({"module": MODULE, "resolved_commit": checked["commit"], "resolved_version": checked["version"],
                       "module_directory": str(checked["source"]), "module_cache": str(checked["cache"]),
                       "module_zip_sha256": checked["artifact_hashes"]["zip"], "module_sum": checked["module_sum"],
                       "source_tree_sha256": checked["source_digest"], "source_files": len(checked["files"]),
                       "module_zip_files": checked["zip_files"], "zip_reports_files": checked["zip_reports_files"],
                       "nested_go_modules": checked["nested_modules"], "reports_directory_present": False,
                       "cache_matches_verified_zip": True, "provider_credentials_forwarded": False})
        report.update(verify_go(go, checked, env))
        if not args.dry_check:
            command = [go] + NATIVE_COMMAND
            report["command"] = command
            report["full_suite_executed"] = True
            code, stdout, stderr, timed_out = run_command(command, checked["source"], env, 900)
            result = retain_test_output(stdout, stderr, stdout_path, stderr_path)
            report["native"] = result
            report["native"].update({"exit_status": code, "launcher_timed_out": timed_out})
            passed = (code == 0 and not timed_out and result["invalid_json_lines"] == 0
                      and result["unterminated_tests"] == 0 and result["counts"]["root"]["total"] > 0
                      and result["counts"]["packages"]["total"] > 0
                      and all(v["fail"] == 0 for v in result["counts"].values()))
            report["native_tests_passed"] = passed
            require(passed, "native_tests", "shipped native suite failed or returned incomplete JSON evidence; inspect retained redacted logs")
        verify_unchanged(checked)
        report["cache_unchanged"] = True
        report["reports_independence_established"] = report["native_tests_passed"] is True
        report["passed"] = True
    except CheckFailure as error:
        report["failure"] = {"stage": error.stage, "reason": error.reason}
    except Exception as error:
        report["failure"] = {"stage": "launcher", "reason": "launcher failed; exception diagnostics withheld", "exception_type": type(error).__name__}
    finally:
        if checked is not None and "cache_unchanged" not in report:
            try:
                verify_unchanged(checked)
                report["cache_unchanged"] = True
            except Exception:
                report["cache_unchanged"] = False
                report["passed"] = False
        report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
    with args.report.open("x", encoding="utf-8") as stream:
        json.dump(report, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    print(json.dumps({"passed": report["passed"], "mode": report["mode"], "report_path": str(args.report),
                      "resolved_commit": report.get("resolved_commit"), "native_tests_passed": report["native_tests_passed"],
                      "failure_stage": report.get("failure", {}).get("stage")}))
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
