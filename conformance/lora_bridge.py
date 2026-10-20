"""Test-only adapters preserving original Python public object shapes and asserts.

Each replaced function executes the Go package. No original implementation is
used as a fallback. Extra Go weight provenance stays in bridge reports and is
omitted only when constructing the original dataclass, which has no such fields.
"""
import functools
import importlib
import inspect
import sys


def install(native_operation, plain):
    module = importlib.import_module("h3ir.lora")
    def record(value):
        if value is None:
            return None
        return module.LoraRecord(**{**value, "h3_variant": tuple(value["h3_variant"]),
                                   "triggers": tuple(value["triggers"]),
                                   "conflicts_with": tuple(value["conflicts_with"])})
    def decode(name, value):
        from h3ir.models import Finding, LoraChoice
        if name == "load_record":
            return record(value[0]), [Finding(**f) for f in value[1]]
        if name == "load_registry":
            return {k: record(v) for k, v in value[0].items()}, [Finding(**f) for f in value[1]], value[2]
        if name == "resolve_loras":
            keys = set(LoraChoice.__dataclass_fields__)
            return [LoraChoice(**{k: v for k, v in c.items() if k in keys}) for c in value[0]], [Finding(**f) for f in value[1]]
        if name == "_parse_front_matter":
            return tuple(value)
        return value
    def replace(owner, name, qualified, decode_name=None):
        original = getattr(owner, name)
        signature = inspect.signature(original)
        @functools.wraps(original)
        def wrapper(*args, **kwargs):
            bound = signature.bind(*args, **kwargs); bound.apply_defaults()
            arguments = dict(bound.arguments)
            if name in ("load_registry", "resolve_loras"):
                from h3ir.config import get_config
                arguments["roots"] = [str(d) for d in get_config().paths.lora_dirs]
            result = native_operation(qualified, arguments)
            return decode(decode_name or name, result)
        for module_name, loaded in list(sys.modules.items()):
            if module_name.startswith("h3ir") and loaded is not None:
                for key, value in list(vars(loaded).items()):
                    if value is original:
                        setattr(loaded, key, wrapper)
        setattr(owner, name, wrapper)
    for name in ("trigger_problem", "load_record", "load_registry", "resolve_loras", "_constraint_violation", "_parse_front_matter"):
        replace(module, name, "h3ir.lora." + name)
    for name in ("what_for", "when_not", "public"):
        replace(module.LoraRecord, name, "h3ir.lora.LoraRecord." + name)
    comfy = importlib.import_module("h3ir.comfy")
    for name in ("find_h3_nodes", "set_prompt", "set_seed", "describe_graph"):
        original = getattr(comfy, name); signature = inspect.signature(original)
        def make(fn, sig, name):
            @functools.wraps(fn)
            def wrapped(*args, **kwargs):
                bound = sig.bind(*args, **kwargs); bound.apply_defaults()
                try:
                    return native_operation("h3ir.comfy." + name, dict(bound.arguments))
                except ValueError as error:
                    raise comfy.ComfyError(str(error)) from error
            return wrapped
        wrapper = make(original, signature, name)
        for module_name, loaded in list(sys.modules.items()):
            if module_name.startswith("h3ir") and loaded is not None:
                for key, value in list(vars(loaded).items()):
                    if value is original:
                        setattr(loaded, key, wrapper)
        setattr(comfy, name, wrapper)
    def outputs(history):
        try:
            return native_operation("h3ir.comfy.Comfy.outputs", {"history": history})
        except ValueError as error:
            raise comfy.ComfyError(str(error)) from error
    comfy.Comfy.outputs = staticmethod(outputs)
