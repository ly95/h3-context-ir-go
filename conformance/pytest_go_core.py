"""Original asserts with recorded native Go component calls; collaborators stay explicit."""
import functools
import importlib
import inspect
import json
import os
from pathlib import Path
import sys
import pytest_capture as cap
import pytest_go_validate as validator

pytest_collection_modifyitems=cap.pytest_collection_modifyitems
pytest_runtest_logstart=cap.pytest_runtest_logstart
pytest_runtest_logfinish=cap.pytest_runtest_logfinish
pytest_runtest_logreport=cap.pytest_runtest_logreport
CORE_CALLS=[]
class GoOperationError(ValueError):
    def __init__(self,message,code):
        super().__init__(message)
        self.code=code


def native_operation(qualified,arguments):
    request={'operation':qualified,'arguments':cap.plain(arguments)}
    observed_callbacks=[]
    p=validator.PROCESS
    p.stdin.write(json.dumps(request,ensure_ascii=False)+'\n');p.stdin.flush()
    while True:
        line=p.stdout.readline()
        if not line:raise RuntimeError('Go core bridge stopped before responding')
        result=json.loads(line)
        if 'callback' not in result:break
        import callbacks
        reply=callbacks.dispatch(result['callback'],result.get('payload'))
        observed_callbacks.append({'name':result['callback'],'payload':result.get('payload'),'reply':reply})
        p.stdin.write(json.dumps(reply,ensure_ascii=False)+'\n');p.stdin.flush()
    def redacted(value):
        if isinstance(value,dict):return {k:('[redacted]' if k in ('api_key','llm_key','authorization','Authorization','access_token','refresh_token','password','bearer_token') else redacted(v)) for k,v in value.items()}
        if isinstance(value,list):return [redacted(v) for v in value]
        return value
    record={'nodeid':cap.CURRENT,'function':qualified,'arguments':redacted(request['arguments'])}
    if observed_callbacks:record['injected_test_callbacks']=redacted(observed_callbacks)
    CORE_CALLS.append(record)
    if result.get('error'):
        record.update(raised='ValueError',error=result['error'])
        if result.get('value') is not None:record['failed_native_value']=redacted(result['value'])
        raise GoOperationError(result['error'],result.get('error_code'))
    record['result']=result.get('value')
    return record['result']

def decode_result(name,value):
    from h3ir.grid import Target
    from h3ir.models import AssetKind,Role,ManifestEntry,SubjectPlan,SpeakerPlan,ShotPlan,CameraMove,DialogueLine
    if name=='h3ir.grid.Target.build':return Target(**{**{k:v for k,v in value.items() if k!='alignment_policy'},'canvas':tuple(value['canvas'])})
    if name in ('h3ir.grid.adapt_canvas','h3ir.grid.canvas_for_aspect'):return tuple(value)
    if name=='h3ir.plan.build_manifest':return [ManifestEntry(**{**x,'kind':AssetKind(x['kind']),'role':Role(x['role']),'px':tuple(x['px']) if x['px'] is not None else None}) for x in value]
    if name=='h3ir.plan.build_subjects':return [SubjectPlan(**x) for x in value]
    if name=='h3ir.plan.build_speakers':return [SpeakerPlan(**x) for x in value]
    if name=='h3ir.plan.allocate_shots':return [ShotPlan(**{**x,'camera':CameraMove(**x['camera']) if x['camera'] else None,'dialogue':[DialogueLine(**v) for v in x['dialogue']]}) for x in value]
    return value

def replace_function(module,name,qualified):
    original=getattr(module,name);signature=inspect.signature(original)
    @functools.wraps(original)
    def wrapper(*args,**kwargs):
        bound=signature.bind(*args,**kwargs);bound.apply_defaults()
        return decode_result(qualified,native_operation(qualified,dict(bound.arguments)))
    # Replace object-identical imported aliases, never guesses based on names.
    for n,loaded in list(sys.modules.items()):
        if n.startswith('h3ir') and loaded is not None:
            for key,value in list(vars(loaded).items()):
                if value is original:setattr(loaded,key,wrapper)
    setattr(module,name,wrapper)

