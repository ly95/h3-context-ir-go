"""Original plan construction calls use native deterministic Plan.Build.

Ambiguous replacement decisions come only from actual archived model outputs.
Source inspection can retain unresolved names; the production compiler refuses
publishing unresolved bindings. No source intent matcher is copied here.
"""
import dataclasses
import copy
import functools
import importlib
import inspect
import json
from pathlib import Path
import sys

ROOT=Path(__file__).resolve().parents[1]
ACTUAL_REPLACEMENTS={}

def actual_replacement(arguments,plain):
    key=json.dumps({k:plain(arguments[k])for k in ('brief','mode','cards')},ensure_ascii=False,sort_keys=True,separators=(',',':'))
    return ACTUAL_REPLACEMENTS.get(key)

def install(native_operation,plain):
    global ACTUAL_REPLACEMENTS
    inputs=json.loads((ROOT/'conformance/fixtures/semantic_cases.json').read_text())
    actual=json.loads((ROOT/'conformance/fixtures/semantic_resolutions.json').read_text())
    resolutions={x['case_id']:x for x in actual['cases']}
    canonical=lambda x:json.dumps(x,ensure_ascii=False,sort_keys=True,separators=(',',':'))
    targets={}
    for item in inputs['cases']:
        if item['input']['kind']!='replacement':continue
        resolution=resolutions[item['case_id']]
        if resolution['input_sha256']!=item['input_sha256']:raise RuntimeError('Actual replacement input hash differs')
        if resolution['status']=='proposed':targets[canonical({k:item['original_arguments'][k] for k in ('brief','mode','cards')})]=(resolution,item['case_id'])
    ACTUAL_REPLACEMENTS=targets
    from h3ir.grid import Target
    from h3ir.models import Plan,Mode,ManifestEntry,AssetKind,Role,SubjectPlan,SpeakerPlan,ShotPlan,CameraMove,DialogueLine,LoraChoice,ModeDecision
    def construct(cls,raw):return cls(**{k:v for k,v in raw.items() if k in {f.name for f in dataclasses.fields(cls)}})
    module=importlib.import_module('h3ir.plan');original=module.build_plan;signature=inspect.signature(original)
    @functools.wraps(original)
    def build(*args,**kwargs):
        bound=signature.bind(*args,**kwargs);bound.apply_defaults();arguments=dict(bound.arguments)
        key=canonical({k:plain(arguments[k]) for k in ('brief','mode','cards')})
        if key in targets:
            resolution,case_id=targets[key];arguments['replacement_targets']=resolution['replacement_targets']
            arguments['actual_replacement_case_id']=case_id;arguments['actual_replacement_report']=resolution['run_report']
        raw=copy.deepcopy(native_operation('h3ir.plan.build_plan',arguments))
        raw['mode']=Mode(raw['mode']);raw['target']=construct(Target,{**raw['target'],'canvas':tuple(raw['target']['canvas'])})
        raw['manifest']=[construct(ManifestEntry,{**x,'kind':AssetKind(x['kind']),'role':Role(x['role']),'px':tuple(x['px']) if x.get('px') is not None else None}) for x in raw['manifest']]
        raw['subjects']=[construct(SubjectPlan,x) for x in raw['subjects']]
        raw['speakers']=[construct(SpeakerPlan,x) for x in raw['speakers']]
        raw['shots']=[construct(ShotPlan,{**x,'camera':construct(CameraMove,x['camera']) if x['camera'] else None,'dialogue':[construct(DialogueLine,d) for d in x['dialogue']]}) for x in raw['shots']]
        raw['loras']=[construct(LoraChoice,x) for x in raw['loras']]
        if raw['mode_decision'] is not None:raw['mode_decision']=construct(ModeDecision,{**raw['mode_decision'],'mode':Mode(raw['mode_decision']['mode'])})
        return construct(Plan,raw)
    for n,loaded in list(sys.modules.items()):
        if n.startswith('h3ir') and loaded is not None:
            for key,value in list(vars(loaded).items()):
                if value is original:setattr(loaded,key,build)
    module.build_plan=build
