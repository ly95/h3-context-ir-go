#!/usr/bin/env python3
"""Report exact recorded-request checks separately from model/output success."""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--report',type=Path,required=True)
    p.add_argument('--label',required=True)
    a=p.parse_args()
    if Path(a.label).name!=a.label or a.label in ('','.','..'):raise SystemExit('label must be one filename component')
    destination=ROOT/f'conformance/reports/semantic-replay-applicability-{a.label}.json'
    if destination.exists():raise SystemExit('Refusing to overwrite an earlier applicability report')
    data=json.loads(a.report.read_text());rows=[];counts=Counter();binaries=Counter()
    for call in data['go_core_calls']:
        if call['function']not in ('h3ir.licence.resolve_licence','h3ir.style.resolve_style'):continue
        args=call['arguments'];value=call.get('result',call.get('failed_native_value',{}));provided=len(args.get('responses',[]));consumed=value.get('recorded_outputs_consumed',0);verified=value.get('actual_recorded_requests_verified',0)
        status='rejected' if call.get('error') else 'all_consumed_requests_exactly_verified' if consumed==verified and consumed else 'unverified'
        counts[status]+=1;binaries[args.get('actual_model_binary_sha256','unrecorded')]+=1
        rows.append({'nodeid':call['nodeid'],'function':call['function'],'actual_case_id':args.get('actual_case_id'),'actual_recorded_model_report':args.get('actual_report'),'actual_model_report_sha256':args.get('actual_model_report_sha256'),'actual_model_binary_sha256':args.get('actual_model_binary_sha256'),'actual_model_responses_provided':provided,'recorded_outputs_consumed':consumed,'actual_recorded_requests_exactly_verified':verified,'unused_recorded_outputs':provided-consumed,'status':status,'error':call.get('error')})
    output={'source_commit':data['source_commit'],'report':str(a.report.resolve().relative_to(ROOT)),'report_sha256':hashlib.sha256(a.report.read_bytes()).hexdigest(),'bridge_binary_sha256':data['go_bridge_sha256'],'counts':dict(counts),'actual_model_binaries':dict(binaries),'recorded_requests_exactly_verified':sum(x['actual_recorded_requests_exactly_verified']for x in rows),'unverified_consumed_request_count':sum(x['recorded_outputs_consumed']-x['actual_recorded_requests_exactly_verified']for x in rows),'scope':'Every counted licence/style replay request is checked in native Go for exact Stage/System and JSON-value-equivalent Input/ResponseSchema before the recorded response is consumed. These are actual historical model responses, not new paid calls or expected outputs. Unused records, errors, other semantic boundaries and original assertion outcomes remain separate. No claim of equal source versions is made from applicability alone.','calls':rows}
    destination.write_text(json.dumps(output,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({k:v for k,v in output.items()if k not in ('calls','scope')}))


if __name__=='__main__':main()
