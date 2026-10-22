# Third-party notices

H3 Context-IR Go is distributed under [Apache License 2.0](LICENSE). The following
materials retain their original attribution and applicable licenses. The root
[NOTICE](NOTICE) summarizes adaptations made by this project; it does not replace
the original notices.

## OpenH3-IR

- Source: [ruashots/open-h3-ir](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5).
- Fixed revision: `fd031e136ae3d89147324c0d1b2aa65e838c21f5`.
- Copyright: 2026 Ruashots and upstream contributors.
- License: [Apache-2.0](third_party/open-h3-ir/LICENSE).
- Original attribution: [NOTICE](third_party/open-h3-ir/NOTICE).

H3 Context-IR Go adapts its public planning, geometry/budget, tokenization, H3 rendering
and validation, evaluation, media/LoRA, upload, HTTP and ComfyUI contracts into Go.
The Go implementation adds an authoritative versioned Document, typed model
contracts, explicit dependencies and policies, strict provenance, bounded repair
and usage accounting. Original function names and formats in fixtures identify
the tested source behavior; they do not imply execution of Python at runtime.

Derived tests and captured source input/output fixtures appear in
`conformance/fixtures`, `conformance/adaptations` and package testdata. Unmodified
golden prompts, published prompt specification text and reference PNGs are in
`third_party/open-h3-ir/testdata`. Additional source media used by native intake
tests is in `compiler/testdata/remaining_media`; its manifest identifies exact
upstream paths and hashes. The [source manifest](conformance/source_manifest.json)
records immutable upstream URLs and digests. Explicit publication redactions,
where present, are described beside their fixture manifests.

`lora/testdata/handpainted-anim-v2` retains the upstream registry example,
guidance and tiny placeholder `.safetensors` file for registry tests. It is not
a trained or usable LoRA model. The same upstream license and notices apply.

The upstream NOTICE also records attribution for MIT-licensed ComfyUI browser
widgets. Those browser widget sources are not bundled in H3 Context-IR Go; their
original notice is preserved unchanged as part of the upstream attribution.

## Qwen2.5 tokenizer vocabulary

- Copyright: Alibaba Cloud and Qwen contributors.
- License: [Apache-2.0](third_party/qwen25-tokenizer/LICENSE).
- Provenance: [NOTICE](third_party/qwen25-tokenizer/NOTICE).
- Included file: `tokens/data/vocab.json`, embedded for exact offline BPE counting.

The vocabulary was obtained from the pinned OpenH3-IR source. Its original Git
blob SHA-1 is `4783fe10ac3adce15ac8f358ef5462739852c569`. Upstream identifies it as
byte-identical to the vocabulary used by H3's conditioning encoder; this is
provenance for the checked-in bytes, not a promise about future model revisions.

## MiniMax H3 format and model

Protocol spelling and examples follow MiniMax's publicly published H3 prompt
guides, as identified in [UPSTREAM.md](docs/UPSTREAM.md). The retained upstream
NOTICE identifies quoted MiniMax examples in its golden prompt fixtures.

H3 Context-IR Go includes no MiniMax model weights or hosted Context-IR implementation.
The library's Apache-2.0 license does not grant a license to MiniMax H3 model
weights or a right to use a hosted service. Those resources and any generated or
caller-provided media remain subject to their own applicable terms.

## External tools and services

The optional `openai` and `chatcompat` adapters depend on the official
[OpenAI Go SDK v3.73.0](https://github.com/openai/openai-go/tree/v3.73.0),
distributed under [Apache-2.0](https://github.com/openai/openai-go/blob/v3.73.0/LICENSE).
Its source is resolved through Go modules, with versions and integrity checksums
recorded in `go.mod` and `go.sum`; it is not vendored into this repository.
The SDK and its transitive dependencies retain their own licenses and notices.

ffmpeg, ffprobe, ComfyUI and model services are optional, separately installed or
configured integrations. Their executables, runtime implementations and model
weights are not distributed by this Go module. Their own licenses and service
terms apply when used.
