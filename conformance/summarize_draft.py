#!/usr/bin/env python3
"""Separate actual complete native Draft calls from native plan-only assertions."""
import argparse
from collections import Counter, defaultdict
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--report',type=Path,required=True)
    parser.add_argument('--label',required=True)
    args=parser.parse_args()
    if Path(args.label).name!=args.label or args.label in ('','.','..'):raise SystemExit('Choose a filename-component label')
    destination=ROOT/f'conformance/reports/draft-boundaries-{args.label}.json'
    if destination.exists():raise SystemExit('Refusing to overwrite prior boundary evidence')
    report=json.loads(args.report.read_text())
    calls=defaultdict(list)
    for call in report['go_core_calls']:calls[call['nodeid']].append(call)
    rows=[]
    failures={x['nodeid']:i for i,x in enumerate(report['reports']) if x['outcome']=='failed'}
    totals=Counter()
    explicit_style_calls=0
    for item in report['collected']:
        native=calls[item['nodeid']]
        drafts=[x for x in native if x['function']=='h3ir.draft.deterministic_draft']
        explicit_style_calls+=sum('style_phrase'in x['arguments'] for x in drafts)
        renders=[x for x in native if x['function']=='h3ir.render.native_draft']
        plans=[x for x in native if x['function']=='h3ir.plan.build_plan']
        kind='full_native_draft' if drafts else 'native_plan_only' if plans else 'other_native_component' if item['native_go_touched'] else 'python_only'
        totals[kind]+=1
        row={'nodeid':item['nodeid'],'source':item['source'],'original_assert_status':item['upstream_status'],'boundary':kind,
             'full_native_draft_calls':len(drafts),'native_artifacts_returned':sum(not x.get('error') for x in drafts),
             'native_artifact_render_calls':len(renders),'native_plan_calls':len(plans)}
        errors=[]
        for call in drafts+renders:
            if call.get('error'):errors.append({'function':call['function'],'error':call['error'],'actual':call.get('failed_native_value')})
        if errors:row['native_errors']=errors
        if item['nodeid']in failures:row['original_failure_report_index']=failures[item['nodeid']]
        rows.append(row)
    output={'source_commit':report['source_commit'],'report':str(args.report.resolve().relative_to(ROOT)),
        'report_sha256':hashlib.sha256(args.report.read_bytes()).hexdigest(),'go_bridge_sha256':report['go_bridge_sha256'],
        'original_assert_counts':report['counts'],'boundaries':dict(totals),
        'full_native_draft_calls':sum(x['full_native_draft_calls']for x in rows),
        'native_artifacts_returned':sum(x['native_artifacts_returned']for x in rows),
        'native_artifact_render_calls':sum(x['native_artifact_render_calls']for x in rows),
        'explicit_style_boundary_calls':explicit_style_calls,
        'scope':'The complete original draft entry invokes public Compiler.Draft with its actual mode argument and explicit partial-card policy. Native draft render calls invoke h3.Compile on the same authoritative Document; no source Python writer generates those artifacts. Other source plans/rendering and orchestration remain explicit Python collaborators. A failed full Draft is never counted as plan-only success.',
        'adapter_policies':['AllowPartialPerception=true reflects supplied source fixture cards, without claiming whole-track observation.','The actual DTO fields and original call arguments are retained in the referenced report; later adapter fixes do not retroactively change this run.','The native style boundary is explicit only in calls whose recorded arguments contain style_phrase; other runs used native Draft card/default styling.','Unrepresentable edits, unsupported profile choices and unavailable media evidence fail explicitly instead of returning stale or substituted output.'],
        'tests':rows}
    destination.write_text(json.dumps(output,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({k:v for k,v in output.items()if k not in ('tests','adapter_policies','scope')}))


if __name__=='__main__':main()
