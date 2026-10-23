GO ?= go
GOFMT ?= gofmt
FUZZTIME ?= 10s

.PHONY: help demo check check-format fmt media-tools vet test test-race fuzz

help:
	@printf '%s\n' \
	  'make demo          Show the library example flags without a model call' \
	  'make check         Run formatting, vet, all tests, race tests, and decoder fuzzing' \
	  'make test          Run all native tests, including actual media integrations' \
	  'make test-race     Run all native tests with the race detector' \
	  'make vet           Vet every Go package' \
	  'make check-format  Report Go files that need formatting' \
	  'make fmt           Format Go source files' \
	  'make fuzz          Fuzz document and evidence decoding (FUZZTIME=10s each)' \
	  'Install ffmpeg and ffprobe before running test, test-race, or check.'

demo:
	@$(GO) run ./examples/manual -h

check: check-format vet test test-race fuzz

check-format:
	@files="$$($(GOFMT) -l .)" || exit $$?; \
	if [ -n "$$files" ]; then \
	  printf '%s\n' "$$files"; \
	  printf '%s\n' 'Run make fmt to format these files.' >&2; \
	  exit 1; \
	fi

fmt:
	$(GOFMT) -w .

media-tools:
	@command -v ffmpeg >/dev/null 2>&1 || { printf '%s\n' 'ffmpeg is required for the complete native test suite.' >&2; exit 1; }
	@command -v ffprobe >/dev/null 2>&1 || { printf '%s\n' 'ffprobe is required for the complete native test suite.' >&2; exit 1; }

vet:
	$(GO) vet ./...

test: media-tools
	$(GO) test -count=1 ./...

test-race: media-tools
	$(GO) test -race -count=1 ./...

fuzz:
	$(GO) test -run='^$$' -fuzz='^FuzzDecode$$' -fuzztime=$(FUZZTIME) .
	$(GO) test -run='^$$' -fuzz='^FuzzDecodeEvidence$$' -fuzztime=$(FUZZTIME) .
