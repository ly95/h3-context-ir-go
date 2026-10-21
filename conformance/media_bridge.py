"""Native media header/probe/cache sampling on the original test's live files.

No synthetic path fixture substitutes for an original temporary file. The Go
process reads the exact path while the upstream test owns it.
"""
import functools
import importlib
import inspect
from pathlib import Path
import shutil
import subprocess
import sys

ORIGINAL_RUN=subprocess.run

def install(native_operation):
    module=importlib.import_module('h3ir.analyse')
    def replace(name):
        original=getattr(module,name);signature=inspect.signature(original)
        @functools.wraps(original)
        def wrapper(*args,**kwargs):
            bound=signature.bind(*args,**kwargs);bound.apply_defaults();arguments=dict(bound.arguments)
            arguments['path']=str(Path(arguments['path']).resolve())
            if name in ('probe_seconds','sample_frames'):
                # Original tests inject a missing executor by object replacement.
                # Translate that explicit injected dependency into the Go tool
                # boundary, rather than depending on Python monkeypatching Go.
                missing=subprocess.run is not ORIGINAL_RUN
                arguments['ffprobe']='/nonexistent/contextir/ffprobe' if missing else shutil.which('ffprobe')
                arguments['ffmpeg']='/nonexistent/contextir/ffmpeg' if missing else shutil.which('ffmpeg')
            if name=='sample_frames':
                from h3ir.config import get_config
                arguments['cache_dir']=str(get_config().paths.cache_dir()/'go-sampled-frames')
            try:
                value=native_operation('h3ir.analyse.'+name,arguments)
            except ValueError as error:
                if name in ('probe_seconds','sample_frames') and missing and getattr(error,'code',None)=='unsupported_capability':
                    raise module.ToolMissing(str(error)) from error
                raise module.AssetAnalysisError(str(error)) from error
            if name=='image_size' and value is not None:return tuple(value)
            return value
        for n,loaded in list(sys.modules.items()):
            if n.startswith('h3ir') and loaded is not None:
                for key,value in list(vars(loaded).items()):
                    if value is original:setattr(loaded,key,wrapper)
        setattr(module,name,wrapper)
    for name in ('image_size','image_mime','sniff_container','probe_seconds','sample_frames'):replace(name)
