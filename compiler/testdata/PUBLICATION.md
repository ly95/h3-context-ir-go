# Published replay fixtures

The recordings in this directory are retained for deterministic, offline Go
regression tests. Model requests, responses, usage, errors, expected documents
and artifacts retain their original bytes and digests.

Two build receipts include publication-only redactions:

- `remaining_highlevel/source_build_receipt.json`
- `remaining_wardrobe_fresh/source_build_receipt.json`

Only the temporary host paths in `command[5]`, `build_directory` and `binary`
are replaced with `<redacted-local-build-path>`. Each receipt identifies itself
as a published derivative and records the original receipt SHA-256 and exact
redacted fields. Source and executable digests are unchanged. This redaction
does not represent a new build or a new model run.

Each enclosing manifest records its original digest and the receipt's updated
file digest. The Go tests independently pin the published manifest and verify
the entire file inventory, exact stage/system/input/schema of every replayed
request, and the resulting document and artifact. No replay checks are removed
or relaxed for publication. Original unredacted receipts remain in the private
development archive.
