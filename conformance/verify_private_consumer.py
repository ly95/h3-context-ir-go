#!/usr/bin/env python3
"""Verify a pushed private ContextIR commit from an isolated consumer module.

No local replace/workspace references. Run only after the requested full commit
has been pushed. Existing Git authentication is used without reading credentials.
All subprocess output is captured; raw Git/Go diagnostics are never persisted or
printed because an authentication failure can contain a credential-bearing URL.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

MODULE = "github.com/ly95/contextir"
GO_MAIN = r'''package main

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "runtime/debug"

    contextir "github.com/ly95/contextir"
    "github.com/ly95/contextir/evaluate"
    "github.com/ly95/contextir/h3"
)

func digest(b []byte) string {
    h := sha256.Sum256(b)
    return hex.EncodeToString(h[:])
}

func run() error {
    // A deterministic consumer smoke case, not an upstream conformance fixture.
    doc := contextir.Document{
        Version: contextir.Version,
        Mode: contextir.ModeText,
        Target: contextir.Target{
            Duration: 8000,
            AspectRatio: "16:9",
            FPS: contextir.Rational{Numerator: 24, Denominator: 1},
        },
        Style: "live-action photographic",
        Shots: []contextir.Shot{{
            ID: "opening", Start: 0, End: 8000,
            Description: contextir.Text("A ceramic cup rests on a table. The camera holds a static shot."),
        }},
        Soundscape: "Rain taps against the window.",
        Music: "N/A",
    }
    artifact, err := h3.Compile(doc, h3.OfficialProfile())
    if err != nil { return fmt.Errorf("compile: %w", err) }
    encoded, err := contextir.Encode(doc)
    if err != nil { return fmt.Errorf("encode: %w", err) }
    decoded, err := contextir.Decode(bytes.NewReader(encoded))
    if err != nil { return fmt.Errorf("decode: %w", err) }
    replayJSON, err := contextir.Encode(decoded)
    if err != nil { return fmt.Errorf("re-encode: %w", err) }
    if !bytes.Equal(encoded, replayJSON) { return errors.New("document replay changed canonical bytes") }
    replay, err := h3.Compile(decoded, h3.OfficialProfile())
    if err != nil { return fmt.Errorf("recompile: %w", err) }
    artifactJSON, err := json.Marshal(artifact)
    if err != nil { return err }
    replayArtifactJSON, err := json.Marshal(replay)
    if err != nil { return err }
    if !bytes.Equal(artifactJSON, replayArtifactJSON) { return errors.New("artifact replay changed bytes") }
    if artifact.DocumentSHA256 != digest(encoded) || artifact.PromptSHA256 != digest([]byte(artifact.Prompt)) {
        return errors.New("published document/prompt hashes do not match actual bytes")
    }
    score, err := evaluate.ScoreArtifact(artifact, evaluate.ArtifactOptions{
        Name: "private-module-consumer",
        Plan: evaluate.Plan{Shots: []evaluate.Shot{{Body: "A ceramic cup rests on a table. The camera holds a static shot."}}},
        Source: "deterministic-consumer-smoke",
    })
    if err != nil { return fmt.Errorf("score: %w", err) }
    aggregate := evaluate.AggregateScores([]evaluate.Score{score})
    comparison, err := evaluate.Compare(aggregate, aggregate)
    if err != nil || comparison.Regressed { return errors.New("self comparison failed") }
    gate, err := evaluate.Gate(evaluate.Run{Scores: []evaluate.Score{score}, Aggregate: aggregate}, nil)
    if err != nil || !score.OK || !gate.OK { return errors.New("offline evaluation gate failed") }
    if score.Shots != 1 || score.Tokens <= 0 { return errors.New("offline evaluation did not measure shipped prompt") }
    info, ok := debug.ReadBuildInfo()
    if !ok { return errors.New("Go build information missing") }
    moduleVersion := ""
    for _, dep := range info.Deps {
        if dep.Path == "github.com/ly95/contextir" {
            if dep.Replace != nil { return errors.New("consumer dependency has a replacement") }
            moduleVersion = dep.Version
        }
    }
    if moduleVersion == "" || moduleVersion == "(devel)" { return errors.New("remote module version missing") }
    return json.NewEncoder(os.Stdout).Encode(map[string]any{
        "module_version": moduleVersion,
        "go_version": info.GoVersion,
        "document_version": doc.Version,
        "artifact_version": artifact.Version,
        "compiler_version": artifact.CompilerVersion,
        "format_version": artifact.FormatVersion,
        "mode": artifact.Mode,
        "frames": artifact.Frames,
        "effective_duration_seconds": artifact.EffectiveDurationSeconds,
        "canonical_document_bytes": len(encoded),
        "canonical_document_sha256": digest(encoded),
        "artifact_json_sha256": digest(artifactJSON),
        "artifact_sha256": artifact.SHA256,
        "profile_sha256": artifact.ProfileSHA256,
        "prompt_sha256": artifact.PromptSHA256,
        "manifest_sha256": artifact.ManifestSHA256,
        "canonical_replay_identical": true,
        "artifact_replay_identical": true,
        "score": score,
        "gate_ok": gate.OK,
        "self_comparison_regressed": comparison.Regressed,
        "scope": "Deterministic public Go API smoke only; no provider, media IO or upstream assertion claim.",
    })
}

func main() {
    if err := run(); err != nil {
        // All data in this program is the synthetic smoke case. No credentials
        // or caller content enter it; the launcher still captures diagnostics.
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
'''


class CheckFailure(Exception):
    def __init__(self, stage: str, reason: str, exit_status: int | None = None):
        self.stage, self.reason, self.exit_status = stage, reason, exit_status
        super().__init__(reason)


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for part in iter(lambda: f.read(1024 * 1024), b""):
            h.update(part)
    return h.hexdigest()


def execute(args: list[str], *, stage: str, cwd: Path, env: dict[str, str]) -> str:
    try:
        p = subprocess.run(args, cwd=cwd, env=env, capture_output=True, text=True, timeout=600, check=False)
    except subprocess.TimeoutExpired:
        raise CheckFailure(stage, "command timed out; raw diagnostics withheld") from None
    except OSError:
        raise CheckFailure(stage, "command could not start; raw diagnostics withheld") from None
    if p.returncode:
        raise CheckFailure(stage, "command failed; raw diagnostics withheld to avoid credential disclosure", p.returncode)
    return p.stdout


def parse_json(value: str, stage: str):
    try:
        return json.loads(value)
    except (ValueError, TypeError):
        raise CheckFailure(stage, "expected machine-readable JSON was not returned") from None


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--commit", required=True, help="Already-pushed full 40-character commit SHA (no branch/default)")
    parser.add_argument("--version", help="Optional exact release tag to install; its origin must equal --commit")
    parser.add_argument("--report", type=Path, help="New absolute JSON report path; existing files are never overwritten")
    parser.add_argument("--test-plan", action="store_true", help="Also run the downloaded plan package TestUpstreamCorpus without modifying the module cache")
    parser.add_argument("--module-cache", type=Path, help="Optional existing external cache from an earlier consumer run (never a worktree)")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-fA-F]{40}", args.commit):
        parser.error("--commit must be a full 40-character hexadecimal SHA")
    commit = args.commit.lower()
    if args.version is not None and not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?", args.version):
        parser.error("--version must be an exact semantic release tag")
    requested_revision = args.version or commit
    if args.report is not None:
        if not args.report.is_absolute():
            parser.error("--report must be an absolute path")
        if args.report.exists():
            parser.error("--report already exists")
        if not args.report.parent.is_dir():
            parser.error("--report parent directory must already exist")
    work = Path(tempfile.mkdtemp(prefix="contextir-private-consumer-")).resolve()
    module_dir = work / "consumer"
    module_dir.mkdir()
    cache = args.module_cache.resolve() if args.module_cache is not None else work / "module-cache"
    if args.module_cache is not None:
        if not args.module_cache.is_absolute() or not cache.is_dir():
            parser.error("--module-cache must be an existing absolute external module cache")
        if not cache.name == "module-cache" or not cache.parent.name.startswith("contextir-private-consumer-"):
            parser.error("--module-cache must come from an earlier external consumer check")
    else:
        cache.mkdir()
    report_path = args.report or (work / "report.json")
    env = dict(os.environ)
    # This isolates module/build caches and disables parent workspaces/go env
    # overrides. Authentication remains the user's existing standard Git setup;
    # this program never inspects or prints it and never changes Git config.
    env.update({
        "GOWORK": "off", "GOENV": "off", "GOFLAGS": "", "GOTOOLCHAIN": "local",
        "GOPATH": str(work / "gopath"), "GOMODCACHE": str(cache),
        "GOCACHE": str(work / "build-cache"), "GOPROXY": "direct",
        "GOPRIVATE": MODULE, "GONOPROXY": MODULE, "GONOSUMDB": MODULE,
        "GIT_TERMINAL_PROMPT": "0",
    })
    report = {
        "schema": "contextir-private-consumer/v1",
        "started_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "module": MODULE, "requested_commit": commit, "requested_version": args.version,
        "work_directory": str(work), "report_path": str(report_path),
        "isolated_module_cache": True, "module_cache_reused": args.module_cache is not None, "workspace_disabled": True,
        "local_replace_allowed": False, "provider_calls": 0,
        "fixture_scope": "Synthetic deterministic consumer smoke, not original upstream tests.",
        "passed": False,
    }
    try:
        execute(["go", "mod", "init", "consumer.invalid/contextir-check"], stage="module_init", cwd=module_dir, env=env)
        raw = execute(["go", "mod", "download", "-json", MODULE + "@" + requested_revision], stage="remote_download", cwd=module_dir, env=env)
        downloaded = parse_json(raw, "remote_download")
        if downloaded.get("Error"):
            raise CheckFailure("remote_download", "module download returned an error; raw diagnostics withheld")
        version = downloaded.get("Version", "")
        origin = downloaded.get("Origin", {})
        actual_commit = origin.get("Hash", "").lower()
        if downloaded.get("Path") != MODULE or actual_commit != commit:
            raise CheckFailure("origin_verification", "downloaded module origin does not equal requested commit")
        if args.version is not None and version != args.version:
            raise CheckFailure("version_verification", "downloaded version differs from the requested exact tag")
        if not version.startswith("v"):
            raise CheckFailure("version_verification", "downloaded module has no canonical version")
        zip_path = Path(downloaded.get("Zip", "")).resolve()
        if not zip_path.is_file() or not zip_path.is_relative_to(cache):
            raise CheckFailure("zip_verification", "module zip is not in isolated download cache")
        report.update({
            "resolved_version": version, "resolved_commit": actual_commit,
            "module_sum": downloaded.get("Sum"), "go_mod_sum": downloaded.get("GoModSum"),
            "module_zip_bytes": zip_path.stat().st_size, "module_zip_sha256": sha256_file(zip_path),
        })
        execute(["go", "mod", "edit", "-require=" + MODULE + "@" + version], stage="require_version", cwd=module_dir, env=env)
        (module_dir / "main.go").write_text(GO_MAIN, encoding="utf-8")
        execute(["gofmt", "-w", "main.go"], stage="gofmt_consumer", cwd=module_dir, env=env)
        execute(["go", "mod", "tidy"], stage="module_tidy", cwd=module_dir, env=env)
        main_mod = parse_json(execute(["go", "mod", "edit", "-json"], stage="main_module_verification", cwd=module_dir, env=env), "main_module_verification")
        if main_mod.get("Replace"):
            raise CheckFailure("main_module_verification", "main module unexpectedly contains replace directives")
        dep = parse_json(execute(["go", "list", "-m", "-json", MODULE], stage="dependency_verification", cwd=module_dir, env=env), "dependency_verification")
        dependency_dir = Path(dep.get("Dir", "")).resolve()
        if dep.get("Replace") or dep.get("Version") != version or not dependency_dir.is_relative_to(cache):
            raise CheckFailure("dependency_verification", "build dependency is not the downloaded canonical module")
        go_env = parse_json(execute(["go", "env", "-json", "GOMOD", "GOWORK", "GOMODCACHE"], stage="workspace_verification", cwd=module_dir, env=env), "workspace_verification")
        if go_env.get("GOWORK") != "off" or Path(go_env.get("GOMOD", "")).resolve() != module_dir / "go.mod" or Path(go_env.get("GOMODCACHE", "")).resolve() != cache:
            raise CheckFailure("workspace_verification", "Go is not using the isolated consumer module/cache")
        report["import_source"] = {"path": dep.get("Path"), "version": dep.get("Version"), "directory": str(dependency_dir), "module_cache": str(cache), "replacement": None, "main_module": str(module_dir / "go.mod")}
        executable = work / "consumer-check"
        execute(["go", "build", "-mod=readonly", "-o", str(executable), "."], stage="consumer_build", cwd=module_dir, env=env)
        result = parse_json(execute([str(executable)], stage="consumer_run", cwd=module_dir, env=env), "consumer_run")
        report["consumer_executable_sha256"] = sha256_file(executable)
        if result.get("module_version") != version or result.get("gate_ok") is not True or result.get("artifact_replay_identical") is not True:
            raise CheckFailure("consumer_verification", "consumer output did not attest the required version/replay/gate")
        if args.test_plan:
            execute(["go", "test", "-mod=readonly", MODULE + "/plan", "-run", "^TestUpstreamCorpus$", "-count=1"], stage="plan_upstream_corpus_test", cwd=module_dir, env=env)
            report["plan_upstream_corpus_test"] = {"passed": True, "test": "TestUpstreamCorpus", "count": 1, "module_cache_source": True}
        report.update({
            "passed": True, "consumer": result,
            "consumer_source_sha256": sha256_file(module_dir / "main.go"),
            "consumer_go_mod_sha256": sha256_file(module_dir / "go.mod"),
            "go_version": execute(["go", "version"], stage="go_version", cwd=module_dir, env=env).strip(),
            "verified_operations": ["remote commit download", "isolated module resolution without replace", "go build -mod=readonly", "run compiled consumer executable", "Document to h3.Compile", "Encode/Decode canonical replay", "artifact replay", "document/prompt hash verification", "ScoreArtifact", "AggregateScores", "Compare", "Gate"],
        })
    except CheckFailure as failure:
        report["failure"] = {"stage": failure.stage, "reason": failure.reason, "exit_status": failure.exit_status}
    except Exception as failure:
        # Do not interpolate exception text: external tools may put secrets in it.
        report["failure"] = {"stage": "launcher", "reason": "launcher error; exception text withheld", "exception_type": type(failure).__name__}
    report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
    try:
        with report_path.open("x", encoding="utf-8") as f:
            json.dump(report, f, ensure_ascii=False, indent=2)
            f.write("\n")
    except OSError:
        print("Could not write new report; no raw diagnostics exposed.", file=sys.stderr)
        return 2
    print(json.dumps({"passed": report["passed"], "report_path": str(report_path), "resolved_version": report.get("resolved_version"), "module_zip_bytes": report.get("module_zip_bytes"), "failure_stage": report.get("failure", {}).get("stage")}, ensure_ascii=False))
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
