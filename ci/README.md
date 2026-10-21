# Native Go checks

The GitHub Actions workflow runs the complete native Go test suite on the exact
minimum toolchain, Go 1.25.0, and the current stable Go release. Every package is
also vetted on both versions. The stable-version job checks formatting, runs the
complete suite with the race detector, and fuzzes document and evidence decoding.

CI installs `ffmpeg`, which supplies both `ffmpeg` and `ffprobe`. The tests exercise
actual image, audio, and video operations with these tools. `make test` and
`make test-race` fail before testing if either executable is missing, so a passing
complete-suite command cannot silently omit these media prerequisites.

Run the same checks locally with Go 1.25 or newer, a C compiler for the race
detector, and `ffmpeg`/`ffprobe` on `PATH`:

```sh
make check
```

Individual targets are available through `make help`. Native tests use the
checked-in Go fixtures and injected or local test doubles; they require no
provider credentials, upstream checkout, historical reports, or live model
requests. The workflow does not install or invoke a Python test harness.

Upstream provenance and behavioral adaptations are documented in
[`docs/UPSTREAM_TESTS.md`](../docs/UPSTREAM_TESTS.md). Historical compatibility
diagnostics are distinct from the native Go acceptance suite.
