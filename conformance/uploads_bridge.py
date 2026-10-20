"""Original store assertions against persistent native Store/Receiver objects."""
import base64
import functools
import importlib
import inspect
from pathlib import Path
import sys


def install(native_operation):
    module=importlib.import_module('h3ir.uploads')
    classes={
        'asset-name-not-a-digest':module.NotADigest,
        'asset-too-large':module.TooLarge,
        'upload-store-full':module.StoreFull,
        'asset-digest-mismatch':module.DigestMismatch,
    }
    def invoke(name,arguments):
        from h3ir.config import get_config
        cfg=get_config()
        arguments={**arguments,'config':{'root':str(cfg.paths.uploads_dir()),'max_bytes':cfg.assets.upload_max_bytes,'store_bytes':cfg.assets.upload_store_bytes,'ttl_hours':cfg.assets.upload_ttl_hours}}
        try:return native_operation('h3ir.uploads.'+name,arguments)
        except ValueError as error:
            cls=classes.get(getattr(error,'code',None),module.UploadRefused)
            raise cls(str(error)) from error
    class Receiver:
        def __init__(self,id):self.id=id
        def write(self,data):invoke('Receiver.write',{'id':self.id,'data':base64.b64encode(data).decode('ascii')})
        def finish(self):
            path,size=invoke('Receiver.finish',{'id':self.id});return Path(path),size
        def abort(self):invoke('Receiver.abort',{'id':self.id})
        def __enter__(self):return self
        def __exit__(self,*args):self.abort()
    def replace(name):
        original=getattr(module,name);signature=inspect.signature(original)
        @functools.wraps(original)
        def wrapper(*args,**kwargs):
            bound=signature.bind(*args,**kwargs);bound.apply_defaults();a=dict(bound.arguments)
            if name in ('path_for','stored'):a['digest']=a.pop('sha256')
            if name=='receiver':a['digest']=a.pop('claimed')
            if name=='touch':a['path']=str(a['path'])
            value=invoke(name,a)
            if name=='receiver':return Receiver(value)
            if name in ('path_for','stored') and value is not None:return Path(value)
            return value
        for n,loaded in list(sys.modules.items()):
            if n.startswith('h3ir') and loaded is not None:
                for key,value in list(vars(loaded).items()):
                    if value is original:setattr(loaded,key,wrapper)
        setattr(module,name,wrapper)
    for name in ('path_for','stored','missing','touch','total_bytes','sweep','receiver'):replace(name)
