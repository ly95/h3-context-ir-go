#!/usr/bin/env python3
"""Run the complete module with its exact declared minimum Go toolchain.

No toolchain/dependency download or provider environment is permitted. The
explicit executable must be the real minimum compiler; a newer patch is not a
substitute. Reports and raw outputs are new files and never replace prior runs.
This standalone acceptance runner is not invoked by ordinary Go tests.
"""

import argparse
import collections
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
import time

from run_native_functionality import compatibility_source_hashes

ROOT = Path(__file__).resolve().parents[1]
SCHEMA = "contextir-go-minimum-compatibility/v1"


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def absolute_path(value):
    path = Path(value)
    if not path.is_absolute():
        raise argparse.ArgumentTypeError("an explicit absolute path is required")
    return path.resolve()


def extra_inputs(root):
    """Protect external test fixtures as well as Go sources/runtime embeds."""
    result = {}
    for directory, directories, files in os.walk(root):
        base = Path(directory)
        directories[:] = sorted(x for x in directories if x not in (".git", ".codegraph", "vendor") and not (base == root / "conformance" and x == "reports"))
        for name in sorted(files):
            path = base / name
            relative = path.relative_to(root)
            is_source = path.suffix == ".go"
            is_fixture = "testdata" in relative.parts or relative.parts[:2] == ("conformance", "fixtures")
            if not (is_source or is_fixture):
                continue
            require(not path.is_symlink() and path.resolve().is_relative_to(root), "Compatibility inputs cannot be external symlinks: " + str(relative))
            if is_fixture:
                result[str(relative)] = digest(path)
    return result


def run_capture(command, root, timeout=120):
    run = subprocess.run(command, cwd=root, capture_output=True, text=True, timeout=timeout)
    require(run.returncode == 0, "Preflight command failed: " + " ".join(command) + "\n" + run.stderr)
    return run.stdout


def run_logged(command, root, output, errors, timeout):
    with output.open("xb") as stdout, errors.open("xb") as stderr:
        process = subprocess.Popen(command, cwd=root, stdout=stdout, stderr=stderr, start_new_session=True)
        try:
            return process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
            raise ValueError("Command exceeded its explicit timeout: " + " ".join(command)) from None


