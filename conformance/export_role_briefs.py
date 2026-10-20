#!/usr/bin/env python3
"""Export one exact original ROLE_CASES input per role, never an oracle output."""
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]

def main():
    source=ROOT/'conformance/fixtures/compiler_calls.json';corpus=json.loads(source.read_text())
    prefix='tests/test_every_role_renders.py::test_every_role_renders_a_draft_that_passes_its_own_validator['
    seen=set();cases=[]
    for call in corpus['calls']:
        if call['function']!='h3ir.plan.build_plan' or not call['nodeid'].startswith(prefix) or call['nodeid'] in seen:continue
        seen.add(call['nodeid']);value={k:call['arguments'][k] for k in ('brief','cards')}
        digest=hashlib.sha256(json.dumps(value,ensure_ascii=False,sort_keys=True,separators=(',',':')).encode()).hexdigest()
        cases.append({'case_id':call['id'],'nodeid':call['nodeid'],'input_sha256':digest,'input':value})
    if len(cases)!=16:raise SystemExit('Expected exactly all 16 original ROLE_CASES')
    output={'kind':'roles','source_commit':corpus['source_commit'],'source_fixture_sha256':hashlib.sha256(source.read_bytes()).hexdigest(),
        'warning':'Exact original Brief and caller-supplied synthetic cards only. No result/expected is included. These are not actual media observations.', 'cases':cases}
    path=ROOT/'conformance/fixtures/role_briefs.json';data=json.dumps(output,ensure_ascii=False,indent=2)+'\n'
    if path.exists() and path.read_text()!=data:raise SystemExit('Refusing to change existing role input corpus')
    path.write_text(data);print(f'Exported {len(cases)} original role inputs')

if __name__=='__main__':main()
