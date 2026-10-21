from pathlib import Path
import json,hashlib,shutil,tempfile,tarfile
ROOT=Path.cwd();RUN=ROOT/'conformance/reports/release-live-round6';RECEIPT=ROOT/'conformance/reports/builds/release-live-round8-live-build.json';TESTDATA=ROOT/'compiler/testdata'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def load(p):return json.loads(Path(p).read_text())
def write(p,v):p.parent.mkdir(parents=True,exist_ok=True);p.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n')
r=load(RUN/'report.json');receipt=load(RECEIPT)
assert r.get('finished_at') and r['passed'] and len(r['cases'])==35
assert sum(len(c['operations']) for c in r['cases'])==40 and all(o['passed'] for c in r['cases'] for o in c['operations'])
assert r['binary_sha256']==receipt['binary_sha256']=='2b34869092e0110606f3a071c18af4f6183e565e4237b8612d28c455d91b7bfa'
assert sha(ROOT/receipt['source_archive'])==receipt['source_archive_sha256'] and sha(receipt['binary'])==receipt['binary_sha256']
roles=load(ROOT/'conformance/reports/release-live-round6-semantic-role-review.json');assert roles['passed'] and roles['summary']['passed']==16 and roles['run_report_sha256']==sha(RUN/'report.json')
verification=load(ROOT/'conformance/reports/release-live-round6-original-input-verification.json');assert verification['passed'] and verification['live_input_sha256']==sha(RUN/'input.json')
source_input=ROOT/'conformance/fixtures/remaining_highlevel_inputs_v1.json';originals={c['case_id']:c for c in load(source_input)['cases']};cases={c['case_id']:c for c in r['cases']};raw_inputs={c['case_id']:c for c in load(RUN/'input.json')['cases']}
wardrobe=next(c for c in load(TESTDATA/'remaining_wardrobe/manifest.json')['cases'] if c['case_id']=='licence-01108')
assert raw_inputs['original-wardrobe-request-authority']['original']==wardrobe['input']
assert wardrobe['input_sha256']=='966730e96f5e1e88e954e6702ddf1134ff932babe29ab23112ea5e025f853add'
assert sha(TESTDATA/'remaining_highlevel/manifest.json')=='6eedad3ba25328b06c9b21b7c1ed838f46fa933b9f6bf8b3c2fa641e8e800dc9'
archive=ROOT/'conformance/reports/remaining-native-before-release-live-round6.tar.gz';assert not archive.exists()
with tarfile.open(archive,'w:gz') as t:
 for rel in ['compiler/testdata/remaining_highlevel','compiler/remaining_highlevel_replay_test.go','compiler/remaining_wardrobe_contract_test.go']:
  t.add(ROOT/rel,arcname=rel)
stage=Path(tempfile.mkdtemp(prefix='.release-live-round6-import-',dir=TESTDATA))
outputs={}
for directory,ids in [('remaining_highlevel',list(originals)),('remaining_wardrobe_fresh',['original-wardrobe-request-authority'])]:
 dest=stage/directory;dest.mkdir()
 manifest={'schema_version':2,'source_commit':r['source_commit'],'executable_sha256':r['binary_sha256'],'source_archive_sha256':receipt['source_archive_sha256'],'product_git_head':receipt['git_head'],'source_input_fixture':str(source_input.relative_to(ROOT)) if directory=='remaining_highlevel' else 'compiler/testdata/remaining_wardrobe/manifest.json#licence-01108.input','source_report':'conformance/reports/release-live-round6/report.json','source_report_sha256':sha(RUN/'report.json'),'source_build_receipt':str(RECEIPT.relative_to(ROOT)),'source_build_receipt_sha256':sha(RECEIPT),'source_live_input_sha256':sha(RUN/'input.json'),'source_assertions_sha256':sha(RUN/'assertions.json'),'model':r['model'],'reasoning':r['reasoning_effort'],'max_output_tokens':r['max_output_tokens'],'scope':'Complete actual public Compile operations from one frozen main batch. Original inputs are converted independently through FromOriginal; every actual request must match stage/system/input/schema before its full response is replayed. All failed candidates, repairs, usage and final normalized Result are preserved. Expected assertions never enter model input.','cases':[]}
 context={'schema':'contextir-release-highlevel-recording/v2','executable_sha256':r['binary_sha256'],'source_archive_sha256':receipt['source_archive_sha256'],'cases':[]}
 for name in ids:
  case=cases[name];op=case['operations'][0];assert op['status']=='compiled' and op['passed'] and op['roundtrip'] and op['usage_complete']
  assert case['options']['max_structure_retries']==(1 if directory=='remaining_wardrobe_fresh' else 3)
  assert case['options']['planner_max_retries']==(1 if directory=='remaining_wardrobe_fresh' else 3)
  source_dir=RUN/name/'op-00-compile';assert source_dir.is_dir()
  shutil.copytree(source_dir,dest/name/'op-00-compile')
  result=load(source_dir/'result.json');assert result['status']=='compiled'
  write(dest/name/'result.json',{'result':result})
  original=originals[name] if directory=='remaining_highlevel' else {'input_sha256':wardrobe['input_sha256'],'input':wardrobe['input'],'nodeid':'tests/test_reference_licence.py::test_the_wardrobe_guard_defers_to_who_governs_wardrobe'}
  entry={'case_id':name,'kind':'compile','input_sha256':original['input_sha256'],'input':original['input'],'nodeid':original['nodeid'],'calls':[],'result':name+'/result.json'}
  for call in op['calls']:
   req=load(RUN/call['request_file']);response=load(RUN/call['response_file']);assert req['stage']==call['stage'];assert response['response']['usage']==call['usage']
   entry['calls'].append({'index':call['index'],'stage':call['stage'],'request':call['request_file'],'output':call.get('output_file',''),'response':call['response_file'],'images':call.get('images_file',''),'model':call['model'],'usage':call['usage'],'usage_valid':call['usage_valid']})
  manifest['cases'].append(entry)
  context['cases'].append({'case_id':name,'brief':case['brief'],'options':case['options'],'responses':[c['response_file'] for c in op['calls']]})
 if directory=='remaining_highlevel':shutil.copyfile(source_input,dest/'inputs.json')
 write(dest/'recording_context.json',context)
 for source,name in [(RECEIPT,'source_build_receipt.json'),(RUN/'report.json','source_live_report.json'),(RUN/'input.json','source_live_input.json'),(RUN/'assertions.json','source_live_assertions.json')]:shutil.copyfile(source,dest/name)
 manifest['files']={str(p.relative_to(dest)):sha(p) for p in sorted(dest.rglob('*')) if p.is_file()}
 write(dest/'manifest.json',manifest);outputs[directory]=sha(dest/'manifest.json')
