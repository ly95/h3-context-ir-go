"""Pin Markdown report citations to a complete immutable audit checkout."""
import argparse
from pathlib import Path
import re
import subprocess
from urllib.parse import quote
p=argparse.ArgumentParser()
p.add_argument("--audit-commit",required=True)
a=p.parse_args()
if not re.fullmatch(r"[0-9a-f]{40}",a.audit_commit): raise SystemExit("full audit commit required")
root=Path(__file__).resolve().parents[2]
reports=(root/"conformance/reports").resolve()
files=subprocess.check_output(["git","ls-files","-z","--","*.md"],cwd=root).decode().split("\0")
changed=[]
for name in files:
 if not name or name.startswith("conformance/reports/"): continue
 path=root/name
 text=path.read_text()
 def replace(m):
  target=m.group(1)
  if "://" in target or target.startswith(("#","/","mailto:")): return m.group(0)
  local,sep,anchor=target.partition("#")
  resolved=(path.parent/local).resolve()
  try: relative=resolved.relative_to(reports)
  except ValueError: return m.group(0)
  if str(relative)=="release-index-v0.2.0.json": return m.group(0)
  if not resolved.is_file(): raise RuntimeError("missing report citation: "+target)
  url="https://github.com/ly95/contextir/blob/"+a.audit_commit+"/conformance/reports/"+quote(relative.as_posix(),safe="/")
  return "]("+url+("#"+anchor if sep else "")+")"
 updated=re.sub(r"]\(([^)\s]+)\)",replace,text)
 if updated!=text:
  path.write_text(updated); changed.append(name)
print("Pinned audit citations in "+str(len(changed))+" Markdown files")
