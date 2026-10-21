#!/usr/bin/env python3
"""Independently inspect final live acceptance results, never call a model.

Expected values come from a separate assertion file and are not sent to the
provider. This checker verifies literal channels, reference graphs, source
roles, endpoint preservation, publication hashes and refinement dependencies.
It does not certify all free-form prose, real acoustic/video perception, or
generated video quality. The design manifest records that evidence boundary.
"""
from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
from pathlib import Path


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def go_json(value) -> bytes:
    # Artifacts use encoding/json on structs, preserving their field order.
    text = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    for old, new in [("&", "\\u0026"), ("<", "\\u003c"), (">", "\\u003e"),
                     ("\u2028", "\\u2028"), ("\u2029", "\\u2029")]:
        text = text.replace(old, new)
    return text.encode()


def pointer(value, path):
    if not path:
        return value
    if not path.startswith("/"):
        raise ValueError("JSONPointer must start with /")
    for part in path[1:].split("/"):
        part = part.replace("~1", "/").replace("~0", "~")
        value = value[int(part)] if isinstance(value, list) else value[part]
    return value


def strings(value):
    if isinstance(value, str):
        yield value
    elif isinstance(value, list):
        for item in value:
            yield from strings(item)
    elif isinstance(value, dict):
        for item in value.values():
            yield from strings(item)


def literal_surface(document):
    """Only emitted descriptions; source observations may contain transcripts."""
    result = [document.get("summary", []), document.get("style", ""),
              document.get("soundscape", ""), document.get("music", "")]
    for kind in ("subjects", "references", "speakers"):
        result.extend(row.get("description", "") for row in document.get(kind, []))
    for shot in document.get("shots", []):
        result.append(shot.get("description", []))
        result.extend(event.get("description", []) for event in shot.get("events", []))
    return "\n".join(strings(result))


def distinct_source_subjects(subjects, sources):
    """Requested independent identities stay independent; extra facts are legal."""
    requested = set(sources)
    selected = [{a.get("source", {}).get("asset_id") for a in subject.get("sources", [])}
                & requested for subject in subjects.values()]
    return all({source} in selected for source in requested)


def dialogue_semantics(document):
    # Generated IDs are graph addresses, not additional caller-owned words.
    return [(line.get("kind") or "speech", line.get("text"), line.get("language"),
             line.get("delivery", ""), bool(line.get("voiceover")))
            for line in document.get("dialogue", [])]


def selected_source_attributes(result, source, accepted):
    """Trace a selected property through pre-projection IDs to physical media.

    Subject/picture labels are graph addresses here, not expected descriptions.
    A property in a target descriptor alone cannot satisfy this assertion.
    """
    media = {row.get("label"): row.get("source", {}).get("asset_id")
             for row in result.get("artifact", {}).get("manifest", {}).get("media", [])}
    source_ids = set()
    for row in result.get("licence", {}).get("subjects", []):
        label = re.fullmatch(r"<Subject ([1-9][0-9]*)>", row.get("label", ""))
        if label and any(media.get(address) == source for address in row.get("sources", [])):
            source_ids.add("subject-" + label.group(1))
    targets = {row["id"]: row for row in result.get("document", {}).get("subjects", [])}
    for projection in result.get("structure", {}).get("subject_projections", []):
        target = targets.get(projection.get("subject_id"), {})
        selected = {attribute for row in projection.get("sources", [])
                    if row.get("subject_id") in source_ids for attribute in row.get("attributes", [])}
        final = set(target.get("attributes", []))
        bound = any(row.get("source", {}).get("asset_id") == source
                    for row in target.get("sources", []))
        if bound and any(attribute in selected and attribute in final for attribute in accepted):
            return True
    return False


