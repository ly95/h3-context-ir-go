"""Native offline evaluation, keeping original assertion bodies unchanged."""
import dataclasses
import functools
import sys

from capture_evaluate import document, plain


def install(native_operation):
    from h3ir.evalloop import score as S, suite as U

    def replace(owner, name, fn):
        old = getattr(owner, name)
        setattr(owner, name, fn)
        for module in list(sys.modules.values()):
            if module and getattr(module, '__name__', '').startswith('h3ir'):
                for key, value in list(vars(module).items()):
                    if value is old:
                        setattr(module, key, fn)

    def invoke(name, args):
        return native_operation('h3ir.evalloop.score.' + name, args)

    replace(S, 'content_words', lambda text: set(invoke('content_words', {'text': text})))
    replace(S, 'jaccard', lambda a, b: invoke('jaccard', {'a': sorted(a), 'b': sorted(b)}))
    replace(S, 'shared_ngram', lambda a, b, n=5: invoke('shared_ngram', {'left': a, 'right': b, 'n': n}))
    replace(S, '_shot_bodies', lambda desc: invoke('_shot_bodies', {'desc': desc}))

    def warnings(doc):
        from h3ir.models import Finding
        return [Finding(**x) for x in invoke('_countable_warnings', {'doc': document(doc)})]

    replace(S, '_countable_warnings', warnings)
    replace(S, 'score_document', lambda doc, name='': S.Score(**invoke('score_document', {'doc': document(doc), 'name': name})))
    replace(S, 'aggregate', lambda scores: S.Aggregate(**invoke('aggregate', {'scores': plain(scores)})))
    replace(S, 'compare', lambda baseline, candidate, baseline_metrics=None: tuple(invoke('compare', {'baseline': plain(baseline), 'candidate': plain(candidate), 'baseline_metrics': plain(baseline_metrics)})))
    def run_dict(self):
        result = native_operation('h3ir.evalloop.suite.Run.dict', {'self': plain(self)})
        # JSON has arrays only; the original in-memory Config contract uses
        # tuples. Restore those declared container types without recomputing any
        # score, policy, provenance value or original assertion.
        for key in ('omit', 'only'):
            result['config'][key] = tuple(result['config'][key])
        return result

    replace(U.Run, 'dict', run_dict)

    def gate(run, baseline):
        result = native_operation('h3ir.evalloop.suite.gate', {'run': plain(run), 'baseline': plain(baseline),
                                                           'baseline_metrics': plain(U.stored_baseline_metrics())})
        return tuple(result)

    replace(U, 'gate', gate)
