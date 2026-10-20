#!/usr/bin/env python3
"""Extract factual input names from a pinned ComfyUI source file, without executing it.

Usage: python3 extract_comfyui_contract.py downloaded_nodes.py COMMIT > comfyui_inputs.json
No upstream implementation code is copied into the resulting fixture.
"""
import ast
import hashlib
import json
import pathlib
import sys

source = pathlib.Path(sys.argv[1]).read_bytes()
commit = sys.argv[2]
nodes = {}
for cls in ast.parse(source).body:
    if not isinstance(cls, ast.ClassDef) or cls.name not in (
        "MiniMaxH3ImageToVideo", "MiniMaxH3ReferenceToVideo"
    ):
        continue
    schema = next(n for n in cls.body if isinstance(n, ast.FunctionDef) and n.name == "define_schema")
    images = []
    prefixes = []
    integers = {}
    for call in ast.walk(schema):
        if not isinstance(call, ast.Call):
            continue
        if ast.unparse(call.func) == "io.Image.Input":
            images.append(ast.literal_eval(call.args[0]))
        elif ast.unparse(call.func) == "io.Int.Input":
            integers[ast.literal_eval(call.args[0])] = {kw.arg: ast.literal_eval(kw.value) for kw in call.keywords if kw.arg in ("min", "max", "step") and isinstance(kw.value, ast.Constant)}
        elif ast.unparse(call.func) == "io.Autogrow.TemplatePrefix":
            prefixes.append({kw.arg: ast.literal_eval(kw.value) for kw in call.keywords if kw.arg in ("prefix", "min", "max")})
    nodes[cls.name] = {"image_inputs": images, "prefixes": prefixes, "integer_inputs": integers}
print(json.dumps({
    "source": "https://github.com/Comfy-Org/ComfyUI/blob/" + commit + "/comfy_extras/nodes_minimax_h3.py",
    "commit": commit,
    "source_sha256": hashlib.sha256(source).hexdigest(),
    "nodes": nodes,
}, indent=2))