def media_address_is_rendered(row, document, prompt):
    # The published paired-frame protocol addresses its pictures without angle
    # brackets in the instruction header. Those addresses are still bound media.
    if document.get("mode") == "fl2va" and row.get("socket") in ("first_frame", "last_frame"):
        last = len(document.get("shots", []))
        duration = document.get("target", {}).get("duration_ms", 0)
        expected = ("How the reference pictures align with the target video — "
                    "Picture 1 (from Shot 1) aligns with the 0.00-second mark of the target video; "
                    f"Picture 2 (from Shot {last}) aligns with the {duration/1000:.2f}-second mark of the target video.")
        first = row["socket"] == "first_frame"
        role, anchor, label = ("first_frame", 0, "<Picture 1>") if first else ("last_frame", duration, "<Picture 2>")
        return (row.get("label") == label and prompt.startswith(expected + "\n")
                and any(role in use.get("roles", []) and use.get("anchor_ms") == anchor
                        for use in row.get("target_uses", [])))
    return bool(row.get("label")) and row["label"] in prompt


def caller_property_reaches_source_target(result, source):
    """Check explicit claim provenance and publication, not color-word intent.

    The semantic reviewer separately compares the actual new garment/color to
    the original request. Here quoted authority cannot live only in metadata.
    """
    media = {row.get("label"): row.get("source", {}).get("asset_id")
             for row in result.get("artifact", {}).get("manifest", {}).get("media", [])}
    labels = {row.get("label") for row in result.get("licence", {}).get("subjects", [])
              if any(media.get(address) == source for address in row.get("sources", []))}
    brief = result.get("brief", {})
    caller = [brief.get("intent", "")] + [row.get("text", "") for row in brief.get("constraints", [])]
    quoted = lambda quote: bool(quote) and any(quote in text for text in caller)
    targets = [row for row in result.get("document", {}).get("subjects", [])
               if any(edge.get("source", {}).get("asset_id") == source for edge in row.get("sources", []))]
    for identity in result.get("licence", {}).get("identity", []):
        if identity.get("label") not in labels or identity.get("descriptor_source") != "request" or not quoted(identity.get("request_quote")):
            continue
        supported = {claim.get("value") for claim in identity.get("claims", [])
                     if claim.get("value") and quoted(claim.get("quote"))}
        selected = {value for value in identity.get("requested_attributes", [])
                    if value in supported or quoted(value)}
        if selected and any(selected <= set(target.get("attributes", [])) for target in targets):
            return True
    return False


