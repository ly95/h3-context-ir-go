"""Full Go Draft plus native artifact rendering; other boundaries stay explicit."""
import pytest_go_combined as combined
import pytest_go_core as core

pytest_collection_modifyitems=combined.pytest_collection_modifyitems
pytest_runtest_logstart=combined.pytest_runtest_logstart
pytest_runtest_logfinish=combined.pytest_runtest_logfinish
pytest_runtest_logreport=combined.pytest_runtest_logreport

def pytest_sessionfinish(session,exitstatus):
    combined.pytest_sessionfinish(session,exitstatus)
    import json
    import os
    from pathlib import Path
    path=Path(os.environ['CONTEXTIR_CAPTURE_PATH']);data=json.loads(path.read_text())
    calls=data['go_core_calls']
    for item in data['collected']:
        node=item['nodeid'];drafts=[x for x in calls if x['nodeid']==node and x['function']=='h3ir.draft.deterministic_draft'];renders=[x for x in calls if x['nodeid']==node and x['function']=='h3ir.render.native_draft']
        item['full_native_draft_calls']=len(drafts)
        item['full_native_draft_artifacts']=sum(not x.get('error')for x in drafts)
        item['native_draft_artifact_render_calls']=len(renders)
        item['native_plan_only']=not drafts and any(x['nodeid']==node and x['function']=='h3ir.plan.build_plan'for x in calls)
    data['implementation_under_test']='Complete original deterministic_draft entries invoke public Go Compiler.Draft with the original explicit mode. Their render_ir entries invoke native h3.Compile on the authoritative Document and return its actual Artifact; the source Python writer never generates these outputs. Separately recorded native semantic boundaries and other core components are distinguished per call. Other source plans/rendering and orchestration still execute Python.'
    data['adapter_differences'].append('Full Draft explicitly selects AllowPartialPerception for original supplied cards without claiming full media observation. Unsupported profile or edit policies and missing evidence remain explicit failures. Original style-boundary output is explicit only where the recorded native Draft arguments include style_phrase. Source observational metadata may be projected from actual native Evidence separately from target Document.Subjects; packet.dto_projections records this policy when selected. Source {{CAM}} internal placeholders, when recorded, project actual validated camera Events only; native Artifact prose always comes from the unchanged authoritative Document.')
    path.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')

def pytest_configure(config):
    combined.pytest_configure(config)
    import draft_bridge
    draft_bridge.install(core.native_operation,core.cap.plain)
