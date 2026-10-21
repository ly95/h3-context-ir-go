"""Original source HTTP assertions run against native Go handlers and intake.

Only the test's injected compile/refine callbacks remain Python. The callback
must differ from the original model compiler; accidental model IO is refused.
"""
import asyncio
import base64
import dataclasses
import functools
import importlib
import itertools
import json


def install(native_operation):
    import callbacks
    import httpx
    from starlette.testclient import TestClient
    from starlette.requests import ClientDisconnect
    from fastapi import HTTPException
    from h3ir.models import AssetKind, AssetRef, Brief, DialogueLine, Role
    from h3ir import service
    from h3ir.config import get_config
    from h3ir import contract
    original_compile = service.compile_brief
    original_refine = service.refine
    sequence = itertools.count(1)

    def config():
        cfg = get_config()
        from h3ir import contract
        return {'root':str(cfg.paths.uploads_dir()),'max_bytes':cfg.assets.upload_max_bytes,
                'store_bytes':cfg.assets.upload_store_bytes,'ttl_hours':cfg.assets.upload_ttl_hours,
                'allow_paths':cfg.assets.allow_paths,'profile':cfg.profile,'package_version':contract._package_version() or ''}

    def source_brief(raw):
        brief_keys = {f.name for f in dataclasses.fields(Brief)}
        asset_keys = {f.name for f in dataclasses.fields(AssetRef)}
        values = {k:v for k,v in raw.items() if k in brief_keys}
        values['assets'] = [AssetRef(**{**{k:v for k,v in x.items() if k in asset_keys},
            'kind':AssetKind(x['kind']),'role':Role(x['role']),
            'px':tuple(x['px']) if x.get('px') is not None else None}) for x in raw.get('assets',[])]
        values['dialogue'] = [DialogueLine(**d) for d in raw.get('dialogue',[])]
        return Brief(**values)

    def failure(error):
        from h3ir.analyse import AssetAnalysisError, ToolMissing
        from h3ir.backend import BackendUnavailable, BackendError
        from h3ir.compile import BriefRefused
        if isinstance(error,BriefRefused):status,code=422,error.code
        elif isinstance(error,BackendUnavailable):status,code=503,'llm-unavailable'
        elif isinstance(error,BackendError):status,code=502,'llm-error'
        elif isinstance(error,ToolMissing):status,code=503,'analysis-tool-missing'
        elif isinstance(error,AssetAnalysisError):status,code=422,'asset-unreadable'
        else:status,code=500,'unexpected-test-callback'
        return {'error':{'kind':type(error).__name__,'status':status,'code':code,'message':str(error)}}

    def compile_callback(raw):
        if service.compile_brief is original_compile:
            return {'error':{'kind':'unmocked-compiler','status':500,'code':'unmocked-compiler',
                'message':'This offline conformance bridge requires an original test-injected compiler'}}
        try:
            # Source passes these as keyword choices, rather than Brief fields.
            result=service.compile_brief(source_brief(raw),opts=service.ProfileOptions(name=get_config().profile),
                seed=raw.get('seed',7),transcripts=raw.get('transcripts',{}),thinking_prose=raw.get('effort')=='max')
        except Exception as error:return failure(error)
        return {'error':{'kind':'unsupported-test-result','status':500,'code':'unsupported-test-result',
            'message':'Successful source IRDocument conversion is not part of this callback bridge'}}

    def refine_callback(raw):
        if service.refine is original_refine:
            return {'error':{'kind':'unmocked-compiler','status':500,'code':'unmocked-compiler',
                'message':'This offline conformance bridge requires an original test-injected refiner'}}
        try:service.refine(None,raw['change'])
        except Exception as error:return failure(error)
        return {'error':{'kind':'unsupported-test-result','status':500,'code':'unsupported-test-result',
            'message':'Successful source IRDocument conversion is not part of this callback bridge'}}
    callbacks.register('service.compile',compile_callback)
    callbacks.register('service.refine',refine_callback)

    def records():
        seeds={}
        for key,value in service._STORE.items():
            document=value.get('doc');result={}
            if document is not None:
                from pytest_capture import plain
                manifest=getattr(getattr(document,'plan',None),'manifest',[])
                result={'artifact':{'prompt':getattr(document,'prompt',''),
                    'manifest':{'media':[{'label':m.label,'socket':m.wiring,'kind':m.kind.value,
                        'sha256':m.sha256,'source':{'asset_id':m.sha256,'track':'audio' if m.kind.value=='audio' else 'visual'}} for m in manifest]}},
                    'document':{'mode':plain(getattr(document,'mode',None))}}
            seeds[key]=result
        return seeds

    class NativeTransport(httpx.BaseTransport):
        def __init__(self):self.client=f'original-test-client-{next(sequence)}'
        def handle_request(self,request):
            value=native_operation('h3ir.service.http',{'client':self.client,'config':config(),
                'method':request.method,'url':str(request.url),
                'headers':{k:request.headers.get_list(k) for k in request.headers.keys()},
                'body':base64.b64encode(request.read()).decode('ascii'),'seeds':records(),
                'native_compiler':service.compile_brief is original_compile})
            return httpx.Response(value['status'],headers=[(k,v) for k,vs in value['headers'].items() for v in vs],
                content=base64.b64decode(value['body']),request=request)

    original_init=TestClient.__init__
    @functools.wraps(original_init)
    def init(self,app,*args,**kwargs):
        original_init(self,app,*args,**kwargs)
        if app is service.app:self._transport=NativeTransport()
    TestClient.__init__=init

    def to_brief(body):
        value=native_operation('h3ir.service._to_brief',{'client':'direct-intake','config':config(),
            'body':base64.b64encode(json.dumps(body.model_dump()).encode('utf-8')).decode('ascii')})
        if not value['ok']:raise HTTPException(value['status'],detail=value['detail'])
        return source_brief(value['brief'])
    service._to_brief=to_brief

    async def drain(request,budget=0):
        cfg=config()
        budget=budget or max(8*1024*1024,cfg['max_bytes']+(1<<20))
        iterator=request.stream().__aiter__();loop=asyncio.get_running_loop()
        def next_chunk(_):
            future=asyncio.run_coroutine_threadsafe(iterator.__anext__(),loop)
            try:return {'value':{'data':base64.b64encode(future.result()).decode('ascii')}}
            except StopAsyncIteration:return {'value':{'eof':True}}
            except ClientDisconnect:return {'value':{'disconnect':True}}
        callbacks.register('service.body_next',next_chunk)
        await asyncio.to_thread(native_operation,'h3ir.service._drain',{'budget':budget})
    service._drain=drain

    for name in ('snapshot','contract','digests','digest','as_json','as_js'):
        original=getattr(contract,name)
        def wrap(name,original):
            @functools.wraps(original)
            def invoke(*args,**kwargs):
                arguments={'package_version':contract._package_version() or ''}
                if name=='digest':arguments['value']=args[0] if args else kwargs['value']
                return native_operation('h3ir.contract.'+name,arguments)
            return invoke
        setattr(contract,name,wrap(name,original))
