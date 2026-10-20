#!/usr/bin/env python3
"""Archive a complete actual ROLE_CASES run without reading held-out expectations."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil

from supplement_archive import supplement

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', type=Path, required=True)
    parser.add_argument('--label', required=True)
    args = parser.parse_args()
    if Path(args.label).name != args.label or args.label in ('', '.', '..'):
        raise SystemExit('label must be a filename component')
    inputs = json.loads((ROOT / 'conformance/fixtures/role_briefs.json').read_text())
    hashes = {x['case_id']: x['input_sha256'] for x in inputs['cases']}
    source = args.run.resolve()
    report = json.loads((source / 'report.json').read_text())
    if report['Source'] != inputs['source_commit'] or report['Kind'] != 'roles':
        raise SystemExit('Unexpected source revision or run kind')
    if len(report['results']) != len(hashes) or {x['case_id'] for x in report['results']} != set(hashes):
        raise SystemExit('Expected every original ROLE_CASES input exactly once')
    if any(x['input_sha256'] != hashes[x['case_id']] for x in report['results']):
        raise SystemExit('An actual run input hash differs from the original corpus')
    target = ROOT / f'conformance/reports/live-roles-{args.label}'
    if target.exists():
        raise SystemExit('Refusing to overwrite an archived run')
    target.mkdir(parents=True)
    shutil.copyfile(source / 'report.json', target / 'report.json')
    files = []
    for path in sorted(list(source.glob('*/call-*')) + list(source.glob('*/result.json'))):
        destination = target / path.relative_to(source)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, destination)
        files.append({'path': str(destination.relative_to(ROOT)), 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
    summary = {
        'source_commit': report['Source'], 'model': report['Model'], 'reasoning': report['Reasoning'],
        'scope': 'Actual production Compile over the 16 original ROLE_CASES Briefs and caller-supplied synthetic cards. Cards are supplied evidence, not fresh media observations. Compilation alone is not a role-semantics or prose-quality verdict.',
        'archived_report': str((target / 'report.json').relative_to(ROOT)), 'files': files,
        'cases': [{k: x.get(k) for k in ('case_id', 'input_sha256', 'status', 'error', 'issues', 'calls')} for x in report['results']],
    }
    (target / 'summary.json').write_text(json.dumps(summary, ensure_ascii=False, indent=2) + '\n')
    supplement(source, target)
    print(json.dumps({'archive': str(target.relative_to(ROOT)), 'cases': [(x['case_id'], x['status']) for x in report['results']], 'trace_files': len(files)}))


if __name__ == '__main__':
    main()
