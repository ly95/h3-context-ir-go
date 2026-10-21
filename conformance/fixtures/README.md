# Upstream fixture corpus

These JSON files preserve inputs, expected outputs and source node IDs from
`ruashots/open-h3-ir` at `fd031e136ae3d89147324c0d1b2aa65e838c21f5`.
The native Go tests read them directly; no Python interpreter is required.
The upstream Apache-2.0 license and attribution are retained in
[`third_party/open-h3-ir`](../../third_party/open-h3-ir).

Inputs and held-out expectations have different purposes. Do not send oracle
files, expected output fields or assertion inventories to a model. The Go
evaluation commands require explicitly selected model-safe input files and
write new results to a caller-selected directory.

For publication, five host-specific temporary asset paths in `compiler_calls.json`
and `semantic_cases.json` were replaced with stable synthetic paths under
`/tmp/contextir-fixtures/geometry/`. File names, image dimensions, source
assertions and expected results were preserved. Each file records its original
SHA-256 and the exact changed JSON pointers in `publication_normalization`.
The affected semantic input has a new `input_sha256`; its original digest is
retained separately as `original_input_sha256`.

Historical resolution and oracle records keep their original input digests;
associate those records through `original_input_sha256`, not the normalized
input digest. They are captured corpus results, not permission to replay a
provider response against a changed request. Strict request-matching tapes are
kept separately in `compiler/testdata` and retain their original request bytes.

Those paths identify captured test fixtures, not files readers must create on
their machines. Tests that need actual media create temporary assets themselves
or use the checked-in assets in `third_party/open-h3-ir/testdata`.