def inspect_events(path, expected_packages, module):
    tests, packages = {}, {}
    outputs = collections.defaultdict(str)
    for line in path.read_text().splitlines():
        event = json.loads(line)
        package, action = event.get("Package"), event.get("Action")
        require(isinstance(package, str), "Go JSON event lacks a package")
        if event.get("Output"):
            outputs[package] += event["Output"]
        if action not in ("run", "pass", "skip", "fail"):
            continue
        if event.get("Test"):
            tests[(package, event["Test"])] = action
        else:
            packages[package] = action
    require(set(packages) == set(expected_packages), "Go test did not finish every listed module package")
    require(tests and all(state == "pass" for state in tests.values()), "Test failures/skips/incomplete tests remain in minimum-toolchain events")
    for package, state in packages.items():
        no_tests = state == "skip" and "[no test files]" in outputs[package] and not any(p == package for p, _ in tests)
        require(state == "pass" or no_tests, "Package did not compile/test successfully: " + package)
    mandatory = ("TestGoCompatibilityEverySourceParses", "TestGoCompatibilityDeclaredLanguageVersion")
    for test in mandatory:
        require(tests.get((module, test)) == "pass", "Missing executed native compatibility guard: " + test)
    return {
        "packages": dict(sorted(packages.items())),
        "package_count": len(packages),
        "test_events": [{"package": p, "test": t, "status": state} for (p, t), state in sorted(tests.items())],
        "passed_test_events": len(tests),
        "failed_test_events": 0,
        "skipped_test_events": 0,
        "mandatory_native_guards": list(mandatory),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--toolchain", type=absolute_path, required=True, help="Absolute path to the real minimum GOROOT/bin/go")
    parser.add_argument("--report", type=absolute_path, required=True, help="New absolute JSON report path")
    parser.add_argument("--module-root", type=absolute_path, default=ROOT)
    parser.add_argument("--timeout-seconds", type=int, default=1200, help="Explicit timeout for each full test/vet command")
    args = parser.parse_args()
    require(args.timeout_seconds > 0, "Timeout must be positive")
    require(args.toolchain.is_file() and os.access(args.toolchain, os.X_OK) and args.toolchain.name == "go", "Toolchain must be an executable GOROOT/bin/go file")
    require((args.module_root / "go.mod").is_file(), "Module root must contain go.mod")
    require(args.report.parent.is_dir(), "Report parent directory must already exist")
    stem = args.report.with_suffix("")
    events = Path(str(stem) + ".go-events.jsonl")
    test_errors = Path(str(stem) + ".test.stderr.txt")
    vet_output = Path(str(stem) + ".vet.stdout.txt")
    vet_errors = Path(str(stem) + ".vet.stderr.txt")
    require(not any(p.exists() for p in (args.report, events, test_errors, vet_output, vet_errors)), "Refusing to overwrite compatibility evidence")
    report = {
        "schema": SCHEMA,
        "started_at_utc": dt.datetime.now(dt.timezone.utc).isoformat(),
        "module_root": str(args.module_root),
        "toolchain": str(args.toolchain),
        "toolchain_sha256": digest(args.toolchain),
        "runner_sha256": digest(Path(__file__).resolve()),
        "command": [str(args.toolchain), "test", "-json", "-count=1", "./..."],
        "vet_command": [str(args.toolchain), "vet", "./..."],
        "passed": False,
        "source_files_unchanged_during_run": False,
        "raw_go_events": str(events),
    }
    start = time.monotonic()
    try:
        directives = []
        for line in (args.module_root / "go.mod").read_text().splitlines():
            fields = line.split("//", 1)[0].split()
            if fields and fields[0] == "go":
                require(len(fields) == 2, "Malformed go language directive")
                directives.append(fields[1])
        require(len(directives) == 1 and re.fullmatch(r"[0-9]+\.[0-9]+(?:\.[0-9]+)?", directives[0]), "go.mod must declare one stable minimum language version")
        declared = directives[0]
        minimum = declared if declared.count(".") == 2 else declared + ".0"
        report.update(declared_go_version=declared, required_go_version="go" + minimum)
        with tempfile.TemporaryDirectory(prefix="contextir-go-minimum-") as directory:
            temporary = Path(directory)
            for name in ("home", "tmp", "build-cache", "gopath", "module-cache"):
                (temporary / name).mkdir()
            environment = {
                "PATH": os.pathsep.join([str(args.toolchain.parent), "/usr/bin", "/bin", "/usr/sbin", "/sbin", "/opt/homebrew/bin", "/usr/local/bin"]),
                "HOME": str(temporary / "home"), "TMPDIR": str(temporary / "tmp"),
                "GOCACHE": str(temporary / "build-cache"), "GOPATH": str(temporary / "gopath"), "GOMODCACHE": str(temporary / "module-cache"),
                "GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off", "GOAUTH": "off", "GOVCS": "*:off", "GOFLAGS": "-mod=readonly",
                "GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
                "TZ": "UTC", "LANG": "en_US.UTF-8",
            }
            # No copy/filter of inherited variables: provider credentials,
            # proxies, overlays, workspace files and per-user go config never
            # reach any Go command or its subprocesses.
            os.environ.clear()
            os.environ.update(environment)
            report["sealed_environment_keys"] = sorted(environment)
            version = run_capture([str(args.toolchain), "version"], args.module_root).strip()
            match = re.fullmatch(r"go version (go[0-9]+\.[0-9]+\.[0-9]+) (\S+)", version)
            require(match is not None, "Toolchain does not report a stable release version")
            report.update(actual_go_version=match.group(1), go_version_output=version)
            require(match.group(1) == "go" + minimum, "Exact declared minimum required; a newer compiler or patch is not minimum-version evidence")
            names = ["GOVERSION", "GOROOT", "GOOS", "GOARCH", "GOTOOLCHAIN", "GOWORK", "GOENV", "GOFLAGS", "GOPROXY", "GOSUMDB", "GOAUTH", "GOCACHE", "GOMODCACHE", "GOPATH", "CGO_ENABLED"]
            go_environment = json.loads(run_capture([str(args.toolchain), "env", "-json", *names], args.module_root))
            require(go_environment["GOVERSION"] == report["actual_go_version"], "Actual Go environment changed compiler version")
            require((Path(go_environment["GOROOT"]) / "bin" / "go").resolve() == args.toolchain, "Requested executable is not its actual GOROOT/bin/go")
            report["go_environment"] = go_environment
            metadata = json.loads(run_capture([str(args.toolchain), "mod", "edit", "-json"], args.module_root))
            require(not metadata.get("Replace"), "Replacement directives cannot substitute unreviewed/local source in compatibility evidence")
            report["module"] = metadata["Module"]["Path"]
            before_extra = extra_inputs(args.module_root)
            before = compatibility_source_hashes(args.module_root)
            report["source_sha256"] = before
            report["additional_input_sha256"] = before_extra
            expected = run_capture([str(args.toolchain), "list", "./..."], args.module_root).splitlines()
            report["expected_packages"] = expected
            print("Running exact minimum", report["actual_go_version"], "across", len(expected), "packages", flush=True)
            report["exit_code"] = run_logged(report["command"], args.module_root, events, test_errors, args.timeout_seconds)
            report["raw_go_events_sha256"] = digest(events)
            report["test_stderr"] = str(test_errors)
            report["test_stderr_sha256"] = digest(test_errors)
            # Run vet even after failing tests: both outcomes remain explicit.
            report["vet_exit_code"] = run_logged(report["vet_command"], args.module_root, vet_output, vet_errors, args.timeout_seconds)
            report.update(vet_stdout=str(vet_output), vet_stdout_sha256=digest(vet_output), vet_stderr=str(vet_errors), vet_stderr_sha256=digest(vet_errors))
            after = compatibility_source_hashes(args.module_root)
            after_extra = extra_inputs(args.module_root)
            report["source_files_unchanged_during_run"] = before == after and before_extra == after_extra
            report["changed_inputs"] = sorted(k for k in set(before) | set(after) if before.get(k) != after.get(k)) + sorted(k for k in set(before_extra) | set(after_extra) if before_extra.get(k) != after_extra.get(k))
            require(report["exit_code"] == 0 and report["vet_exit_code"] == 0, "Minimum-toolchain test or vet command failed; inspect raw logs")
            require(report["source_files_unchanged_during_run"], "Source or fixture inputs changed during compatibility validation")
            report["counts"] = inspect_events(events, expected, report["module"])
            report["passed"] = True
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        report["error"] = str(error)
    report["elapsed_seconds"] = round(time.monotonic() - start, 3)
    report["finished_at_utc"] = dt.datetime.now(dt.timezone.utc).isoformat()
    with args.report.open("x") as output:
        json.dump(report, output, ensure_ascii=False, indent=2)
        output.write("\n")
    print(("PASS" if report["passed"] else "FAIL") + ": " + str(args.report), flush=True)
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
