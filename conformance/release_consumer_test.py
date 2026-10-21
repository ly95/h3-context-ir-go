"""Offline controls for release replay; these are not live acceptance evidence."""
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

import release_consumer
from verify_private_native import CheckFailure


GO_CONTROLS = r'''package main

import (
 "context"
 "encoding/base64"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "testing"
 contextir "github.com/ly95/contextir"
 "github.com/ly95/contextir/compiler"
 "github.com/ly95/contextir/deepseek"
 "github.com/ly95/contextir/h3"
)

func save(t *testing.T, root, name string, value any) {
 t.Helper(); data,err:=json.Marshal(value); if err!=nil {t.Fatal(err)}
 if err=os.WriteFile(filepath.Join(root,name),data,0600);err!=nil {t.Fatal(err)}
}

func setupReplay(t *testing.T, images []deepseek.Image, failed bool) (*replay,contextir.ModelRequest) {
 t.Helper(); root:=t.TempDir()
 request:=contextir.ModelRequest{Stage:"plan",System:"frozen system",Input:json.RawMessage(`{"request":"frozen"}`),ResponseSchema:json.RawMessage(`{"type":"object"}`)}
 response:=responseRecord{Response:contextir.ModelResponse{JSON:json.RawMessage(`{"accepted":true}`),Model:"recorded-model",Usage:contextir.Usage{InputTokens:3,OutputTokens:2}}}
 if failed {response.Error=&safeError{Code:contextir.CodeModelOutput,Stage:"deepseek.response",Issues:[]contextir.Issue{{Code:contextir.CodeModelOutput,Path:"output",Message:"invalid JSON"}}}}
 hashes:=map[string]bool{}
 for _,image:=range images {
  payload,_:=base64.StdEncoding.DecodeString("AAECAw==")
  hash:=digest(payload);hashes[hash]=true
  response.Images=append(response.Images,struct{SHA256 string `json:"sha256"`;FixtureSHA256 string `json:"fixture_sha256"`;Detail string `json:"detail"`}{digest([]byte(image.URI)),hash,image.Detail})
 }
 save(t,root,"request.json",request);save(t,root,"response.json",response);save(t,root,"output.json",response.Response.JSON)
 imageFile:=""
 if len(images)>0 {imageFile="images.json";save(t,root,imageFile,map[string]any{"images":images})}
 entry:=call{Index:1,Stage:"plan",Model:"recorded-model",Usage:response.Response.Usage,UsageValid:true,RequestFile:"request.json",ResponseFile:"response.json",OutputFile:"output.json",ImagesFile:imageFile}
 return &replay{Root:root,Operation:operation{Calls:[]call{entry}},MediaHashes:hashes},request
}

func TestArchivedResponseRequiresExactProductionRequest(t *testing.T) {
 for _,field:=range []string{"stage","system","input","schema","extra_call"} {
  t.Run(field,func(t *testing.T){
   model,request:=setupReplay(t,nil,false)
   switch field {
   case "stage":request.Stage="refine"
   case "system":request.System+=" changed"
   case "input":request.Input=json.RawMessage(`{"request":"different"}`)
   case "schema":request.ResponseSchema=json.RawMessage(`{"type":"array"}`)
   case "extra_call":if _,err:=model.Generate(context.Background(),request);err!=nil {t.Fatal(err)}
   }
   response,err:=model.Generate(context.Background(),request)
   if err==nil||!response.UsageInvalid||len(response.JSON)!=0||model.Mismatch==nil {t.Fatal("drift reused archived output")}
  })
 }
}

func TestRealImageConditioningCannotBeReplaced(t *testing.T) {
 original:=deepseek.Image{URI:"data:image/png;base64,AAECAw==",Detail:"original"}
 for _,field:=range []string{"uri","detail","omitted","unbound_fixture","tampered_hash"} {
  t.Run(field,func(t *testing.T){
   model,request:=setupReplay(t,[]deepseek.Image{original},false)
   images:=[]deepseek.Image{original}
   switch field {
   case "uri":images[0].URI="data:image/png;base64,AQ=="
   case "detail":images[0].Detail="low"
   case "omitted":images=nil
   case "unbound_fixture":model.MediaHashes=map[string]bool{}
   case "tampered_hash":var response responseRecord;if err:=readRelative(model.Root,"response.json",&response);err!=nil{t.Fatal(err)};response.Images[0].SHA256="wrong";save(t,model.Root,"response.json",response)
   }
   response,err:=model.GenerateWithImages(context.Background(),request,images)
   if err==nil||len(response.JSON)!=0||model.Mismatch==nil {t.Fatal("image drift reused archived output")}
  })
 }
 model,request:=setupReplay(t,[]deepseek.Image{original},false)
 if _,err:=model.GenerateWithImages(context.Background(),request,[]deepseek.Image{original});err!=nil||model.Position!=1 {t.Fatal("exact original image replay rejected",err)}
}

func TestFailedAttemptsAreReproducedWithUsage(t *testing.T) {
 model,request:=setupReplay(t,nil,true)
 response,err:=model.Generate(context.Background(),request)
 var typed *contextir.Error
 if !errors.As(err,&typed)||typed.Code!=contextir.CodeModelOutput||typed.Stage!="deepseek.response"||response.Usage.InputTokens!=3||model.Position!=1||model.Mismatch!=nil {t.Fatal("original failed attempt disappeared",err)}
}

func TestExplicitMediaMayBeBesideFixtureButRecordingMembersCannotEscape(t *testing.T) {
 root:=t.TempDir();fixture:=filepath.Join(root,"conformance");media:=filepath.Join(root,"third_party","image.png")
 if err:=os.Mkdir(fixture,0700);err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(filepath.Dir(media),0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(media,[]byte("pinned image fixture"),0600);err!=nil {t.Fatal(err)}
 path,err:=explicitMediaFile(fixture,"../third_party/image.png")
 if err!=nil||path!=media {t.Fatal("explicit producer media path was rejected",path,err)}
 hash:=digest([]byte("pinned image fixture"))
 image,err:=resolveFixtureImage(fixture,"../third_party/image.png",hash)
 if err!=nil||image.Detail!="original"||image.URI=="" {t.Fatal("original pinned media bytes did not resolve",err)}
 if _,err:=resolveFixtureImage(fixture,"../third_party/image.png",digest([]byte("other content")));err==nil {t.Fatal("wrong pinned fixture hash was accepted")}
 if err:=os.WriteFile(filepath.Join(filepath.Dir(media),"wrong.png"),[]byte("other content"),0600);err!=nil {t.Fatal(err)}
 if _,err:=resolveFixtureImage(fixture,"../third_party/wrong.png",hash);err==nil {t.Fatal("wrong source path reused original image hash")}
 if _,err:=resolveFixtureImage(fixture,"../third_party/missing.png",hash);err==nil {t.Fatal("missing explicit media source was accepted")}
 if _,err:=relativeFile(fixture,"../third_party/image.png");err==nil {t.Fatal("recording member escaped its immutable root")}
}

func TestRealDiskPersistenceRecompilesTheSameArtifact(t *testing.T) {
 document:=contextir.Document{Version:contextir.Version,Mode:contextir.ModeText,Target:contextir.Target{Duration:8000,AspectRatio:"16:9",FPS:contextir.Rational{Numerator:24,Denominator:1}},Style:"live-action photographic",Shots:[]contextir.Shot{{ID:"opening",Start:0,End:8000,Description:contextir.Text("A ceramic cup rests on a table.")}},Soundscape:"Rain taps against the window.",Music:"N/A"}
 artifact,err:=h3.Compile(document,h3.OfficialProfile());if err!=nil {t.Fatal(err)}
 result:=compiler.Result{Version:compiler.Version,Status:"compiled",Document:document,Artifact:artifact}
 directory:=filepath.Join(t.TempDir(),"saved")
 restored,err:=persisted(result,directory);if err!=nil||!equalValue(result,restored) {t.Fatal("disk restoration changed published result",err)}
 for _,name:=range []string{"result.json","document.json"} {if info,err:=os.Stat(filepath.Join(directory,name));err!=nil||info.Size()==0 {t.Fatal("file was not actually persisted",name,err)}}
 result.Artifact.Prompt+=" unauthorized change"
 if _,err:=persisted(result,filepath.Join(t.TempDir(),"bad"));err==nil {t.Fatal("artifact/document divergence was accepted")}
}
'''


