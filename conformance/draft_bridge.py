"""The original full draft entry executes public Go Compiler.Draft.

Native plans retain the actual Document/Artifact; their renderer always calls
native h3.Compile. Source Python draft/writer/renderer code is never invoked for
these plans. Unsupported edits fail explicitly rather than returning stale text.
"""
import copy
import dataclasses
import functools
import importlib
import inspect
import sys


def install(native_operation, plain):
    models=importlib.import_module('h3ir.models')
    from h3ir.grid import Target
    from h3ir.render import RenderResult
    draft=importlib.import_module('h3ir.draft')
    render=importlib.import_module('h3ir.render')
    class NativeDraftPlan(models.Plan):
        pass
    def construct(cls,raw):
        return cls(**{k:v for k,v in raw.items() if k in {f.name for f in dataclasses.fields(cls)}})
    def decode(raw):
        raw=copy.deepcopy(raw)
        raw['mode']=models.Mode(raw['mode'])
        raw['target']=construct(Target,{**raw['target'],'canvas':tuple(raw['target']['canvas'])})
        raw['manifest']=[construct(models.ManifestEntry,{**x,'kind':models.AssetKind(x['kind']),'role':models.Role(x['role']),'px':tuple(x['px']) if x.get('px') is not None else None})for x in raw['manifest']]
        raw['subjects']=[construct(models.SubjectPlan,x)for x in raw['subjects']]
        raw['speakers']=[construct(models.SpeakerPlan,x)for x in raw['speakers']]
        raw['shots']=[construct(models.ShotPlan,{**x,'camera':construct(models.CameraMove,x['camera'])if x['camera']else None,'dialogue':[construct(models.DialogueLine,d)for d in x['dialogue']]})for x in raw['shots']]
        raw['loras']=[construct(models.LoraChoice,x)for x in raw['loras']]
        if raw['mode_decision']is not None:raw['mode_decision']=construct(models.ModeDecision,{**raw['mode_decision'],'mode':models.Mode(raw['mode_decision']['mode'])})
        return construct(NativeDraftPlan,raw)
    original=draft.deterministic_draft;signature=inspect.signature(original)
    @functools.wraps(original)
    def native_draft(*args,**kwargs):
        bound=signature.bind(*args,**kwargs);bound.apply_defaults()
        arguments=dict(bound.arguments)
        licence=arguments['licence']
        if licence is None:
            licence=importlib.import_module('h3ir.licence').resolve_licence(arguments['brief'],arguments['cards'])
        style=importlib.import_module('h3ir.style').resolve_style(arguments['brief'],arguments['cards'],licence)
        data={**arguments,'style_phrase':style.phrase}
        import plan_bridge
        resolved=plan_bridge.actual_replacement(arguments,plain)
        if resolved:
            actual,case_id=resolved
            data.update(replacement_targets=actual['replacement_targets'],actual_replacement_case_id=case_id,actual_replacement_report=actual['run_report'])
        packet=native_operation('h3ir.draft.deterministic_draft',data)
        plan=decode(packet['plan']);plan._native_packet=packet;plan._native_baseline=plain(plan)
        return plan
    renderer=render.render_ir
    @functools.wraps(renderer)
    def native_render(plan,*args,**kwargs):
        if not isinstance(plan,NativeDraftPlan):
            return renderer(plan,*args,**kwargs)
        raw=native_operation('h3ir.render.native_draft',{'packet':plan._native_packet,'plan':plain(plan),'baseline':plan._native_baseline})
        return RenderResult(prompt=raw['prompt'],sections=raw['sections'],notes=raw['notes'],shot_bodies={int(k):v for k,v in raw['shot_bodies'].items()})
    for source,replacement in ((original,native_draft),(renderer,native_render)):
        for name,module in list(sys.modules.items()):
            if name.startswith('h3ir') and module is not None:
                for key,value in list(vars(module).items()):
                    if value is source:setattr(module,key,replacement)
    draft.deterministic_draft=native_draft
    render.render_ir=native_render
