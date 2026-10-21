#!/usr/bin/env python3
"""Observe unchanged upstream offline-evaluation calls; never contact a model."""
import argparse
import dataclasses
import enum
import functools
import inspect
import json
import os
from pathlib import Path
import sys


def plain(value):
    if isinstance(value, enum.Enum):
        return value.value
    if dataclasses.is_dataclass(value):
        return {f.name: plain(getattr(value, f.name)) for f in dataclasses.fields(value)}
    if isinstance(value, dict):
        return {str(k): plain(v) for k, v in value.items()}
    if isinstance(value, (list, tuple)):
        return [plain(x) for x in value]
    if isinstance(value, (set, frozenset)):
        return sorted(plain(x) for x in value)
    if value is None or isinstance(value, (bool, str, float, int)):
        return value
    raise TypeError(type(value).__name__)


def document(doc):
    findings = list(getattr(doc, 'findings', getattr(doc, 'warnings', [])))
    plan = getattr(doc, 'plan', None)
    return {'mode': plain(getattr(doc, 'mode', '')), 'sections': getattr(doc, 'sections', {}),
            'prompt_tokens': getattr(doc, 'prompt_tokens', 0), 'findings': plain(findings),
            'source': getattr(doc, 'source', 'written'), 'fallback_reason': getattr(doc, 'fallback_reason', None),
            'provenance': plain(getattr(doc, 'provenance', {})),
            'plan': {'shots': [{'body': s.body, 'word_target': s.word_target} for s in plan.shots] if plan else []}}


class Capture:
    def __init__(self):
        self.current = ''
        self.calls = []

    def pytest_runtest_logstart(self, nodeid, location):
        self.current = nodeid

    def pytest_configure(self, config):
        from h3ir.evalloop import score as S, suite as U
        for name in ('content_words', 'jaccard', 'shared_ngram', '_countable_warnings', '_shot_bodies', 'score_document', 'aggregate', 'compare'):
            self.wrap(S, name, 'h3ir.evalloop.score.' + name)
        self.wrap(U, 'gate', 'h3ir.evalloop.suite.gate')
        self.wrap(U.Run, 'dict', 'h3ir.evalloop.suite.Run.dict')

    def wrap(self, owner, name, qualified):
        fn = getattr(owner, name)
        signature = inspect.signature(fn)

        @functools.wraps(fn)
        def call(*args, **kwargs):
            bound = signature.bind(*args, **kwargs)
            bound.apply_defaults()
            arguments = {k: document(v) if k == 'doc' else plain(v) for k, v in bound.arguments.items()}
            result = fn(*args, **kwargs)
            self.calls.append({'id': 'evaluate-%04d' % (len(self.calls) + 1), 'nodeid': self.current,
                               'function': qualified, 'arguments': arguments, 'result': plain(result)})
            return result

        setattr(owner, name, call)
        # suite imports score helpers before patching; keep aliases native-observable.
        for module in list(sys.modules.values()):
            if module and getattr(module, '__name__', '').startswith('h3ir'):
                for key, value in list(vars(module).items()):
                    if value is fn:
                        setattr(module, key, call)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    if args.out.exists():
        raise SystemExit('Refusing to overwrite an earlier capture')
    args.out = args.out.resolve()
    args.source = args.source.resolve()
    sys.path.insert(0, str(args.source.resolve()))
    os.chdir(args.source)
    import pytest
    capture = Capture()
    result = pytest.main([str(args.source / 'tests'), '-q', '--disable-warnings'], plugins=[capture])
    args.out.write_text(json.dumps({'source_commit': 'fd031e136ae3d89147324c0d1b2aa65e838c21f5',
                                   'scope': 'Actual unchanged original pytest calls; observations only; no supplementary fabricated cases.',
                                   'pytest_exit_code': int(result), 'calls': capture.calls}, ensure_ascii=False, indent=2) + '\n')
    print('captured', len(capture.calls), 'calls;', len({c['nodeid'] for c in capture.calls}), 'original nodeids')
    return int(result)


if __name__ == '__main__':
    raise SystemExit(main())
