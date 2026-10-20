# ComfyUI adapter

`Decode`, `FindH3Nodes`, `SetPrompt`, `SetSeed`, `DescribeGraph` and `Outputs` inspect or copy an API graph without changing the caller's graph. Seeds retain their full unsigned 64-bit value. Ambiguous H3 conditioning nodes require an explicit node ID.

`New(Config)` supplies an explicit remote endpoint, HTTP transport, timeout and response byte limit. The client streams image uploads and supports queue submission, history polling, output download, stats and installed H3 node discovery. It reads no environment or local ComfyUI directories, follows no redirects and never silently switches endpoints. `Wait` has explicit polling and elapsed-time limits and respects cancellation.

Nine captured graph helper invocations from the pinned Apache-2.0 open-h3-ir suite are compared with the Go implementation. Additional local HTTP tests exercise uploads, polling, error responses, cancellation, limits and download bytes; these are supplementary tests, not upstream HTTP coverage.
