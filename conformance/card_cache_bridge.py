"""Native observation-cache policy and per-request audio metadata, live source files.

The source visual analyzer remains an explicit injected collaborator on a miss;
no Python cache policy or audio-card algorithm is used.
"""
import dataclasses
import functools
import hashlib
import importlib
import inspect
import json
from pathlib import Path
import sys

PENDING={}
NEXT=0

def install(native_operation,plain):
    module=importlib.import_module('h3ir.analyse')
    def directory():
        from h3ir.config import get_config
        return str(get_config().paths.cache_dir()/'go-observation-cards')
    def identity(ref,model):
        contract={'version':module.ANALYZER_VERSION,'image_instructions':module.IMAGE_INSTRUCTIONS,'image_schema':module.IMAGE_SCHEMA,'video_schema':module.VIDEO_SCHEMA,'camera_question':module.CAMERA_QUESTION,'sampling_fractions':module.VIDEO_FRAME_FRACTIONS,
            'visual_input':{'note':ref.note,'provenance':ref.provenance,'px':ref.px}}
        digest=hashlib.sha256(json.dumps(contract,ensure_ascii=False,sort_keys=True,separators=(',',':')).encode()).hexdigest()
        return {'source_sha256':ref.sha256,'kind':ref.kind.value,'asset_id':ref.sha256,'model_identity':model,'analyzer_version':module.ANALYZER_VERSION,'contract_sha256':digest}
    def card(raw):
        fields={f.name for f in dataclasses.fields(module.AssetCard)}
        raw={k:v for k,v in raw.items() if k in fields};raw['kind']=module.AssetKind(raw['kind'])
        return module.AssetCard(**raw)
    import callbacks
    def observe(payload):
        backend,refs,seed=PENDING[payload['callback_id']];ref=refs[payload['index']]
        try:
            if ref.kind is module.AssetKind.IMAGE:value=module.analyse_image(backend,ref,seed=seed)
            elif ref.kind is module.AssetKind.VIDEO:value=module.analyse_video(backend,ref,seed=seed)
            else:raise RuntimeError('Audio must never reach the visual observation callback')
            return {'value':plain(value)}
        except module.AssetAnalysisError as error:return {'error':{'kind':type(error).__name__,'message':str(module._name_the_asset(error,ref))}}
    callbacks.register('media.visual_observation',observe)
    for name in ('_cache_key','_cache_path','save_cached','load_cached','analyse_audio','analyse_all'):
        original=getattr(module,name);signature=inspect.signature(original)
        def wrap(name,original,signature):
            @functools.wraps(original)
            def invoke(*args,**kwargs):
                global NEXT
                bound=signature.bind(*args,**kwargs);bound.apply_defaults();a=bound.arguments
                data={'directory':directory()}
                if name=='_cache_path':data['digest']=a['key']
                elif name=='analyse_all':
                    NEXT+=1;id=NEXT;refs=a['refs'];PENDING[id]=(a['backend'],refs,a['seed'])
                    data.update(items=[{'ref':ref,'key':identity(ref,a['backend'].cfg.model if a['use_cache'] and ref.kind is not module.AssetKind.AUDIO else ''),'transcript':(a['transcripts']or{}).get(ref.sha256,'')} for ref in refs],use_cache=a['use_cache'],callback_id=id)
                elif name=='analyse_audio':
                    ref=a['ref'];data.update(ref=ref,transcript=a['transcript'])
                    if a['duration_s'] is not None:
                        data['ref']=dataclasses.replace(ref,seconds=a['duration_s'])
                else:data.update(ref=a['ref'],key=identity(a['ref'],a['model']))
                if name=='save_cached':data['card']=a['card']
                try:value=native_operation('h3ir.analyse.'+name,data)
                except ValueError as error:
                    if getattr(error,'code',None)=='unsupported_capability':raise module.ToolMissing(str(error)) from error
                    raise module.AssetAnalysisError(str(error)) from error
                finally:
                    if name=='analyse_all':PENDING.pop(id,None)
                if name=='_cache_path':return Path(value)
                if name=='load_cached':return card(value['card']) if value.get('card') is not None else None
                if name=='save_cached':return None
                if name=='analyse_audio':return card(value)
                if name=='analyse_all':return {k:card(v) for k,v in value['cards'].items()}
                return value
            return invoke
        replacement=wrap(name,original,signature)
        for n,loaded in list(sys.modules.items()):
            if n.startswith('h3ir') and loaded is not None:
                for key,value in list(vars(loaded).items()):
                    if value is original:setattr(loaded,key,replacement)
        setattr(module,name,replacement)
