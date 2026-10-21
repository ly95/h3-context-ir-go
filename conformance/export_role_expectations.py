#!/usr/bin/env python3
"""Derive held-out role checks from the same captured original build_plan returns."""
import argparse
import copy
import dataclasses
import hashlib
import json
from pathlib import Path
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[1]
PIN='fd031e136ae3d89147324c0d1b2aa65e838c21f5'

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--source',type=Path,required=True);a=parser.parse_args();source=a.source.resolve()
    if subprocess.check_output(['git','rev-parse','HEAD'],cwd=source,text=True).strip()!=PIN or subprocess.check_output(['git','status','--porcelain','--untracked-files=no'],cwd=source,text=True):raise SystemExit('Requires clean pinned original source')
    sys.path.insert(0,str(source))
    from h3ir import models as M
    from h3ir.grid import Target
    from h3ir.plan import audio_relations
    from h3ir.render import render_retention,render_subject_definitions
    import pytest_capture as cap
    inputs=json.loads((ROOT/'conformance/fixtures/role_briefs.json').read_text())
    calls={x['id']:x for x in json.loads((ROOT/'conformance/fixtures/compiler_calls.json').read_text())['calls']}
    def construct(cls,raw):return cls(**{k:v for k,v in raw.items() if k in {f.name for f in dataclasses.fields(cls)}})
    def plan(raw):
        raw=copy.deepcopy(raw);raw['mode']=M.Mode(raw['mode']);raw['target']=construct(Target,{**raw['target'],'canvas':tuple(raw['target']['canvas'])})
        raw['manifest']=[construct(M.ManifestEntry,{**x,'kind':M.AssetKind(x['kind']),'role':M.Role(x['role']),'px':tuple(x['px'])if x['px']is not None else None}) for x in raw['manifest']]
        raw['subjects']=[construct(M.SubjectPlan,x)for x in raw['subjects']];raw['speakers']=[construct(M.SpeakerPlan,x)for x in raw['speakers']]
        raw['shots']=[construct(M.ShotPlan,{**x,'camera':construct(M.CameraMove,x['camera'])if x['camera']else None,'dialogue':[construct(M.DialogueLine,d)for d in x['dialogue']]})for x in raw['shots']]
        raw['loras']=[construct(M.LoraChoice,x)for x in raw['loras']]
        if raw['mode_decision']is not None:raw['mode_decision']=construct(M.ModeDecision,{**raw['mode_decision'],'mode':M.Mode(raw['mode_decision']['mode'])})
        return construct(M.Plan,raw)
    cases=[]
    for item in inputs['cases']:
        call=calls[item['case_id']];original=call['result'];obj=plan(original)
        cases.append({k:item[k]for k in ('case_id','nodeid','input_sha256')})
        cases[-1].update(expected_original_plan=original,expected={
            'mode':original['mode'],'target':original['target'],'task_types':original['task_types'],
            'references':original['manifest'],'subjects':original['subjects'],'speakers':original['speakers'],
            'audio_relations':cap.plain(audio_relations(obj)),
            'retention_text':render_retention(obj),'subject_definitions':render_subject_definitions(obj)},
            oracle_scope='Exact original call.Result plus explicitly supplementary original pure formatter/algebra outputs; never model input.')
    output={'source_commit':PIN,'kind':'roles','warning':'Held-out expected values. Never read by the actual model runner.',
        'native_socket_policy':'Source runtime wiring is retained verbatim. Native base-checkpoint socket corrections must be recorded explicitly; they do not waive role, copy/property, pose/layout, retention or binding requirements.',
        'cases':cases}
    path=ROOT/'conformance/fixtures/role_expectations.json';data=json.dumps(output,ensure_ascii=False,indent=2)+'\n'
    if path.exists()and path.read_text()!=data:raise SystemExit('Refusing to rewrite an existing role oracle')
    path.write_text(data);print(f'Exported {len(cases)} separate held-out role expectations')

if __name__=='__main__':main()
