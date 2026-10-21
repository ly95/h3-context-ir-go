# Original upstream assertion job

`.github/workflows/ci.yml` has an `upstream-conformance` job for pushes, pull requests and manual reruns. It checks out the unmodified upstream repository at `fd031e136ae3d89147324c0d1b2aa65e838c21f5`, installs `conformance/requirements.lock` in a separate Python virtual environment, and builds this repository's native Go bridge and media test executable.

The job runs two complete collections of the same 991 original tests:

1. The original Python implementation, with the observational `pytest_capture` plugin.
2. `pytest_go_combined`, which replaces the implemented collaborators with calls to the compiled Go bridge and replays archived model outputs through the current native contracts.

Both must pass all 991 tests, collect the exact original node IDs, and have no skips. Native assertion coverage must include the set in the immutable report referenced by `conformance/reports/combined_latest.json`. Tests that retain Python collaborators are reported separately; 991 passing original tests does not mean every production stage was implemented in Go. Incompatible archived model outputs fail the current contract and the job; the job does not discard their fields, skip their cases or replace them with fabricated answers.

`ffmpeg` and `ffprobe` are installed and checked. The native media integration tests execute against these actual tools. Any skipped media test fails this job, even if the test executable otherwise exits successfully.

Test execution takes place in a Linux network namespace containing only an enabled loopback interface, after dependencies have been installed. The test user is unprivileged. The child environment is built from an allowlist and has a temporary empty home, no provider credentials or proxy configuration, and no automatic pytest plugin loading. The Python transport guard also rejects external network access. Local HTTP mocks, callbacks and test servers remain usable on loopback. There are no live provider requests in this job.

Every attempt uploads its environment/tool versions, bridge hash, native media events, complete pytest reports and stdout/stderr, and `summary.json`, including failures. Output directories cannot be overwritten. The runner verifies the upstream working tree is clean before and after execution, including nonignored untracked files.

For a local diagnostic run, use the same pinned checkout and virtual environment:

```sh
python3 -m venv /tmp/contextir-ci-python
/tmp/contextir-ci-python/bin/python -m pip install -r conformance/requirements.lock
go build -o /tmp/contextir-go-bridge ./conformance/bridge
go test -c -o /tmp/contextir-media.test ./media
/tmp/contextir-ci-python/bin/python ci/run_upstream.py \
  --source /absolute/path/to/clean-pinned-upstream \
  --binary /tmp/contextir-go-bridge \
  --media-binary /tmp/contextir-media.test \
  --output /tmp/contextir-conformance-unique-attempt \
  --allow-host-network
```

`--allow-host-network` is a local diagnostic option. It keeps the clean environment and Python network guard but does not isolate the Go process's network. The workflow never passes it. Install working `ffmpeg`/`ffprobe` first, and use a fresh output path for every attempt.

The workflow is configured for reruns. This configuration is not evidence of a successful hosted run: the account's existing GitHub Actions billing restriction has not been changed or bypassed, and hosted execution has not been verified. Local results and hosted results must be reported separately.
