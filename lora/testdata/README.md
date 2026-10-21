# LoRA registry test fixtures

`handpainted-anim-v2/` is a synthetic registry example from the pinned
[open-h3-ir source](https://github.com/ruashots/open-h3-ir/tree/fd031e136ae3d89147324c0d1b2aa65e838c21f5).
The original fixture bytes are preserved for the native Go parser and registry
tests.

`handpainted_anim_v2.safetensors` is a 79-byte text placeholder. It contains no
model weights and is not a usable safetensors model. The all-zero checksum and
`license: "personal use only"` field in `howtouse.md` are test inputs for parsing
registry metadata; they do not define the license of ContextIR or supply a
license for any real model.

See the repository's [LICENSE](../../LICENSE), [NOTICE](../../NOTICE), and
[upstream notices](../../third_party/open-h3-ir/NOTICE) for the software's
licensing and provenance. Applications that load real LoRA files must use the
license and checksum supplied with those files.