class ReleaseConsumerControls(unittest.TestCase):
    def test_go_adapter_rejects_request_and_image_drift_and_retains_failures(self):
        with tempfile.TemporaryDirectory(prefix="contextir-release-controls-") as temporary:
            work = Path(temporary)
            (work / "main.go").write_bytes(release_consumer.TEMPLATE.read_bytes())
            (work / "main_test.go").write_text(GO_CONTROLS)
            run = subprocess.run(["go", "test", "-count=1", str(work / "main.go"), str(work / "main_test.go")],
                                 cwd=release_consumer.ROOT, capture_output=True, text=True, timeout=120)
            self.assertEqual(run.returncode, 0, run.stdout + run.stderr)

    def test_frozen_build_must_match_recording_and_downloaded_bytes(self):
        with tempfile.TemporaryDirectory(prefix="contextir-release-source-controls-") as temporary:
            work = Path(temporary)
            binary = work / "original-binary"
            binary.write_bytes(b"synthetic offline provenance control")
            runtime = {"go.mod": b"module test\n", "cmd/contextir-live-eval/main.go": b"package main\n"}
            hashes = {name: hashlib.sha256(data).hexdigest() for name, data in runtime.items()}
            archive = work / "sources.tar.gz"
            with tarfile.open(archive, "w:gz") as saved:
                for name, data in runtime.items():
                    entry = tarfile.TarInfo(name)
                    entry.size = len(data)
                    saved.addfile(entry, io.BytesIO(data))
            build = {"schema": "contextir-live-frozen-build/v1", "passed": True, "exit_code": 0,
                     "archived_sources_unchanged": True, "working_sources_unchanged": True,
                     "binary": str(binary), "binary_sha256": release_consumer.sha256_file(binary),
                     "runtime_sources": hashes, "sources": hashes, "source_archive": str(archive),
                     "source_archive_sha256": release_consumer.sha256_file(archive)}
            live = {"binary_sha256": build["binary_sha256"]}
            checked = {"files": dict(hashes)}
            self.assertTrue(release_consumer.check_live_source(build, live, checked)["downloaded_runtime_matches_live_build"])
            for field in ("binary", "downloaded_source", "archive"):
                with self.subTest(field=field):
                    changed_live = dict(live)
                    changed_checked = {"files": dict(hashes)}
                    changed_build = dict(build)
                    if field == "binary":
                        changed_live["binary_sha256"] = "0" * 64
                    elif field == "downloaded_source":
                        changed_checked["files"]["go.mod"] = "0" * 64
                    else:
                        changed_build["source_archive_sha256"] = "0" * 64
                    with self.assertRaises(CheckFailure):
                        release_consumer.check_live_source(changed_build, changed_live, changed_checked)


if __name__ == "__main__":
    unittest.main()
