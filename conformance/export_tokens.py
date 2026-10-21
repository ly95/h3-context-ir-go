#!/usr/bin/env python3
"""Export exact upstream BPE IDs for actual prompt corpus and named supplements."""
import argparse
import json
from pathlib import Path
import sys

ROOT=Path(__file__).resolve().parents[1]
PIN="fd031e136ae3d89147324c0d1b2aa65e838c21f5"

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source',type=Path,required=True)
    args=parser.parse_args()
    source=args.source.resolve()
    sys.path.insert(0,str(source))
    from h3ir.tokens import _encoding, ADDED_TOKENS
    enc=_encoding()
    cases=[]
    def add(id,text,origin,nodeid=None):
        ids=enc.encode(text,allowed_special=set(ADDED_TOKENS))
        cases.append({'id':id,'origin':origin,'nodeid':nodeid,'text':text,'ids':ids,'count':len(ids)})
    original=json.loads((ROOT/'conformance/fixtures/validator_calls.json').read_text())
    if original['source_commit']!=PIN:raise SystemExit('unexpected corpus revision')
    for case in original['calls']:
        add(case['id'],case['text'],'upstream_pytest_actual_validator_input',case['nodeid'])
    for path in sorted((source/'h3ir/golden').glob('*.txt')):
        add(path.name,path.read_text(encoding='utf-8'),'upstream_golden_file')
    for index,text in enumerate((
        '',"We're I've I'M she'd won't can't A'S A'ſ A’ſ",'a\n\nb\r\nc\r\t d  e   ',
        '\u00a0\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000',
        '中文标点，。！？你好，世界。こんにちは。한글 العربية e\u0301 é',
        '0 1234567890 ١٢٣ ۱۲۳ १२३ １２３ 𝟘𝟙𝟚',
        '👩🏽‍💻 🚀 🏳️‍🌈 ❤️ 🇯🇵 text🙂text',
        '<d>[Chinese] 我们明天见，好吗？</d><scenetrans><d>[English] don\'t.</d><cutoff>',
        ''.join(ADDED_TOKENS),
    )):add(f'supplementary-{index+1:02d}',text,'supplementary_unicode_boundary_not_an_upstream_test')
    path=ROOT/'conformance/fixtures/token_encodings.json'
    path.write_text(json.dumps({'source_commit':PIN,'cases':cases},ensure_ascii=False,indent=2)+'\n')
    print(f'Exported {len(cases)} exact BPE sequences ({len(original["calls"])} actual original validator inputs; supplemental cases labelled separately)')
if __name__=='__main__':main()
