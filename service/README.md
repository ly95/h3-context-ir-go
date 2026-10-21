# Optional HTTP service

`New(Config)` creates a stdlib `http.Handler` around explicit upload, compile,
refine and record-storage capabilities. It opens no listeners and reads no
environment. `CompileFunc` receives the prepared `plan.Brief`; a typical adapter
uses `compiler.FromOriginal` followed by an initialized compiler's `Compile`.
`RefineFunc` can use that compiler's `Refine` directly. Seed, effort and caller
transcripts reach these adapters as typed values, so unsupported execution
options can be refused at the configured provider boundary.

Routes are `PUT /v1/assets/{sha256}`, `POST /v1/briefs`,
`GET/PATCH /v1/briefs/{id}`, `GET /v1/briefs/{id}/prompt`, and read-only
`/v1/contract`, `/v1/capabilities`, `/v1/directors`, `/v1/directors/{id}` and
`/v1/loras`. Uploaded bytes have no HTTP read route. `/health` requires an explicit
readiness probe and returns 503 until one reports readiness. `MaxRequestBytes`
bounds JSON requests; the upload store separately enforces streamed file and
aggregate limits. Unknown fields, malformed JSON, duplicate fields, unheld
uploads, conflicting source locators and unknown roles receive structured
refusals. Local paths are opened only with `AllowPaths: true`.

`PrepareBrief` exposes the same request conversion without HTTP. `ParseBrief`
performs the wire-schema part independently. Neither interprets user intent.
`Drain` bounds refused-body consumption and distinguishes peer disconnects.
`Error` preserves a safe status/code/message through injected compile adapters.

The default `MemoryRecords` belongs to one service instance, deep-copies stored
values and atomically increments versions. Inject `RecordStore` for persistence.
A compile/refine result must contain a valid native `Document` whose recomputed
artifact hash matches the returned prompt and wiring before it is published.
Responses contain presentation, reviewable plan and versioned native IR layers;
the IR carries the authoritative document and full H3 artifact provenance.
Refinement stores the amended seed, effort, mode and caller transcripts with the
result. Stable native asset IDs map transcripts back to original upload digests
and keep repeated uses of one file distinct; removing an earlier attachment does
not reassign the remaining source's notes or pairing. Refinement cannot introduce
an unattached source or rewrite a stored source locator/hash.
Before compilation/refinement starts, service-owned uploads must still exist.
An evicted dependency returns `422 asset-not-uploaded` with the missing digests
and reupload instructions before a model call. Active requests refresh those
uploads through the store's TTL/LRU `Touch` mechanism. If a source disappears
during the operation, the service cancels it and preserves any partial call
ledger instead of publishing an artifact with a missing source.

Typed compiler clarification questions produce `status: needs_input`, the actual
question/options and the default if unanswered. No document or renderable prompt
is published for an unresolved mechanism choice. Submit the original brief again
with its offered `answer.anchor_or_reference`; both `opening_frame` and
`appears_in` become explicit image roles before another compile. Unknown answer
IDs or values are refused. Pending records can be read; prompt retrieval and
refinement explain that the clarification must be answered first.
An injected compiler may explicitly select `continue_with_recorded_choice`;
that policy and the chosen mode must be recorded, and the unresolved question
remains visible beside the ready artifact. The service never enables it itself.

The compiler's call ledger is authoritative. Success and pending responses retain
it with `usage_complete` and `unmetered_asset_ids` in IR provenance. A compile or
refine failure carries the same fields beside its structured error; a failed
attempt's reported usage is never replaced with zero or counted a second time.

The request contract, all eight section digests, and the JSON/JS exports share
one native versioned definition. Run `go test ./service ./cmd/contextir` to verify
request/HTTP/refusal behavior, a full create/read/refine lifecycle, publication
integrity, concurrent record versions and actual CLI exports. Native contract v3
includes additional fields and refusal codes, with explicit version/digest
checks; it does not reuse upstream Python v2's digest. Source mappings are in
[upstream tests](../docs/UPSTREAM_TESTS.md).

One deliberate request-default difference: omitted `seed` remains unspecified;
it is not silently replaced with the source implementation's default 7. An
explicit seed, including 7, is preserved and requires actual provider support.
The response IR uses ContextIR's native version and structural manifest rather
than pretending its Go document is the original Python dataclass.
