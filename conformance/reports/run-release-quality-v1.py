#!/usr/bin/env python3
"""Record offline full race/vet/format validation on unchanged native inputs."""
import collections
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'conformance'))
from run_native_functionality import compatibility_source_hashes
from verify_go_compatibility import extra_inputs, inspect_events
from verify_private_native import sealed_environment

report=ROOT/'conformance/reports/local-quality-release-v1.json'
assert not report.exists(), 'refuse to replace previous evidence'
work=Path(tempfile.mkdtemp(prefix='contextir-release-quality-'))
env=sealed_environment(work,work/'module-cache')
os.environ.clear();os.environ.update(env)
source=compatibility_source_hashes(ROOT);fixtures=extra_inputs(ROOT)
packages=subprocess.check_output(['go','list','./...'],cwd=ROOT,text=True).splitlines()
v={'schema':'contextir-release-local-quality/v1','started_at':dt.datetime.now(dt.timezone.utc).isoformat(),
   'passed':False,'source_sha256':source,'fixture_sha256':fixtures,'sealed_environment_keys':sorted(env),
   'go_version':subprocess.check_output(['go','version'],text=True).strip(),'provider_calls':0,'checks':[]}

def digest(path): return hashlib.sha256(path.read_bytes()).hexdigest()
def run(name,cmd):
    out=report.with_name('local-quality-release-v1.'+name+'.stdout.txt')
    err=report.with_name('local-quality-release-v1.'+name+'.stderr.txt')
    assert not out.exists() and not err.exists()
    with out.open('xb') as o,err.open('xb') as e:
        r=subprocess.run(cmd,cwd=ROOT,stdout=o,stderr=e,timeout=600)
    c={'name':name,'command':cmd,'exit_code':r.returncode,'stdout':str(out.relative_to(ROOT)),
       'stdout_sha256':digest(out),'stderr':str(err.relative_to(ROOT)),'stderr_sha256':digest(err),
       'passed':r.returncode==0}
    v['checks'].append(c)
    return c,out

try:
    c,events=run('race',['go','test','-race','-json','-count=1','-timeout=300s','./...'])
    if c['passed']:
        details=inspect_events(events,packages,'github.com/ly95/contextir')
        tests=details.pop('test_events')
        c['counts']={'root_tests':sum('/' not in x['test'] for x in tests),
                     'subtests':sum('/' in x['test'] for x in tests),'failed':0,'skipped':0,
                     'packages':details['package_count']}
    c,_=run('vet',['go','vet','./...'])
    paths=[p for p in source if p.endswith('.go')]
    c,out=run('gofmt',['gofmt','-l',*paths]);c['passed']=c['passed'] and not out.read_bytes()
    run('diff',['git','diff','--check'])
    v['source_files_unchanged_during_run']=source==compatibility_source_hashes(ROOT) and fixtures==extra_inputs(ROOT)
    v['passed']=v['source_files_unchanged_during_run'] and all(x['passed'] for x in v['checks'])
except Exception as e:
    v['error']=str(e)
finally:
    v['finished_at']=dt.datetime.now(dt.timezone.utc).isoformat()
    report.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({'report':str(report.relative_to(ROOT)),'passed':v['passed'],
                  'checks':[{k:x.get(k) for k in ('name','passed','counts')} for x in v['checks']]}))
raise SystemExit(0 if v['passed'] else 1)
