"""Only native pure style/summary formatters; full rendering stays authoritative Go H3.

The legacy Plan is just a transport DTO for the formatting helper's resolved
task/reference labels. This adapter does not create a second product renderer.
"""
import sys


def install(native_operation, plain):
    from h3ir import render as R
    functions = {
        'style_sentence': lambda phrase: native_operation('h3ir.render.style_sentence', {'phrase': phrase}),
        'style_prefix': lambda phrase: native_operation('h3ir.render.style_prefix', {'phrase': phrase}),
        'render_summary': lambda plan: native_operation('h3ir.render.render_summary', {'plan': plain(plan)}),
    }
    replacements = {getattr(R, name): value for name, value in functions.items()}
    for name, loaded in list(sys.modules.items()):
        if name.startswith('h3ir') and loaded is not None:
            for key, value in list(vars(loaded).items()):
                for original, replacement in replacements.items():
                    if value is original:
                        setattr(loaded, key, replacement)
    for name, value in functions.items():
        setattr(R, name, value)
