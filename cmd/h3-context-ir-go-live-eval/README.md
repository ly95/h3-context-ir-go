# Native live acceptance recorder

This command exercises the public `compiler.Brief` workflow with an explicitly
configured DeepSeek Responses model. It does not read dotenv, expected model
outputs, or historical recordings. Input media must be public or synthetic and
must be identified by a fixture-relative path and its full SHA256 digest.
Recording user-owned/private content requires a separate approved policy.

```sh
go build -o /tmp/h3-context-ir-go-live-eval ./cmd/h3-context-ir-go-live-eval
/tmp/h3-context-ir-go-live-eval \
  -input conformance/release_acceptance_inputs.json \
  -assertions conformance/release_acceptance_assertions.json \
  -out /tmp/h3-context-ir-go-live-new-run \
  -base-url https://api.deepseek.com \
  -model deepseek-flash \
  -api-key-env DEEPSEEK_API_KEY \
  -parallel 2 -profile official \
  -reasoning-effort low -max-output-tokens 12288 \
  -max-structure-retries 1 -max-planner-retries 1
```

The output directory must not exist. Assertions are a separate file and never
enter model inputs. Every case executes independently without input deduplication.
Cases default to `OfficialProfile`, explicit `snap_up`, `fail`, and
`supplied_content_wins`. A case may explicitly select `profile: "comfy"` to
preserve an original test's `ComfyUIProfile`. Structural and planner repairs each default to one;
`-max-structure-retries` and `-max-planner-retries` explicitly select zero through
three extra attempts. Effective limits are stored in the overall report and each
case's compiler options. A case may explicitly override both limits with
`"retry_limits": {"structure": 1, "planner": 1}`; both fields are required and
accept zero through three. Missing overrides preserve the CLI budgets. These
profile and budget selections are harness options, never model instructions.
Transport failures and invalid usage remain immediate
failures; increasing a content-repair budget does not enable HTTP retries.
Clarification defaults to
`require`; a fixture can explicitly choose `continue_with_recorded_choice`.
These effective options are recorded per case. Still-image perception uses the
same recorded model, real complete image bytes, `Detail: original`, structured
cards, and one content repair. Supplied cards/evidence require a declared
observation scope. This command supplies no audio or video perception adapter.

Every successful operation runs Compile or Refine, persists the authoritative
Document through Encode/Decode, rebuilds and compares the complete H3 Artifact,
and reloads the complete Result JSON before the next Refine. A failed or
unpublished operation stops subsequent changes in that case. Other cases continue.
Only a compiled, fully metered, successfully persisted operation passes by default.
A separate assertion may explicitly expect `needs_input` or an exact typed error;
that records a tested rejection, not a successful publication.
Assertions can use either `error_code` or a nonempty `error_codes` array of
distinct codes. The actual status and typed error must match; unrelated network
or perception failures never satisfy a family containing only input/model-output
contract errors. Both assertion forms remain isolated from model inputs.

Each operation keeps `result.json`, `operation.json`, its actual model requests,
accepted JSON bytes, complete response metadata, rejected candidates, actual image
inputs and their image/fixture hashes. `calls.json` updates after each invocation;
the overall `report.json` updates after each operation. Report/model files use
0600 permissions. No HTTP authorization header or credential is serialized.
The report records binary and fixture digests. The caller is responsible for
building from and retaining a verified immutable source snapshot.

The independent Go checks support JSON-pointer equality, string containment,
array length and exact prompt substring counts. Release-specific cross-field and
semantic checks remain separate and must inspect final Document/Artifact files.
Passing a model response schema or these deterministic checks alone does not
establish prose quality, full media understanding or generated video quality.