def inspect(result, semantic, previous=None):
    problems = []
    def require(ok, message):
        if not ok:
            problems.append(message)
    artifact = result.get("artifact", {})
    document = result.get("document", {})
    prompt = artifact.get("prompt", "")
    if semantic.get("no_artifact"):
        require(not prompt and not artifact.get("sha256"), "failed/needs_input result published an artifact")
        return problems
    require(result.get("status") == "compiled", "publication did not finish compiled")
    if not prompt:
        return problems + ["compiled result lacks final prompt"]
    require(artifact.get("mode") == document.get("mode"), "artifact mode differs from Document")
    require(artifact.get("target") == document.get("target"), "artifact target differs from Document")
    require(sha(prompt.encode()) == artifact.get("prompt_sha256"), "prompt SHA does not match final bytes")
    for field, value in [("document", document), ("manifest", artifact.get("manifest", {})),
                         ("profile", artifact.get("profile", {}))]:
        require(sha(go_json(value)) == artifact.get(field + "_sha256"), field + " SHA does not match publication")
    identity = copy.deepcopy(artifact)
    identity["sha256"] = ""
    require(sha(go_json(identity)) == artifact.get("sha256"), "artifact identity SHA does not match complete artifact")

    target_end = document.get("target", {}).get("duration_ms", 0)
    assets = {x["id"]: x for x in document.get("assets", [])}
    subjects = {x["id"]: x for x in document.get("subjects", [])}
    references = {x["id"]: x for x in document.get("references", [])}
    speakers = {x["id"]: x for x in document.get("speakers", [])}
    shots = document.get("shots", [])
    shot_ids = {x["id"] for x in shots}
    manifest = artifact.get("manifest", {})
    media = manifest.get("media", [])
    actual_source_roles = {}
    source_ids = set()
    for subject in subjects.values():
        for attribution in subject.get("sources", []):
            source = attribution.get("source", {}).get("asset_id")
            source_ids.add(source)
            require(source in assets, "subject attribution names an unattached source")
            actual_source_roles.setdefault(source, set()).add(attribution.get("role"))
    for reference in references.values():
        source = reference.get("source", {}).get("asset_id")
        source_ids.add(source)
        require(source in assets, "reference names an unattached source")
        actual_source_roles.setdefault(source, set()).update(reference.get("roles", []))
        if reference.get("speaker_id"):
            require(reference["speaker_id"] in speakers, "reference names a missing speaker")
        require(set(reference.get("shot_ids", [])) <= shot_ids, "reference names missing target shots")
        matches = [use for row in media if row.get("source") == reference.get("source")
                   for use in row.get("target_uses", []) if use.get("reference_id") == reference["id"]]
        require(len(matches) == 1, "reference lacks exactly one manifest target use")
        if len(matches) == 1:
            for key in ("roles", "retention", "anchor_ms", "speaker_id", "replaces", "paired_video_id"):
                require(matches[0].get(key) == reference.get(key), "manifest target use differs in " + key)
    require({x.get("source", {}).get("asset_id") for x in media} == source_ids,
            "manifest media source set differs from final Document source set")
    for row in media:
        require(row.get("source", {}).get("asset_id") in assets, "manifest names an unattached asset")
        require(media_address_is_rendered(row, document, prompt), "manifest media address is absent or incorrectly rendered")
        for use in row.get("target_uses", []):
            for target in use.get("shots", []):
                require(target.get("id") in shot_ids, "manifest use names a missing shot")
                interval = target.get("interval", {})
                require(0 <= interval.get("start_ms", -1) < interval.get("end_ms", -1) <= target_end,
                        "manifest target interval escapes final timeline")
    # A dormant voice-property Speaker is legitimate metadata but receives no
    # (SN) vocal label. The manifest numbers actual vocal performers only.
    line_definitions = {line["id"]: line for line in document.get("dialogue", [])}
    performing_speakers = {speaker for shot in shots for event in shot.get("events", [])
                           if event.get("dialogue") for speaker in line_definitions.get(
                               event["dialogue"].get("line_id"), {}).get("speaker_ids", [])}
    for name, actual in (("subjects", subjects), ("references", references), ("speakers", performing_speakers)):
        require({x.get("id") for x in manifest.get(name, [])} == set(actual),
                "manifest " + name + " ID set differs from Document")
    labels = {row.get("label") for name in ("media", "subjects", "references", "speakers")
              for row in manifest.get(name, [])}
    # Parsing rendered protocol labels is not intent inference.
    for label in re.findall(r"<(?:Subject|Picture|Image|Video|Audio) \d+>|\(S\d+\)", prompt):
        require(label in labels, "unbound generated prompt label: " + label)
    last_end = 0
    for shot in shots:
        require(shot.get("start_ms") == last_end, "shots are not contiguous and ordered")
        require(last_end < shot.get("end_ms", -1) <= target_end, "shot endpoint is outside timeline")
        last_end = shot.get("end_ms", -1)
        for event in shot.get("events", []):
            if "at_ms" in event:
                require(shot["start_ms"] <= event["at_ms"] < shot["end_ms"], "event time escapes its shot")
    require(last_end == target_end, "shots do not reach final target end")
    for speaker in speakers.values():
        require(not speaker.get("subject_id") or speaker["subject_id"] in subjects, "speaker binds a missing subject")
        require(not speaker.get("voice_reference_id") or speaker["voice_reference_id"] in references,
                "speaker binds a missing voice reference")

    lines = document.get("dialogue", [])
    if "exact_dialogue" in semantic:
        require([x.get("text") for x in lines] == semantic["exact_dialogue"], "final dialogue definitions differ from fixed words")
    if "dialogue_languages" in semantic:
        require([x.get("language") for x in lines] == semantic["dialogue_languages"], "fixed dialogue language changed")
    line_map = {x["id"]: x for x in lines}
    parts = {key: [] for key in line_map}
    parts_shots = {key: set() for key in line_map}
    for shot in shots:
        for event in shot.get("events", []):
            part = event.get("dialogue")
            if part:
                require(part.get("line_id") in line_map, "dialogue event references unknown line")
                if part.get("line_id") in parts:
                    parts[part["line_id"]].append(part)
                    parts_shots[part["line_id"]].add(shot["id"])
    for line in lines:
        utterance = parts[line["id"]]
        require("".join(p.get("text", "") for p in utterance) == line.get("text"),
                "emitted dialogue parts do not concatenate to exact final utterance")
        require(set(line.get("speaker_ids", [])) <= set(speakers), "dialogue names missing speaker")
        if semantic.get("literal_only_dialogue"):
            require(line["text"] not in literal_surface(document), "fixed words leaked into emitted non-dialogue prose")
        if semantic.get("dialogue_crosses_shots"):
            require(len(parts_shots[line["id"]]) >= 2, "requested continuous line does not span shots")
            for i, part in enumerate(utterance):
                require(bool(part.get("continues_from_previous")) == (i > 0), "incorrect continues-from flag")
                require(bool(part.get("continues_to_next")) == (i < len(utterance)-1), "incorrect continues-to flag")
        if semantic.get("offscreen_only"):
            require(line.get("voiceover") is True, "offscreen words became onscreen dialogue")
            require(all(speakers[s].get("onscreen") is False and not speakers[s].get("subject_id")
                        for s in line.get("speaker_ids", []) if s in speakers), "voiceover invented a visual speaker")
    if "visible_text" in semantic:
        errors.extend(visible_text_errors(document, prompt, semantic))
    for source in semantic.get("dormant_voice_bindings", []):
        voice_references = [r for r in references.values()
                            if r.get("source", {}).get("asset_id") == source
                            and r.get("kind") == "audio" and "voice" in r.get("roles", [])]
        require(bool(voice_references), "dormant voice source lacks its typed audio reference")
        for reference in voice_references:
            speaker = speakers.get(reference.get("speaker_id"), {})
            require(bool(speaker) and speaker.get("voice_reference_id") == reference["id"],
                    "dormant voice reference and speaker are not reciprocally bound")
            require(not speaker.get("subject_id") and speaker.get("onscreen") is not True,
                    "dormant voice property invented a visible speaking actor")
        require(not any(a.get("source", {}).get("asset_id") == source
                        for s in subjects.values() for a in s.get("sources", [])),
                "audio voice source was attributed to a visual subject")
    if "source_subject_count" in semantic:
        require(sum(bool(x.get("sources")) for x in subjects.values()) == semantic["source_subject_count"],
                "wrong number of source-bound target identities")
    if "distinct_source_subjects" in semantic:
        require(distinct_source_subjects(subjects, semantic["distinct_source_subjects"]),
                "independent requested source identities were merged or lost")
    for source in semantic.get("required_sources", []):
        require(source in source_ids, "required source disappeared: " + source)
    for source in semantic.get("absent_sources", []):
        require(source not in assets and source not in source_ids, "removed source remains bound: " + source)
    for source, allowed in semantic.get("source_roles", {}).items():
        require(bool(actual_source_roles.get(source, set()) & set(allowed)), "declared source role is not used in final Document: " + source)
    for source, at in semantic.get("required_anchor", {}).items():
        expected = target_end if at == "end" else at
        require(any(r.get("source", {}).get("asset_id") == source and r.get("anchor_ms") == expected
                    for r in references.values()), "missing exact frame anchor: " + source)
    for source in semantic.get("fully_preserved_sources", []):
        relevant = [r for r in references.values() if r.get("source", {}).get("asset_id") == source]
        require(bool(relevant) and all(r.get("retention") == "fully_preserved" for r in relevant),
                "actual frame pixels are not fully preserved: " + source)
    for wanted in semantic.get("merged_subject_source_sets", []):
        require(any({a.get("source", {}).get("asset_id") for a in s.get("sources", [])} == set(wanted)
                    for s in subjects.values()), "single target lost its selected multi-source projection")
    attributes = {attribute for s in subjects.values() for attribute in s.get("attributes", [])}
    for attribute in semantic.get("required_subject_attributes", []):
        require(attribute in attributes, "required accepted target property absent: " + attribute)
    for attribute in semantic.get("absent_subject_attributes", []):
        require(attribute not in attributes, "replaced source property remains in target: " + attribute)
    for source in semantic.get("caller_property_from_sources", []):
        require(caller_property_reaches_source_target(result, source),
                "caller-authorized property did not reach the source-bound target: " + source)
    for alternatives in semantic.get("required_subject_attribute_alternatives", []):
        require(bool(attributes & set(alternatives)),
                "required accepted target property absent: " + " or ".join(alternatives))
    for source, alternatives in semantic.get("required_source_attributes", {}).items():
        for accepted in alternatives:
            require(selected_source_attributes(result, source, accepted),
                    "selected target property missing from exact source " + source + ": " + " or ".join(accepted))
    if semantic.get("no_subject_pose"):
        require(all(not s.get("pose") and not s.get("pose_licensed") for s in subjects.values() if s.get("sources")),
                "content reference was promoted into source-pose lock")
    if semantic.get("replacement_motion_required") and semantic.get("declared_role") != "placed_subject":
        require(bool(result.get("replacement_motion_obligations")), "incoming/replacement motion obligations absent")
    if "preserve_dialogue_from_op" in semantic and previous:
        base = previous[semantic["preserve_dialogue_from_op"]]
        require(dialogue_semantics(document) == dialogue_semantics(base.get("document", {})),
                "Refine altered unchanged fixed dialogue semantics")
        require(result.get("brief", {}).get("dialogue", []) == base.get("brief", {}).get("dialogue", []),
                "Refine altered unchanged caller-owned Brief.Dialogue")
    if "preserve_evidence_from_op" in semantic and previous:
        base = previous[semantic["preserve_evidence_from_op"]]
        removed = set(semantic.get("allow_removed_evidence", []))
        expected = [e for e in base.get("evidence", []) if e.get("asset_id") not in removed]
        require(result.get("evidence", []) == expected, "Refine changed unchanged actual observation evidence")
    if "preserve_projection_sources_from_op" in semantic and previous:
        def sets(r):
            return sorted(sorted(a["source"]["asset_id"] for a in s.get("sources", []))
                          for s in r.get("document", {}).get("subjects", []) if len(s.get("sources", [])) > 1)
        require(sets(result) == sets(previous[semantic["preserve_projection_sources_from_op"]]),
                "Refine rebound selected identities by position instead of stable physical source")
    return problems