# Recheck the entire frozen batch immediately before replacing native fixtures.
assert sha(RUN/'report.json')==manifest['source_report_sha256']
old=stage/'old-highlevel';(TESTDATA/'remaining_highlevel').rename(old)
(stage/'remaining_highlevel').rename(TESTDATA/'remaining_highlevel')
assert not (TESTDATA/'remaining_wardrobe_fresh').exists()
(stage/'remaining_wardrobe_fresh').rename(TESTDATA/'remaining_wardrobe_fresh')
shutil.rmtree(stage)
p=ROOT/'compiler/remaining_highlevel_replay_test.go';text=p.read_text();assert text.count('6eedad3ba25328b06c9b21b7c1ed838f46fa933b9f6bf8b3c2fa641e8e800dc9')==1;text=text.replace('6eedad3ba25328b06c9b21b7c1ed838f46fa933b9f6bf8b3c2fa641e8e800dc9',outputs['remaining_highlevel']);text=text.replace('tape.Directory = "remaining_highlevel"','tape.Directory = "remaining_highlevel"\n\tremainingCompleteRecordingInventory(t, tape)',1);p.write_text(text)
p=ROOT/'compiler/remaining_wardrobe_fresh_replay_test.go';text=p.read_text();assert text.count('pending-actual-finished-release-live-run')==1;p.write_text(text.replace('pending-actual-finished-release-live-run',outputs['remaining_wardrobe_fresh']))
p=ROOT/'compiler/remaining_wardrobe_contract_test.go';text=p.read_text();needle='\tt.Helper()\n\tb, l, _ := remainingRecordingReplay(t, "remaining_wardrobe", "licence", id)';assert text.count(needle)==1;p.write_text(text.replace(needle,'\tt.Helper()\n\tif id == "licence-01108" {\n\t\treturn remainingWardrobeFreshCompile(t)\n\t}\n\tb, l, _ := remainingRecordingReplay(t, "remaining_wardrobe", "licence", id)',1))
p=ROOT/'conformance/adaptations/remaining_wardrobe.json';a=load(p)
for c in a['cases']:
 if c['go_test']!='TestRemainingWardrobe/GuardOwnership':continue
 c['adaptation']='The original unchanged reference/request wardrobe-authority assertions and contrary typed-decision guard remain. The keep case replays its original DecideLicence tape. The change case uses the exact original licence-01108 FromOriginal input in the complete fresh public Compiler.Compile recording, then supplies its actual Licence to the unchanged native GuardOwnership assertion. This is an explicit Go API adaptation from isolated DecideLicence to full Compile, not a relabelled subset recording. Every real request stage/system/input/schema, full response, retry, usage and complete normalized Result is checked; no expected assertion is included in model input.'
 c['additional_recording_manifests']=['compiler/testdata/remaining_wardrobe_fresh/manifest.json']
 c['fresh_complete_compile']={'original_case_id':'licence-01108','case_id':'original-wardrobe-request-authority','profile':'h3-comfyui-17k5-v1','max_structure_retries':1,'planner_max_retries':1,'recording_manifest_sha256':outputs['remaining_wardrobe_fresh'],'source_report_sha256':sha(RUN/'report.json'),'executable_sha256':r['binary_sha256'],'source_archive_sha256':receipt['source_archive_sha256']}
write(p,a)
print(json.dumps({'manifest_sha256':outputs,'history_archive':str(archive.relative_to(ROOT)),'history_archive_sha256':sha(archive),'report_sha256':sha(RUN/'report.json')},indent=2))
