#!/usr/bin/env python3
"""Replay a frozen live run through the exact downloaded private Go module.

First run verify_private_consumer.py for the already-pushed release candidate.
This command then builds a different, external host with that exact downloaded
dependency, no replacement/workspace, no provider credentials and offline Go
resolution. It replays every recorded model attempt only after complete request
and actual image conditioning equality, writes/reads real Result/Document files,
and executes Refine from the restored Result. Live semantic checks remain a
separate result: successful replay does not convert a failed live case to a pass.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import shutil
import subprocess
import sys
import tarfile
import tempfile

from verify_private_native import (CheckFailure, decode_json, require, sealed_environment,
                                   sha256_file, source_files, verify_consumer, verify_unchanged)

ROOT = Path(__file__).resolve().parents[1]
MODULE = "github.com/ly95/contextir"
TEMPLATE = Path(__file__).with_name("release_consumer.go.txt")


def command(args, cwd, env, stage, timeout=600):
    try:
        run = subprocess.run(args, cwd=cwd, env=env, capture_output=True, timeout=timeout)
    except (OSError, subprocess.TimeoutExpired):
        raise CheckFailure(stage, "external consumer command could not complete; raw diagnostics withheld") from None
    if run.returncode != 0:
        error = CheckFailure(stage, "external consumer command failed; raw diagnostics withheld (SHA-256/size retained)")
        error.details = {"exit_status": run.returncode,
                         "stdout_bytes": len(run.stdout), "stdout_sha256": hashlib.sha256(run.stdout).hexdigest(),
                         "stderr_bytes": len(run.stderr), "stderr_sha256": hashlib.sha256(run.stderr).hexdigest()}
        # This executable's errors are controlled, content-free diagnostics from
        # the template, unlike external Git/Go command diagnostics. Retain only
        # its known error prefix and a bounded numeric case identifier.
        if stage == "consumer_replay":
            message = run.stderr.decode("utf-8", errors="strict").strip()
            own_messages = {
                "production requested an unrecorded model call", "recorded call order or stage differs",
                "production stage/system/input/schema differs; response reuse prohibited",
                "recorded request could not be read", "recorded image input could not be read",
                "image input has no recorded actual conditioning", "image conditioning count differs",
                "actual image conditioning bytes/detail differ", "complete recorded model response could not be read",
                "recorded call accounting differs from the complete response", "recorded image provenance count differs",
                "recorded image hashes do not match actual conditioning bytes", "image conditioning has no original fixture behind it",
                "recorded response output is unavailable", "recorded output differs from complete response",
                "recorded failed call lacks its typed error contract", "operation failure contract differs from live execution",
                "final Result differs from live execution", "production did not consume every recorded model attempt",
                "published Document/Artifact files differ from actual final Result",
                "recorded initial Brief differs from original fixture conversion",
            }
            prefix, separator, reason = message.partition(": ")
            if separator and prefix.startswith("case ") and prefix[5:].isdigit() and reason in own_messages:
                error.details["consumer_diagnostic"] = message
        raise error
    return run.stdout, run.stderr


def sealed_json(args, cwd, env, stage):
    stdout, _ = command(args, cwd, env, stage)
    return decode_json(stdout, stage)


def check_live_source(build, live, checked):
    require(build.get("schema") == "contextir-live-frozen-build/v1"
            and build.get("passed") is True and build.get("exit_code") == 0
            and build.get("archived_sources_unchanged") is True
            and build.get("working_sources_unchanged") is True,
            "live_source", "live recording has no successful frozen-source build receipt")
    binary_sha = build.get("binary_sha256")
    require(isinstance(binary_sha, str) and len(binary_sha) == 64
            and binary_sha == live.get("binary_sha256"),
            "live_binary", "live run and frozen build have different executables")
    binary = Path(build.get("binary", ""))
    require(binary.is_absolute() and binary.is_file() and sha256_file(binary) == binary_sha,
            "live_binary", "original live executable is unavailable or changed")
    runtime = build.get("runtime_sources")
    require(isinstance(runtime, dict) and len(runtime) > 1 and "go.mod" in runtime
            and "cmd/contextir-live-eval/main.go" in runtime,
            "live_source", "frozen build lacks a complete runtime-source inventory")
    sources = build.get("sources")
    require(isinstance(sources, dict) and all(sources.get(name) == sha for name, sha in runtime.items()),
            "live_source", "runtime source inventory differs from the frozen source inventory")
    require(all(checked["files"].get(name) == sha for name, sha in runtime.items()),
            "published_source", "downloaded module differs from the exact live executable runtime sources")
    archive = Path(build.get("source_archive", ""))
    if not archive.is_absolute():
        archive = ROOT / archive
    require(archive.is_file() and sha256_file(archive) == build.get("source_archive_sha256"),
            "live_source", "frozen-source archive is unavailable or changed")
    with tarfile.open(archive) as saved:
        members = {item.name: item for item in saved.getmembers()}
        require(len(members) == len(saved.getmembers()), "live_source", "frozen archive contains duplicate names")
        for name, sha in runtime.items():
            parts = PurePosixPath(name).parts
            require(parts and ".." not in parts and not name.startswith("/") and "\\" not in name,
                    "live_source", "runtime source path is unsafe")
            member = members.get(name)
            require(member is not None and member.isfile(), "live_source", "frozen runtime source is missing or indirect")
            stream = saved.extractfile(member)
            require(stream is not None, "live_source", "frozen runtime source cannot be read")
            with stream:
                require(hashlib.sha256(stream.read()).hexdigest() == sha,
                        "live_source", "frozen runtime source bytes differ from the build receipt")
    return {"binary_sha256": binary_sha, "runtime_source_files": len(runtime),
            "runtime_sources_sha256": hashlib.sha256(json.dumps(runtime, sort_keys=True, separators=(",", ":")).encode()).hexdigest(),
            "source_archive_sha256": build["source_archive_sha256"],
            "downloaded_runtime_matches_live_build": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--consumer-report", type=Path, required=True,
                        help="Successful verify_private_consumer.py report for this exact published candidate")
    parser.add_argument("--source-manifest", type=Path, required=True,
                        help="Successful build_release_eval.py frozen build receipt for this live run")
    parser.add_argument("--run-directory", type=Path, required=True,
                        help="Complete contextir-live-eval output directory with report.json")
    parser.add_argument("--fixture", type=Path, required=True,
                        help="Original explicit live suite input; image paths are relative to its parent")
    parser.add_argument("--report", type=Path, required=True, help="New absolute JSON report path")
    args = parser.parse_args()
    for path in (args.consumer_report, args.source_manifest, args.fixture):
        if not path.is_absolute() or not path.is_file():
            parser.error("all evidence paths must be existing absolute files")
    if not args.run_directory.is_absolute() or not args.run_directory.is_dir():
        parser.error("--run-directory must be an existing absolute directory")
    if not args.report.is_absolute() or args.report.exists() or not args.report.parent.is_dir():
        parser.error("--report must be a new absolute file in an existing directory")
    if "module-cache" in args.report.resolve().parts or args.report.resolve().is_relative_to(args.run_directory.resolve()):
        parser.error("reports cannot be placed in immutable module or live recording directories")
    work = Path(tempfile.mkdtemp(prefix="contextir-release-consumer-")).resolve()
    report = {"schema": "contextir-release-consumer/v1", "passed": False,
              "started_at": dt.datetime.now(dt.timezone.utc).isoformat(), "provider_calls": 0,
              "work_directory": str(work), "report_path": str(args.report),
              "consumer_report": str(args.consumer_report), "consumer_report_sha256": sha256_file(args.consumer_report),
              "source_manifest": str(args.source_manifest), "source_manifest_sha256": sha256_file(args.source_manifest),
              "run_directory": str(args.run_directory.resolve()), "fixture": str(args.fixture.resolve()),
              "fixture_sha256": sha256_file(args.fixture), "template_sha256": sha256_file(TEMPLATE),
              "launcher_sha256": sha256_file(Path(__file__)), "local_replace_allowed": False,
              "provider_credentials_forwarded": False, "workspace_disabled": True,
              "network_policy": "offline Go resolution and replay-only model; not an OS network firewall"}
    checked, initial_live = None, None
    try:
        checked = verify_consumer(args.consumer_report)
        require(not args.report.resolve().is_relative_to(checked["cache"]), "report_path", "report cannot modify module cache")
        report.update({"module": MODULE, "resolved_commit": checked["commit"], "resolved_version": checked["version"],
                       "module_sum": checked["module_sum"], "module_zip_sha256": checked["artifact_hashes"]["zip"],
                       "source_tree_sha256": checked["source_digest"]})
        run_directory = args.run_directory.resolve()
        initial_live = source_files(run_directory)
        live_path = run_directory / "report.json"
        live = decode_json(live_path.read_bytes(), "live_report")
        require(live.get("schema") == "contextir-live-eval-report/v1" and live.get("finished_at"),
                "live_report", "live recording is incomplete or has an unknown schema")
        require(live.get("fixture_sha256") == report["fixture_sha256"],
                "live_fixture", "explicit fixture differs from the live recording")
        manifest = decode_json(args.source_manifest.read_bytes(), "source_manifest")
        report["live_source"] = check_live_source(manifest, live, checked)
        report.update({"live_report_sha256": sha256_file(live_path), "live_acceptance_passed": live.get("passed") is True,
                       "live_assertions_applied": live.get("assertions_applied") is True,
                       "live_recording_files": len(initial_live),
                       "live_recording_tree_sha256": hashlib.sha256(json.dumps(initial_live, sort_keys=True, separators=(",", ":")).encode()).hexdigest()})
        go = shutil.which("go")
        require(go is not None, "go_executable", "Go executable is unavailable")
        env = sealed_environment(work, checked["cache"])
        consumer = work / "consumer"
        consumer.mkdir()
        (consumer / "go.mod").write_text("module consumer.invalid/contextir-release\n\ngo 1.25\n\nrequire " + MODULE + " " + checked["version"] + "\n")
        (consumer / "main.go").write_bytes(TEMPLATE.read_bytes())
        command([go, "mod", "tidy"], consumer, env, "consumer_tidy")
        declaration = sealed_json([go, "mod", "edit", "-json"], consumer, env, "consumer_module")
        require(not declaration.get("Replace"), "consumer_module", "consumer has a replacement")
        dependency = sealed_json([go, "list", "-m", "-json", MODULE], consumer, env, "consumer_dependency")
        require(dependency.get("Version") == checked["version"] and not dependency.get("Replace")
                and Path(dependency.get("Dir", "")).resolve() == checked["source"],
                "consumer_dependency", "consumer does not import the exact downloaded source")
        executable = work / "release-consumer"
        build_command = [go, "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", str(executable), "."]
        command(build_command, consumer, env, "consumer_build")
        report.update({"build_command": build_command, "consumer_executable_sha256": sha256_file(executable),
                       "consumer_go_mod_sha256": sha256_file(consumer / "go.mod"),
                       "import_source": {"directory": str(checked["source"]), "version": dependency["Version"], "replacement": None}})
        run_command = [str(executable), "--run-directory", str(run_directory), "--fixture", str(args.fixture.resolve()),
                       "--output", str(work / "persisted"), "--module-version", checked["version"]]
        result = sealed_json(run_command, consumer, env, "consumer_replay")
        require(result.get("passed") is True and result.get("module_version") == checked["version"]
                and result.get("provider_calls") == 0, "consumer_result", "external replay did not attest the expected result")
        report.update({"consumer": result, "run_command": run_command,
                       "go_environment": {name: env[name] for name in ("GOWORK", "GOENV", "GOFLAGS", "GOTOOLCHAIN", "GOPROXY", "GOSUMDB", "GOAUTH")},
                       "persisted_directory": str(work / "persisted"),
                       "scope": "Exact frozen live-runtime source and recording replay against a remotely downloaded module, actual disk persistence and Refine. Live semantic acceptance remains a separate outcome."})
        verify_unchanged(checked)
        require(source_files(run_directory) == initial_live, "recording_immutability", "live recording bytes changed during replay")
        report.update({"cache_unchanged": True, "recording_unchanged": True, "passed": True})
    except CheckFailure as error:
        report["failure"] = {"stage": error.stage, "reason": error.reason}
        if hasattr(error, "details"):
            report["failure"]["command_result"] = error.details
    except Exception as error:
        report["failure"] = {"stage": "launcher", "reason": "release replay failed; raw exception diagnostics withheld", "exception_type": type(error).__name__}
    finally:
        if checked is not None and "cache_unchanged" not in report:
            try:
                verify_unchanged(checked)
                report["cache_unchanged"] = True
            except Exception:
                report["cache_unchanged"] = False
                report["passed"] = False
        if initial_live is not None and "recording_unchanged" not in report:
            try:
                report["recording_unchanged"] = source_files(args.run_directory.resolve()) == initial_live
            except Exception:
                report["recording_unchanged"] = False
            if not report["recording_unchanged"]:
                report["passed"] = False
        report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
    with args.report.open("x", encoding="utf-8") as stream:
        json.dump(report, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    print(json.dumps({"passed": report["passed"], "report_path": str(args.report),
                      "resolved_commit": report.get("resolved_commit"),
                      "live_acceptance_passed": report.get("live_acceptance_passed"),
                      "failure_stage": report.get("failure", {}).get("stage")}))
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
