#!/usr/bin/env python3
"""Additional source-function differential probes; NOT original pytest assertions."""
import argparse
from collections import Counter
import dataclasses
import json
from pathlib import Path
import sys
from types import SimpleNamespace

from capture_evaluate import document, plain


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--source', type=Path, required=True)
    p.add_argument('--renders', type=Path, required=True)
    p.add_argument('--out', type=Path, required=True)
    args = p.parse_args()
    if args.out.exists():
        raise SystemExit('Refusing to overwrite a prior probe')
    sys.path.insert(0, str(args.source))
    from h3ir.evalloop import score as S, suite as U
    from h3ir.models import Finding, Mode
    from h3ir.tokens import count
    calls = []

    def observe(name, arguments, result, source_nodeid=''):
        calls.append({'id': 'evaluate-probe-%04d' % (len(calls) + 1),
                      'origin': 'supplementary_original_function_on_captured_render',
                      'source_nodeid': source_nodeid, 'function': name,
                      'arguments': plain(arguments), 'result': plain(result)})

    class Doc(SimpleNamespace):
        @property
        def errors(self): return [f for f in self.findings if f.severity == 'ERROR']
        @property
        def warnings(self): return [f for f in self.findings if f.severity == 'WARN']
        @property
        def ok(self): return not self.errors
        @property
        def fell_back(self): return self.source == 'draft' and bool(self.fallback_reason)

    all_scores = []
    for call in json.loads(args.renders.read_text())['calls']:
        if call['function'] != 'h3ir.render.render_ir':
            continue
        plan = call['arguments']['plan']
        shots = [SimpleNamespace(body=x.get('body', ''), word_target=x['word_target']) for x in plan['shots']]
        doc = Doc(mode=Mode(plan['mode']), sections=call['result']['sections'],
                  plan=SimpleNamespace(shots=shots, total_word_target=lambda: sum(x.word_target for x in shots)),
                  findings=[Finding('X5-render-note', 'WARN', n) for n in call['result']['notes']],
                  prompt_tokens=count(call['result']['prompt']), provenance={}, source='draft', fallback_reason=None)
        result = S.score_document(doc, call['id'])
        all_scores.append(result)
        observe('h3ir.evalloop.score.score_document', {'doc': document(doc), 'name': call['id']}, result, call['nodeid'])
    # Aggregates reuse every captured source render. The empty input exercises
    # the original default result; both remain labelled supplemental probes.
    groups = [[], all_scores] + [[s for s in all_scores if s.mode == mode] for mode in sorted({s.mode for s in all_scores})]
    aggregates = []
    for scores in groups:
        result = S.aggregate(scores)
        aggregates.append(result)
        observe('h3ir.evalloop.score.aggregate', {'scores': scores}, result)
    for base in aggregates:
        for candidate in aggregates:
            for metrics in (None, {'errors', 'clean_rate'}, set()):
                observe('h3ir.evalloop.score.compare', {'baseline': base, 'candidate': candidate, 'baseline_metrics': metrics}, S.compare(base, candidate, metrics))
    # Gate consumes original function outputs and explicit compile errors; no
    # original test is claimed for this standalone gate exercise.
    U.stored_baseline_metrics = lambda: {'errors', 'clean_rate'}
    for agg in aggregates:
        for errors in ([], ['explicit captured-run compile failure']):
            run = U.Run(config=U.RunConfig(label='supplementary captured render'), agg=agg, errors=errors)
            for baseline in (None, aggregates[0], agg):
                observe('h3ir.evalloop.suite.gate', {'run': run, 'baseline': baseline, 'baseline_metrics': U.stored_baseline_metrics()}, U.gate(run, baseline))
    args.out.write_text(json.dumps({'source_commit': 'fd031e136ae3d89147324c0d1b2aa65e838c21f5',
                                   'scope': 'Supplementary exact source-function probes using137 actual upstream render outputs. These are NOT extra original pytest assertions.',
                                   'render_fixture': str(args.renders), 'calls': calls}, ensure_ascii=False, indent=2) + '\n')
    print(len(calls), dict(Counter(c['function'] for c in calls)))


if __name__ == '__main__':
    main()