def configure(config,*,mode_replay=True):
    validator.pytest_configure(config)
    grid=importlib.import_module('h3ir.grid')
    for name in ('align_frame_count','video_latent_t','audio_latent_t','legal_frames','frames_for_seconds','rows_per_latent_frame','adapt_canvas','canvas_for_aspect','s_ss_text','instruction_line_for','ms_to_timestamp','timestamp_to_ms'):replace_function(grid,name,'h3ir.grid.'+name)
    original=grid.Target.__dict__['build'].__func__;signature=inspect.signature(original)
    @functools.wraps(original)
    def build(cls,*args,**kwargs):
        bound=signature.bind(cls,*args,**kwargs);bound.apply_defaults()
        return decode_result('h3ir.grid.Target.build',native_operation('h3ir.grid.Target.build',dict(bound.arguments)))
    grid.Target.build=classmethod(build)
    replace_function(importlib.import_module('h3ir.tokens'),'count','h3ir.tokens.count')
    plan=importlib.import_module('h3ir.plan')
    for name in ('build_manifest','build_subjects','build_speakers','allocate_shots','derive_task_types'):replace_function(plan,name,'h3ir.plan.'+name)

    import lora_bridge
    lora_bridge.install(native_operation,cap.plain)
    import media_bridge
    media_bridge.install(native_operation)
    import card_cache_bridge
    card_cache_bridge.install(native_operation,cap.plain)
    import uploads_bridge
    uploads_bridge.install(native_operation)
    import backend_bridge
    backend_bridge.install(native_operation,cap.plain)
    import service_bridge
    service_bridge.install(native_operation)
    import compile_intake_bridge
    compile_intake_bridge.install(native_operation,cap.plain)
    import director_bridge
    director_bridge.install(native_operation)
    import scope_contract_bridge
    scope_contract_bridge.install(native_operation)
    import render_format_bridge
    render_format_bridge.install(native_operation,cap.plain)
    import evaluate_bridge
    evaluate_bridge.install(native_operation)
    import plan_bridge
    plan_bridge.install(native_operation,cap.plain)
    if mode_replay:
        import semantic_bridge
        semantic_bridge.install(native_operation,cap.plain)

def pytest_configure(config):
    configure(config)

def pytest_sessionfinish(session,exitstatus):
    validator.pytest_sessionfinish(session,exitstatus)
    path=Path(os.environ['CONTEXTIR_CAPTURE_PATH']);report=json.loads(path.read_text(encoding='utf-8'))
    touched={c['nodeid'] for c in CORE_CALLS}|{c['nodeid'] for c in report['validator_calls']}
    counts={};functions={}
    for c in CORE_CALLS:functions[c['function']]=functions.get(c['function'],0)+1
    bynode={}
    for c in CORE_CALLS:bynode.setdefault(c['nodeid'],set()).add(c['function'].rsplit('.',1)[0])
    for item in report['collected']:
        modules=sorted(bynode.get(item['nodeid'],set()))
        if item['go_validator_touched']:modules.append('h3ir.validate')
        item['native_go_modules']=modules;item['native_go_touched']=item['nodeid'] in touched
        if item['native_go_touched']:counts[item['upstream_status']]=counts.get(item['upstream_status'],0)+1
    report.update(implementation_under_test='Go validator/grid/tokens/full plan construction, LoRA, Comfy graph, local media, uploads, chat-compatible backend, director/Scope helpers, three pure h3 render formatters and optional HTTP intake/handlers. Semantic mode/scope calls are native only when explicitly recorded, using actual paid model output tapes. The original tests, rendering/orchestration and injected doubles otherwise remain Python.',adapter_differences=['HTTP seed omission stays unspecified (nil) in native intake; the source test compiler callback receives the legacy keyword default 7. Explicit wire seed values are preserved. Successful source IRDocument compiler callbacks are unsupported; the product Go lifecycle has separate native tests.','Scope enum parsing and prose interpretation remain Python unless a recorded native semantic build replaces them. Native Scope formatter/algebra methods consume explicit structured values. The whole source renderer remains Python; only explicitly recorded pure formatters execute Go.'],go_core_calls=CORE_CALLS,go_core_functions=functions,native_go_assertion_counts=counts,native_go_not_covered_count=len(report['collected'])-sum(counts.values()))
    path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