def basic(result, check):
    try:
        kind = check["kind"]
        value = pointer(result, check.get("path", ""))
        prompt = result.get("artifact", {}).get("prompt", "")
        if kind == "json_equals":
            return value == check["equals"]
        if kind == "json_contains":
            return isinstance(value, str) and check["contains"] in value
        if kind == "array_length":
            return isinstance(value, list) and len(value) == check["count"]
        if kind == "prompt_contains":
            return check["contains"] in prompt
        if kind == "prompt_excludes":
            return check["contains"] not in prompt
        if kind == "prompt_count":
            return prompt.count(check["contains"]) == check["count"]
    except (KeyError, TypeError, IndexError, ValueError):
        return False
    return False


def run(report_path, fixture_path, assertions_path):
    report = json.loads(report_path.read_text())
    fixture = json.loads(fixture_path.read_text())
    expected = json.loads(assertions_path.read_text())
    root = report_path.parent
    failures = []
    cases = {c["case_id"]: c for c in report.get("cases", [])}
    ids = [c["case_id"] for c in fixture["cases"]]
    if len(cases) != len(ids) or set(cases) != set(ids) or set(expected["cases"]) != set(ids):
        failures.append("case inventory differs from frozen fixture/assertion inventory")
    for key, path in (("fixture_sha256", fixture_path), ("assertions_sha256", assertions_path)):
        if report.get(key) != sha(path.read_bytes()):
            failures.append(key + " differs from actual frozen input bytes")
    results = []
    for item in fixture["cases"]:
        cid = item["case_id"]
        cr = cases.get(cid, {})
        operations = cr.get("operations", [])
        specs = expected["cases"][cid]["operations"]
        previous = []
        for index, spec in enumerate(specs):
            errors = []
            if index >= len(operations):
                results.append({"case_id": cid, "operation_index": index, "passed": False, "errors": ["operation never ran"]})
                continue
            operation = operations[index]
            path = (root / operation["result_file"]).resolve()
            if root.resolve() not in path.parents:
                raise ValueError("recorded result path escapes report root")
            result = json.loads(path.read_text())
            status = spec.get("status", "compiled")
            if operation.get("status") != status or result.get("status") != status:
                errors.append("actual status differs from expected " + status)
            code = operation.get("error_code", operation.get("error", {}).get("code") if operation.get("error") else None)
            allowed_codes = spec.get("error_codes", [spec["error_code"]] if spec.get("error_code") else [])
            if allowed_codes:
                if code not in allowed_codes:
                    errors.append("typed failure code differs from independent expected error")
            elif operation.get("error"):
                errors.append("unexpected operation error")
            for check in spec.get("checks", []):
                if not basic(result, check):
                    errors.append("independent final-result check failed: " + json.dumps(check, ensure_ascii=False))
            semantic = spec.get("semantic", {})
            if status == "compiled":
                errors.extend(inspect(result, semantic, previous))
                if not operation.get("roundtrip"):
                    errors.append("persisted Go Document -> Compile roundtrip did not pass")
                if not result.get("usage_complete") or any(not c.get("usage_valid") for c in operation.get("calls", [])):
                    errors.append("successful operation has incomplete or invalid usage accounting")
            elif semantic.get("no_artifact"):
                errors.extend(inspect(result, semantic, previous))
            if semantic.get("no_model_calls") and operation.get("calls"):
                errors.append("invalid boundary input made model calls")
            if index > 0 and ("preserve_evidence_from_op" in semantic) and any(c.get("images") for c in operation.get("calls", [])):
                errors.append("Refine made fresh image observations instead of reusing unchanged evidence")
            for call in operation.get("calls", []):
                for name in ("request_file", "response_file"):
                    file = (root / call[name]).resolve()
                    if root.resolve() not in file.parents or not file.is_file():
                        errors.append("actual stage recording missing: " + name)
            if item.get("observation_scope") == "real_png_live_observation" and index == 0 and status == "compiled":
                observed = {image["fixture_sha256"] for call in operation.get("calls", []) for image in call.get("images", [])}
                wanted = {media["sha256"] for media in item.get("media", {}).values()}
                if not wanted or observed != wanted:
                    errors.append("fresh live PNG observation does not cover actual pinned fixture bytes")
            previous.append(result)
            results.append({"case_id": cid, "operation_index": index, "expected_status": status,
                            "passed": not errors, "errors": errors})
    return {"schema": "contextir-live-independent-verification/v1", "passed": not failures and all(r["passed"] for r in results),
            "report_sha256": sha(report_path.read_bytes()), "fixture_sha256": sha(fixture_path.read_bytes()),
            "assertions_sha256": sha(assertions_path.read_bytes()), "inventory_errors": failures,
            "operations": results, "total": len(results), "passed_operations": sum(r["passed"] for r in results),
            "failed_operations": sum(not r["passed"] for r in results),
            "evidence_scope": "Final publication, typed source graph, literal/timing/role invariants; free-form semantic prose and generated video quality require separately recorded review."}


