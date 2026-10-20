# Audit archive boundary

This directory contains immutable test histories, public-input model traces,
coverage reports and reproducible source snapshots. It has no Go source and is
not an importable package.

Its separate `go.mod` keeps the audit archive out of the parent library's module
ZIP. Go callers download the library and native fixtures without hundreds of
megabytes of historical reports. Clone the private repository to inspect and
replay the full evidence. Nothing is removed from Git history or the repository.

`conformance/README.md` and `docs/UPSTREAM_TESTS.md` identify the latest results and
the exact boundaries of each run. A larger archive or a passing Python baseline
does not imply complete native Go equivalence.
