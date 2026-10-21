# Audit evidence and release data

The complete audit, including failed historical runs, model requests/responses,
source snapshots and reproduction tools, is preserved at
[`b8f24802ac310089eabf7918d5beeb5721d58d62`](https://github.com/ly95/contextir/tree/b8f24802ac310089eabf7918d5beeb5721d58d62)
and the `codex/conformance-evidence` branch. The latest release proof is indexed
in [release-index.json](https://github.com/ly95/contextir/blob/main/conformance/reports/release-index.json).

This release tree retains the four baseline files and eight complete recording
directories needed by the existing CI, byte-for-byte: 2,262 files / 37,030,618 bytes.
The [keep-set manifest](release-thin-ci-keep-set-v1.json) records every file hash.
No baseline, source assertion, recording response or CI gate was changed.
The nested `go.mod` excludes this audit data from the library module ZIP.

For the complete audit, use a separate checkout:

```sh
git clone --single-branch --branch codex/conformance-evidence https://github.com/ly95/contextir.git /tmp/contextir-audit
```

Use the full immutable commit in the release index to reproduce that audit
snapshot. Ordinary Go library tests use their self-contained testdata and do not
need the full audit checkout.