def visible_text_errors(document, prompt, semantic):
    """Count typed displays, not repetitions describing the same visible prop.

    Free prose can still introduce another sign or a readable continuation with
    no event. Its physical meaning needs the separate composition/independent
    semantic review; sharing a shot does not automatically make it one display.
    """
    displays = [(shot["id"], event["visible_text"])
                for shot in document.get("shots", []) for event in shot.get("events", [])
                if event.get("visible_text")]
    errors = []
    if [text for _, text in displays] != semantic["visible_text"]:
        errors.append("visible lettering changed, duplicated or was omitted")
    if "visible_text_shots" in semantic and [shot for shot, _ in displays] != semantic["visible_text_shots"]:
        errors.append("visible lettering is in an unauthorized shot")
    for word in semantic["visible_text"]:
        if '"' + word + '"' not in prompt:
            errors.append("exact visible text is absent from the quoted rendered channel")
    return errors


def self_test():
    source = Path(__file__).parent / "reports/remaining-highlevel-compile-v2/transcript-model-selection/result.json"
    r = json.loads(source.read_text())["result"]
    assert not inspect(r, {"exact_dialogue": ["We close at six, not half past."]}), "known archived publication rejected"
    for name, mutate in [
        ("prompt hash", lambda d: d["artifact"].update(prompt=d["artifact"]["prompt"]+"x")),
        ("source graph", lambda d: d["document"]["references"][0]["source"].update(asset_id="phantom")),
        ("literal words", lambda d: d["document"]["dialogue"][0].update(text="changed")),
        ("manifest role", lambda d: d["artifact"]["manifest"]["media"][0]["target_uses"][0].update(roles=["music"])),
        ("publication hash", lambda d: d["artifact"].update(sha256="0"*64)),
    ]:
        wrong = copy.deepcopy(r)
        mutate(wrong)
        assert inspect(wrong, {"exact_dialogue": ["We close at six, not half past."]}), name + " drift escaped checker"
    subjects = {"a": {"sources": [{"source": {"asset_id": "one"}}]},
                "b": {"sources": [{"source": {"asset_id": "two"}}]},
                "extra": {"sources": [{"source": {"asset_id": "one"}}]}}
    assert distinct_source_subjects(subjects, ["one", "two"])
    assert not distinct_source_subjects({"merged": {"sources": [
        {"source": {"asset_id": "one"}}, {"source": {"asset_id": "two"}}]}}, ["one", "two"])
    renamed = copy.deepcopy(r["document"])
    renamed["dialogue"][0].update(id="another-line", speaker_ids=["another-speaker"])
    assert dialogue_semantics(renamed) == dialogue_semantics(r["document"])
    renamed["dialogue"][0]["text"] = "changed words"
    assert dialogue_semantics(renamed) != dialogue_semantics(r["document"])
    release = Path(__file__).parent / "reports"
    positive_path = release / "release-live-round2/single-target-model-merge/op-00-compile/result.json"
    negative_path = release / "release-live-round1/single-target-model-merge/op-00-compile/result.json"
    positive = json.loads(positive_path.read_text())
    accepted = ["wheels", "wheels from the second picture"]
    assert selected_source_attributes(positive, "asset-2", accepted)
    assert not selected_source_attributes(positive, "asset-1", accepted), "wheel source swapped"
    old_failure = json.loads(negative_path.read_text())
    assert not selected_source_attributes(old_failure, "asset-2", accepted), "first-wave missing wheels accepted"
    for name, value in [("descriptor only", []), ("unselected body", ["carbon fibre body"])]:
        wrong = copy.deepcopy(positive)
        wrong["structure"]["subject_projections"][0]["sources"][1]["attributes"] = value
        assert not selected_source_attributes(wrong, "asset-2", accepted), name + " satisfied source property"
    long_form = copy.deepcopy(positive)
    long_form["structure"]["subject_projections"][0]["sources"][1]["attributes"] = [accepted[1]]
    long_form["document"]["subjects"][0]["attributes"][1] = accepted[1]
    assert selected_source_attributes(long_form, "asset-2", accepted)
    paired = json.loads((release / "release-live-round2/paired-first-last-style-transform/op-00-compile/result.json").read_text())
    for row in paired["artifact"]["manifest"]["media"]:
        assert media_address_is_rendered(row, paired["document"], paired["artifact"]["prompt"])
        wrong_header = paired["artifact"]["prompt"].replace("8.00-second mark", "7.00-second mark", 1)
        assert not media_address_is_rendered(row, paired["document"], wrong_header)
        wrong_label = copy.deepcopy(row)
        wrong_label["label"] = "<Picture 9>"
        assert not media_address_is_rendered(wrong_label, paired["document"], paired["artifact"]["prompt"])
    property_control = {
        "brief": {"intent": "Change the shirt to the new requested color."},
        "artifact": {"manifest": {"media": [{"label": "<Picture 1>", "source": {"asset_id": "one"}}]}},
        "licence": {"subjects": [{"label": "<Subject 1>", "sources": ["<Picture 1>"]}],
                    "identity": [{"label": "<Subject 1>", "descriptor_source": "request",
                                  "request_quote": "Change the shirt", "requested_attributes": ["new-color shirt"],
                                  "claims": [{"value": "new-color shirt", "quote": "new requested color"}]}]},
        "document": {"subjects": [{"attributes": ["new-color shirt", "unchanged jeans"],
                                     "sources": [{"source": {"asset_id": "one"}}]}]},
    }
    assert caller_property_reaches_source_target(property_control, "one")
    for name, mutate in [
        ("descriptor only", lambda d: d["document"]["subjects"][0].update(attributes=["unchanged jeans"], descriptor="new-color shirt")),
        ("wrong source", lambda d: d["document"]["subjects"][0]["sources"][0]["source"].update(asset_id="other")),
        ("unquoted authority", lambda d: d["licence"]["identity"][0].update(request_quote="Words the caller never wrote")),
        ("unsupported new property", lambda d: d["licence"]["identity"][0].update(claims=[])),
    ]:
        wrong = copy.deepcopy(property_control)
        mutate(wrong)
        assert not caller_property_reaches_source_target(wrong, "one"), name + " escaped property provenance"
    same_sign = {
        "shots": [{"id": "shot-1", "description": [{"text": "One sign reads 出口 EXIT 07."}],
                   "events": [{"kind": "visible_text", "visible_text": "出口 EXIT 07"}]},
                  {"id": "shot-2", "description": [{"text": "Only tile and pillars remain visible."}], "events": []}]
    }
    sign_semantic = {"visible_text": ["出口 EXIT 07"], "visible_text_shots": ["shot-1"]}
    same_sign_prompt = 'One sign reads 出口 EXIT 07. The visible text reads "出口 EXIT 07".'
    assert same_sign_prompt.count("出口 EXIT 07") == 2
    assert not visible_text_errors(same_sign, same_sign_prompt, sign_semantic)
    for name, mutate in [
        ("duplicated typed display", lambda d: d["shots"][0]["events"].append(copy.deepcopy(d["shots"][0]["events"][0]))),
        ("changed literal", lambda d: d["shots"][0]["events"][0].update(visible_text="出口 EXIT 08")),
        ("wrong shot", lambda d: d["shots"][1]["events"].append(d["shots"][0]["events"].pop())),
        ("extra visible words", lambda d: d["shots"][0]["events"].append({"kind": "visible_text", "visible_text": "EXTRA"})),
    ]:
        wrong = copy.deepcopy(same_sign)
        mutate(wrong)
        assert visible_text_errors(wrong, same_sign_prompt, sign_semantic), name + " escaped typed display contract"
    assert visible_text_errors(same_sign, "The rendered fixed words disappeared.", sign_semantic)
    print(json.dumps({"self_test": "passed", "positive_publication": 1, "negative_mutations": 5,
                      "independent_sources_positive_and_negative": True,
                      "semantic_dialogue_id_change_and_word_drift": True,
                      "source_bound_equivalent_wheels_positive_and_firstwave_negative": True,
                      "paired_frame_official_addresses_and_header_drift": True,
                      "caller_property_provenance_and_final_target": True,
                      "same_prop_echo_allowed_typed_display_word_count_shot_still_strict": True}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report", type=Path)
    parser.add_argument("--fixture", type=Path, default=Path(__file__).with_name("release_acceptance_inputs.json"))
    parser.add_argument("--assertions", type=Path, default=Path(__file__).with_name("release_acceptance_assertions.json"))
    parser.add_argument("--out", type=Path)
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
    else:
        if not args.report or not args.out:
            parser.error("--report and --out are required")
        result = run(args.report, args.fixture, args.assertions)
        with args.out.open("x") as stream:
            json.dump(result, stream, ensure_ascii=False, indent=2)
            stream.write("\n")
        print(json.dumps({k: result[k] for k in ("passed", "total", "passed_operations", "failed_operations")}))
        raise SystemExit(0 if result["passed"] else 1)
