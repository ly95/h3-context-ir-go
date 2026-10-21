"""Observe unmodified upstream pytest runs; no assertion or return value is changed.

Loaded before collection so ``from h3ir.validate import validate`` sees this wrapper.
The original Python validator still executes. Its success never counts as Go success.
"""
from __future__ import annotations

import ast
import dataclasses
import enum
import functools
import importlib
import inspect
import json
import os
from pathlib import Path
import platform
import time

STATE = {"collected": [], "reports": [], "validator_calls": [], "compiler_calls": [], "budget_calls": [], "shots_calls": [], "lora_calls": [], "comfy_calls": []}
CURRENT = None
CAPTURE_ORIGIN = "upstream_pytest"
START = time.time()


def plain(value):
    if dataclasses.is_dataclass(value) and not isinstance(value, type):
        return plain(dataclasses.asdict(value))
    if isinstance(value, enum.Enum):
        return value.value
    if isinstance(value, dict):
        return {str(k): plain(v) for k, v in value.items()}
    if isinstance(value, (list, tuple)):
        return [plain(v) for v in value]
    if isinstance(value, (set, frozenset)):
        return sorted(plain(v) for v in value)
    if isinstance(value, Path):
        return str(value)
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    # Backend handles and test mocks must not leak repr(), config or credentials.
    return {"unserialized_type": type(value).__module__ + "." + type(value).__name__}



def capture_function(fn, qualified, category):
    signature = inspect.signature(fn)
    @functools.wraps(fn)
    def wrapper(*args, **kwargs):
        global CAPTURE_ORIGIN
        bound = signature.bind(*args, **kwargs)
        bound.apply_defaults()
        record = {"id": f"{category}-{len(STATE[category]) + 1:05d}",
                  "nodeid": CURRENT, "function": qualified, "origin": CAPTURE_ORIGIN,
                  "arguments": plain(dict(bound.arguments))}
        STATE[category].append(record)
        try:
            result = fn(*args, **kwargs)
        except Exception as error:
            record["raised"] = type(error).__module__ + "." + type(error).__name__
            raise
        record["result"] = plain(result)
        if type(result).__module__ == "h3ir.grid" and type(result).__name__ == "Target":
            previous_origin = CAPTURE_ORIGIN
            CAPTURE_ORIGIN = "supplementary_target_property_probe"
            try:
                record["target_properties"] = {name: plain(getattr(result, name)) for name in
                    ("effective_seconds", "latent_t", "video_rows", "audio_rows", "in_trained_range")}
                record["target_properties"]["s_ss_snapped"] = result.s_ss("snapped")
                record["target_properties"]["s_ss_nominal"] = result.s_ss("nominal")
            finally:
                CAPTURE_ORIGIN = previous_origin
        return result
    return wrapper


