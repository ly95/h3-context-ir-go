# Exact H3 prompt tokenizer

`Count(text)` and `Encode(text)` use the actual Qwen2.5 byte BPE vocabulary that upstream verified byte-identical to H3's conditioning encoder. The Go implementation uses only the standard library: Unicode pre-tokenization, byte-to-Unicode vocabulary decoding, rank-ordered BPE merges, and all 26 added tokens. It performs no network, filesystem-path lookup, or external process calls at runtime.

The 2,776,833-byte vocabulary is embedded in the binary and validated against Git blob SHA-1 `4783fe10ac3adce15ac8f358ef5462739852c569` before use. It increases source and binary size deliberately to make exact offline counting reproducible. Attribution and Apache-2.0 license are in `third_party/qwen25-tokenizer`.

`<d>`, `</d>`, `<scenetrans>`, and `<cutoff>` are ordinary BPE input, not added tokens. In the original tokenizer `<d>` has 2 tokens and `</d>` has 3. Invalid UTF-8 or a broken vocabulary returns an error; there is no approximate fallback.

Tests replay the 39 actual calls to the original `h3ir.tokens.count`. They also compare every token ID for 386 original validator inputs, nine original golden prompts, and nine explicitly identified supplementary Unicode boundary cases (404 sequences). This validates tokenization, not intent understanding or video quality.
