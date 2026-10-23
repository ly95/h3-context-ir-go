# Native Go checks

The GitHub Actions workflow uses the Go version declared in `go.mod`. It checks
formatting, vets every package, runs the complete native test suite and race
detector, and fuzzes document and evidence decoding.

CI installs `ffmpeg`, which supplies both `ffmpeg` and `ffprobe`. The tests exercise
actual image, audio, and video operations with these tools. `make test` and
`make test-race` fail before testing if either executable is missing, so a passing
complete-suite command cannot silently omit these media prerequisites.

Run the same checks locally with the toolchain declared in `go.mod`, a C compiler
for the race detector, and `ffmpeg`/`ffprobe` on `PATH`:

```sh
make check
```

Individual targets are available through `make help`. Native tests use the
checked-in Go fixtures and injected or local test doubles; they require no
provider credentials, upstream checkout, historical reports, or live model
requests. The workflow does not install or invoke a Python test harness.

Upstream provenance and behavioral adaptations are documented in
[conformance](../conformance/README.md). Historical compatibility
diagnostics are distinct from the native Go acceptance suite.