def pytest_configure(config):
    # Observe LoRA before compile imports its aliases; keep original returns unchanged.
    lora_module = importlib.import_module("h3ir.lora")
    for name in ("_parse_front_matter", "_inline_map", "_scalar", "trigger_problem", "load_record", "load_registry", "resolve_loras", "_file_sha", "_constraint_violation"):
        setattr(lora_module, name, capture_function(getattr(lora_module, name), "h3ir.lora." + name, "lora_calls"))
    for name in ("what_for", "when_not", "public"):
        setattr(lora_module.LoraRecord, name, capture_function(getattr(lora_module.LoraRecord, name), "h3ir.lora.LoraRecord." + name, "lora_calls"))
    # Install before validate/plan import aliases of these pure functions.
    grid = importlib.import_module("h3ir.grid")
    for name in ("align_frame_count", "video_latent_t", "audio_latent_t", "legal_frames",
                 "frames_for_seconds", "rows_per_latent_frame", "adapt_canvas", "canvas_for_aspect",
                 "s_ss_text", "instruction_line_for", "ms_to_timestamp", "timestamp_to_ms"):
        setattr(grid, name, capture_function(getattr(grid, name), "h3ir.grid." + name, "budget_calls"))
    grid.Target.build = classmethod(capture_function(grid.Target.__dict__["build"].__func__,
                                                     "h3ir.grid.Target.build", "budget_calls"))
    tokens = importlib.import_module("h3ir.tokens")
    tokens.count = capture_function(tokens.count, "h3ir.tokens.count", "budget_calls")
    validate_module = importlib.import_module("h3ir.validate")
    original = validate_module.validate

    @functools.wraps(original)
    def capture_validate(text, ctx=None, **kwargs):
        effective = ctx or validate_module.Context(**kwargs)
        findings = original(text, ctx, **kwargs)
        STATE["validator_calls"].append({
            "id": f"validate-{len(STATE['validator_calls']) + 1:05d}",
            "nodeid": CURRENT,
            "text": text,
            "context": plain(effective),
            "findings": plain(findings),
        })
        return findings

    validate_module.validate = capture_validate
    for module_name, functions in {
        "h3ir.mode": ("infer_mode",),
        "h3ir.plan": ("build_manifest", "build_subjects", "build_speakers", "allocate_shots", "build_plan", "derive_task_types"),
    }.items():
        module = importlib.import_module(module_name)
        for name in functions:
            fn = getattr(module, name)
            signature = inspect.signature(fn)

            def make_wrapper(fn, signature, qualified):
                @functools.wraps(fn)
                def wrapper(*args, **kwargs):
                    bound = signature.bind(*args, **kwargs)
                    bound.apply_defaults()
                    record = {"id": f"compiler-{len(STATE['compiler_calls']) + 1:05d}",
                              "nodeid": CURRENT, "function": qualified,
                              "arguments": plain(dict(bound.arguments))}
                    # Append before invoking: nested calls retain deterministic IDs.
                    STATE["compiler_calls"].append(record)
                    try:
                        result = fn(*args, **kwargs)
                    except Exception as error:
                        record["raised"] = type(error).__module__ + "." + type(error).__name__
                        raise
                    record["result"] = plain(result)
                    return result
                return wrapper
            setattr(module, name, make_wrapper(fn, signature, module_name + "." + name))

    for module_name, names in {"h3ir.shots": ("validate_proposal", "spans", "shot_schema"),
                               "h3ir.draft": ("draft_camera",)}.items():
        module = importlib.import_module(module_name)
        for name in names:
            setattr(module, name, capture_function(getattr(module, name), module_name + "." + name, "shots_calls"))


    comfy_module = importlib.import_module("h3ir.comfy")
    for name in ("find_h3_nodes", "set_prompt", "set_seed", "describe_graph"):
        comfy_original = getattr(comfy_module, name)
        wrapped = capture_function(comfy_original, "h3ir.comfy." + name, "comfy_calls")
        for module_name, loaded in list(__import__("sys").modules.items()):
            if module_name.startswith("h3ir") and loaded is not None:
                for key, value in list(vars(loaded).items()):
                    if value is comfy_original:
                        setattr(loaded, key, wrapped)
        setattr(comfy_module, name, wrapped)
    comfy_module.Comfy.outputs = staticmethod(capture_function(comfy_module.Comfy.outputs, "h3ir.comfy.Comfy.outputs", "comfy_calls"))


def pytest_collection_modifyitems(session, config, items):
    root = Path(str(config.rootpath))
    for item in items:
        path = Path(str(item.path))
        source = path.read_text(encoding="utf-8")
        function = item.originalname or item.name.split("[")[0]
        nodes = [n for n in ast.walk(ast.parse(source)) if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef)) and n.name == function]
        node = nodes[0] if nodes else None
        assertions = [] if node is None else [
            {"line": n.lineno, "source": ast.get_source_segment(source, n)}
            for n in ast.walk(node) if isinstance(n, ast.Assert)
        ]
        STATE["collected"].append({
            "nodeid": item.nodeid,
            "source": str(path.relative_to(root)),
            "function": function,
            "line_start": node.lineno if node else item.location[1] + 1,
            "line_end": node.end_lineno if node else None,
            "parameters": plain(getattr(getattr(item, "callspec", None), "params", {})),
            "assertions": assertions,
            "go_status": "pending",
        })


def pytest_runtest_logstart(nodeid, location):
    global CURRENT
    CURRENT = nodeid


def pytest_runtest_logfinish(nodeid, location):
    global CURRENT
    CURRENT = None


def pytest_runtest_logreport(report):
    record = {"nodeid": report.nodeid, "phase": report.when,
              "outcome": report.outcome, "duration_s": report.duration}
    if report.failed or report.skipped:
        record["detail"] = str(report.longrepr)
    if hasattr(report, "wasxfail"):
        record["wasxfail"] = report.wasxfail
    STATE["reports"].append(record)


def pytest_sessionfinish(session, exitstatus):
    output = os.environ.get("CONTEXTIR_CAPTURE_PATH")
    if not output:
        return
    by_node = {}
    for report in STATE["reports"]:
        by_node.setdefault(report["nodeid"], []).append(report)
    counts = {}
    for item in STATE["collected"]:
        phases = by_node.get(item["nodeid"], [])
        item["phases"] = phases
        item["upstream_status"] = (
            "failed" if any(p["outcome"] == "failed" for p in phases) else
            "skipped" if any(p["outcome"] == "skipped" for p in phases) else
            "passed" if any(p["phase"] == "call" and p["outcome"] == "passed" for p in phases) else
            "not_run")
        counts[item["upstream_status"]] = counts.get(item["upstream_status"], 0) + 1
    payload = {"schema_version": 1, "implementation_under_test": "upstream Python (not Go)",
               "python": platform.python_version(), "pytest_exit_status": int(exitstatus),
               "duration_s": time.time() - START, "counts": counts, **STATE}
    Path(output).write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
