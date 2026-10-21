"""Read only the finished, same-binary release-live-round6 recordings."""
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
RUN = ROOT / "conformance/reports/release-live-round6"
RECEIPT = ROOT / "conformance/reports/builds/release-live-round8-live-build.json"
OUTPUT = ROOT / "conformance/reports/release-live-round6-recording-inventory.json"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def member(name):
    rel = Path(name)
    if rel.is_absolute() or ".." in rel.parts:
        raise ValueError(f"Non-local recording member: {name}")
    path = RUN / rel
    if not path.is_file() or path.is_symlink():
        raise ValueError(f"Missing or indirect recording member: {name}")
    return path


def load(path):
    return json.loads(path.read_text())


report = load(RUN / "report.json")
receipt = load(RECEIPT)
if not report.get("finished_at"):
    raise SystemExit("The complete report is not finished; no final inventory was written.")
if report["binary_sha256"] != receipt["binary_sha256"]:
    raise ValueError("Report belongs to a different frozen binary")
for field, hash_field in [("binary", "binary_sha256"), ("source_archive", "source_archive_sha256")]:
    if digest(Path(receipt[field])) != receipt[hash_field]:
        raise ValueError(f"Frozen {field} changed")

fixture = load(member(report["fixture_file"]))
if digest(member(report["fixture_file"])) != report["fixture_sha256"]:
    raise ValueError("Complete input snapshot changed")
if digest(member(report["assertions_file"])) != report["assertions_sha256"]:
    raise ValueError("Separate business assertions changed")
inputs = {case["case_id"]: case for case in fixture["cases"]}
calls, errors, rejected, retries, negative, repaired, operations, missing = [], [], [], [], [], [], [], []
usage = Counter()
statuses = Counter()

for case in report["cases"]:
    completed = case.get("operations") or []
    for index in range(len(completed), 1 + len(inputs[case["case_id"]].get("changes") or [])):
        change = inputs[case["case_id"]]["changes"][index - 1] if index else ""
        missing.append({"case_id": case["case_id"], "operation_index": index,
                        "operation": "refine" if index else "compile",
                        "change_sha256": hashlib.sha256(change.encode()).hexdigest()})
    for operation in completed:
        op = {"case_id": case["case_id"], "operation_index": operation["index"],
              "operation": operation["operation"], "status": operation["status"],
              "passed": operation["passed"], "usage_complete": operation["usage_complete"],
              "error": operation.get("error"), "error_code": operation.get("error_code"),
              "options": case["options"], "result_file": operation.get("result_file"),
              "calls": []}
        statuses[op["status"]] += 1
        if op["status"] == "failed":
            statuses["expected_typed_rejection" if op["passed"] and op["error_code"] else "unexpected_failed_gate"] += 1
        op_negative = []
        clear_review_indices = []
        for call in operation.get("calls") or []:
            response = load(member(call["response_file"]))
            request = load(member(call["request_file"]))
            if request["stage"] != call["stage"]:
                raise ValueError("Call stage differs from the actual request")
            for field in ["request_file", "response_file", "output_file", "images_file"]:
                if call.get(field):
                    member(call[field])
            record = {"case_id": case["case_id"], "operation": op["operation"],
                      "operation_index": op["operation_index"], "call_index": call["index"],
                      "stage": call["stage"], "model": call["model"],
                      "usage": call["usage"], "usage_valid": call["usage_valid"],
                      "error": response.get("error"),
                      "rejected_output_present": bool(response.get("rejected_output")),
                      **{field: call.get(field, "") for field in ["request_file", "response_file", "output_file", "images_file"]}}
            if response["response"].get("model") != call["model"] or response["response"]["usage"] != call["usage"]:
                raise ValueError("Call usage/model differs from its actual full response")
            calls.append(record)
            op["calls"].append(record)
            usage.update({"input_tokens": call["usage"]["input_tokens"], "output_tokens": call["usage"]["output_tokens"]})
            if response.get("error"):
                errors.append(record)
            if response.get("rejected_output"):
                rejected.append(record)
            diagnostics = request.get("input", {}).get("diagnostics")
            if diagnostics:
                retries.append({"case_id": case["case_id"], "operation_index": op["operation_index"],
                                "call_index": call["index"], "stage": call["stage"], "diagnostics": diagnostics})
            if call["stage"] == "semantic_review" and call.get("output_file"):
                findings = load(member(call["output_file"])).get("findings", [])
                if findings:
                    entry = {**record, "findings": findings}
                    negative.append(entry)
                    op_negative.append(entry)
                else:
                    clear_review_indices.append(call["index"])
        if op_negative and op["status"] == "compiled" and any(i > op_negative[-1]["call_index"] for i in clear_review_indices):
            repaired.append({"case_id": case["case_id"], "operation_index": op["operation_index"],
                             "negative_review_call_indices": [c["call_index"] for c in op_negative],
                             "later_clear_review_call_indices": [i for i in clear_review_indices if i > op_negative[-1]["call_index"]],
                             "final_status": op["status"]})
        operations.append(op)

