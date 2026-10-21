# Unmodified upstream test assets

Source: https://github.com/ruashots/open-h3-ir

Commit: `fd031e136ae3d89147324c0d1b2aa65e838c21f5`.

`golden/` is a byte-for-byte copy of `h3ir/golden/` at that commit: nine text
fixtures and the two PNG reference assets used by the original controls/eval suite.
No golden output or mutation expectation has been weakened for the Go implementation.

Copyright upstream contributors. Licensed under Apache-2.0; see the parent
[LICENSE](../LICENSE) and [NOTICE](../NOTICE). Exact original paths, SHA-256 and
immutable source URLs are in [source_manifest.json](../../../conformance/source_manifest.json).

`prompts/base-en.txt` is the unmodified published prompt specification from the
same commit. Its full SHA-256 and the original example's SHA-256 are pinned by
`h3/TestRemainingSurfacePublishedFL2VASpecBytes`; the test compares actual bytes.

`surface_manifest.json` records the independently extracted original 16-row
`ROLE_CASES` and all three real-asset geometry declarations (including repeated
filenames). It carries the immutable source hashes; native tests also pin the
manifest hash. These are test inputs, not a generated expected native result.
