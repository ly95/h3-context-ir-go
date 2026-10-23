# Contributing

H3 Context-IR Go is an independent Go library. Read [AGENTS.md](AGENTS.md) and the
[architecture](docs/ARCHITECTURE.md) before changing its contracts.

## Local checks

Use Go 1.27.3 or newer. Install ffmpeg and ffprobe to run the real-media integration
tests; their absence produces explicit skips and is not a complete media check.
No Python environment, provider credentials or paid model calls are required.

```sh
make check
```

This runs formatting, vet, all native tests, race tests and decoder fuzzing. A C
compiler is needed for the race detector. `make test` / `make test-race` refuse
to run without the media tools; direct `go test ./...` may instead skip those
integration cases. Format changed Go files with `gofmt`. CI uses the Go version
declared in `go.mod`. Keep ordinary tests deterministic and offline; use loopback servers,
explicit dependency injection and fixed public or synthetic fixtures.

## Contract and behavior changes

The versioned `Document` is authoritative. Prompt text, media wiring, hashes and
diagnostics must derive from that same document. Update schemas, wire contracts,
version checks and public documentation when a contract changes.

Interpret intent through explicit typed input or a model's structured contract.
Preserve caller choices, exact dialogue, source evidence and observable errors.
Keep retries bounded and metered. Do not add hidden fallback, truncation, model
substitution, or an implicit environment dependency.

When porting an upstream test, preserve its business invariant, successful
scenario and relevant failure controls through public Go interfaces. Record the
source and any explicit API/input adaptation; see [upstream tests](docs/UPSTREAM_TESTS.md).
A static inventory, a component differential and a complete workflow test support
different claims.

## Fixtures and recorded model calls

Keep provider-safe inputs separate from expected assertions. A replay must match
the full typed request before consuming a response and must retain failed
attempts and usage. Replay is not a fresh model evaluation.

Do not commit API keys, private media locators, application user content, local
absolute paths or generated evaluation reports. Use public or synthetic media,
record provenance and retain the original license and notices. Running a live
recorder is an explicit, separately configured operation that may incur charges.

## Licensing

Contributions to H3 Context-IR Go are distributed under [Apache-2.0](LICENSE). Preserve
third-party notices and document new dependencies or copied assets in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Model weights and third-party
services retain their own licenses and terms.