files = {}
for path in sorted(RUN.rglob("*")):
    if path.is_symlink():
        raise ValueError("A frozen recording became a symlink")
    if path.is_file():
        files[str(path.relative_to(RUN))] = digest(path)

inventory = {
    "schema": "contextir-complete-live-recording-inventory/v2",
    "created_at": datetime.now(timezone.utc).isoformat(),
    "reviewer_kind": "coding_agent_independent_artifact_review",
    "run_directory": str(RUN.relative_to(ROOT)),
    "binary_sha256": receipt["binary_sha256"],
    "product_git_head": receipt.get("git_head", receipt.get("product_git_head")),
    "source_archive_sha256": receipt["source_archive_sha256"],
    "build_receipt_sha256": digest(RECEIPT), "report_sha256": digest(RUN / "report.json"),
    "input_sha256": report["fixture_sha256"], "assertions_sha256": report["assertions_sha256"],
    "model": report["model"], "reasoning_effort": report["reasoning_effort"],
    "max_output_tokens": report["max_output_tokens"], "case_count": len(report["cases"]),
    "planned_operation_count": sum(1 + len(c.get("changes") or []) for c in fixture["cases"]),
    "executed_operation_count": len(operations), "operation_status_counts": dict(statuses),
    "unexecuted_operations": missing, "model_call_count": len(calls),
    "provider_error_attempt_count": len(errors), "provider_errors": errors,
    "rejected_json_or_protocol_output_attempt_count": len(rejected),
    "rejected_json_or_protocol_outputs": rejected,
    "classification_note": "Provider adapter errors and rejected JSON/protocol outputs can describe the same attempt; their counts must not be added as distinct calls.",
    "pipeline_validation_retry_request_count": len(retries),
    "pipeline_validation_retry_requests": retries,
    "negative_semantic_review_attempt_count": len(negative),
    "negative_semantic_finding_count": sum(len(c["findings"]) for c in negative),
    "negative_semantic_reviews": negative,
    "operations_repaired_after_negative_semantic_review_count": len(repaired),
    "operations_repaired_after_negative_semantic_review": repaired,
    "usage_totals": dict(usage), "operations": operations,
    "files": files, "formal_report_passed": report["passed"],
    "semantic_release_accepted": None,
    "semantic_acceptance_note": "Inventory records actual calls and formal gates; independent role/core/boundary reviews remain separate acceptance evidence. A compiled result is not evidence that its first draft or arbitrary generated media was correct.",
    "scope": "Only this complete main batch is counted. Separate original-Licence helper recordings, other binaries and historical batches are excluded.",
}
OUTPUT.write_text(json.dumps(inventory, ensure_ascii=False, indent=2) + "\n")
print(json.dumps({key: inventory[key] for key in ["planned_operation_count", "executed_operation_count", "operation_status_counts", "model_call_count", "provider_error_attempt_count", "negative_semantic_review_attempt_count", "negative_semantic_finding_count", "usage_totals"]}, indent=2))
