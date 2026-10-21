#!/usr/bin/env python3
"""Review classifications for untouched original assertions; no coverage upgrade."""
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]

def classify(node):
    file,name=node.split('::',1)
    if file.endswith('test_supported_python_versions.py'):
        return ('not_applicable_python_syntax','Python source grammar compatibility, not a Go package requirement.',[])
    if file.endswith('test_golden_asset_geometry.py')or'published_fl2va_example' in name:
        return ('independent_fixture_oracle','Original fixture byte/geometry integrity check remains Python. Native header inspection has separate actual-file tests.',['media/header.go','media/header_cache_test.go'])
    if file.endswith('test_director.py'):
        return ('native_api_exists_bridge_gap','The native director registry/dial/vocabulary APIs exist; these original static data assertions still inspect source Python constants. No pass is inferred.',['compiler/director.go','compiler/director_contract.go','compiler/scope_contract.go','compiler/shots.go'])
    if file.endswith('test_contract.py'):
        return ('native_api_exists_bridge_gap','Go service publishes these contracts/refusal catalogues and has native digest checks. These original model-introspection/static-table assertions remain unbridged.',['service/contract.go','service/prepare.go','service/service_test.go'])
    if file.endswith('test_shot_planning.py'):
        if 'signature' in name or 'metric' in name:
            return ('semantic_or_eval_utility_gap','Original depiction-signature lexical classifier/distinctness metric is not ported. Native typed planning exists; equivalent original metric behavior is unverified.',['compiler/shots.go'])
        return ('native_api_exists_bridge_gap','Native ValidateShotProposal/ShotSpans exist, but these exact original assertions have not been routed through them. Repair behavior still needs the original assert replay.',['compiler/shots.go','compiler/shot_schema.go'])
    if file.endswith('test_pinned_shots.py'):
        return ('native_api_exists_bridge_gap','Native shot schema and typed repair planning exist; source standalone schema/fix-prompt checks remain unbridged.',['compiler/shot_schema.go','compiler/structure.go','planner/planner.go'])
    if file.endswith('test_creativity.py'):
        if 'unknown_word' in name:
            return ('known_policy_difference','Source parse silently defaults an unknown setting; native public construction rejects unknown explicit settings. No default-parity claim.',['compiler/scope.go','compiler/scope_contract.go'])
        if 'detectors' in name:
            return ('legacy_heuristic_api_not_ported','Original independent regex detectors are intentionally replaced by typed semantic ScopeInput; direct legacy detector assertions have no bridge.',['compiler/scope.go'])
        return ('eval_harness_or_static_guard_gap','Original evaluation metrics, success gates/provenance or static dial tables are not covered by a native original-assert bridge. Native compile/validation does not prove the eval harness equivalent.',['compiler/scope.go','compiler/scope_contract.go','cmd/contextir-upstream-eval/main.go'])
    if file.endswith('test_reference_licence.py'):
        if 'classification' in name or 'tidied' in name or 'style_terms' in name:
            return ('standalone_legacy_utility_gap','Standalone source style cleanup/order/taxonomy utility API is unbridged. Native typed medium/licence decisions exist, but are not counted as this helper test passing.',['compiler/medium.go','compiler/licence.go'])
        if 'inflected' in name or 'bare_adjective' in name:
            return ('legacy_heuristic_api_not_ported','Original free-text transformation regex helper is replaced by typed model medium decisions. These exact standalone helper calls still need semantic input/tape adaptation.',['compiler/medium.go'])
        return ('pipeline_contract_needs_native_behavior_verification','The source prompt/spec/reference precedence guard is not connected to a native original-assert equivalent. Related native code exists; that alone does not establish parity.',['compiler/licence.go','compiler/structure.go','planner/planner.go'])
    if file.endswith('test_deterministic.py'):
        return ('standalone_render_or_static_guard_gap','Original sound partition/retention grammar/text normalization/render casing or rule-ID static guard is unbridged; the authoritative native renderer is not claimed byte-equivalent from helper tests.',['h3/compile.go','compiler/draft.go'])
    if file.endswith('test_video_reference.py'):
        return ('native_api_exists_bridge_gap','Native media rejects missing resources and versions image/video observation contracts. These source analyzer/version assertions still execute only Python.',['media/perceiver.go','deepseek/perception.go'])
    if file.endswith('test_endpoint_portability.py'):
        return ('native_api_exists_bridge_gap','Native backend has typed failures, but this source Python exception-inheritance assertion has not been adapted to the Go error contract.',['chatcompat/types.go'])
    if file.endswith('test_every_role_renders.py'):
        return ('independent_fixture_oracle','Source verifies its role-case table covers its own enum. Native role contract is exercised elsewhere; this source fixture guard is retained independently.',['conformance/fixtures/role_briefs.json'])
    return ('pipeline_contract_needs_native_behavior_verification','Source writer/orchestration/role-fact helper, static specification assertion or input-boundary guard remains unbridged. Related native pipeline behavior needs independent original-assert validation; it is not labelled merely a bridge gap.',['compiler/structure.go','planner/planner.go','compiler/draft.go','h3/compile.go'])

def main():
    path=ROOT/'conformance/reports/coverage_union.json';data=json.loads(path.read_text());rows=[]
    for node in data['still_python_only_exact_list']:
        kind,detail,evidence=classify(node)
        rows.append({'nodeid':node,'gap_class':kind,'review_note':detail,'native_implementation_locations':evidence,'original_native_assert_status':'not_covered'})
    from collections import Counter
    output={'source_commit':data['source_commit'],'scope':'Manual evidence classifications of untouched tests, not pass claims. A native API location proves presence only; unspecified behavior remains unverified. Only the two Python syntax checks are not applicable.',
        'counts':dict(Counter(x['gap_class']for x in rows)),'tests':rows}
    (ROOT/'conformance/reports/coverage_gaps.json').write_text(json.dumps(output,ensure_ascii=False,indent=2)+'\n');print(json.dumps(output['counts']))

if __name__=='__main__':main()
