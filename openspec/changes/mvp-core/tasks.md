<!--
Build order follows the design: start with a living TUI window for fast feedback,
then build the state contract (foundation), the backend registry, the launcher,
complete the monitor against real state, and finish with wire-up, docs, and
verification. Every implementation group ends with tests; the repo enforces 100%
coverage. All code, comments, and commits are in English.
-->

## 1. Scaffolding & tooling

- [ ] 1.1 Initialize the Go module `github.com/akuaku-ai/akuaku` (recreate `go.mod`) and commit it
- [ ] 1.2 Add Charm dependencies: `bubbletea`, `lipgloss`, `bubbles`
- [ ] 1.3 Establish the package layout: `cmd/akuaku`, `internal/state`, `internal/backend`, `internal/launcher`, `internal/tui`
- [ ] 1.4 Add a Makefile and CI running `build`, `go vet`, lint, and `go test -race -coverprofile`, with a gate that fails below 100% coverage
- [ ] 1.5 Add `golangci-lint` config and a `gofmt`/`goimports` check to CI

## 2. TUI living window (agent-monitor spike)

- [ ] 2.1 Implement a minimal Bubble Tea program (`Model`/`Init`/`Update`/`View`) that opens a live window and quits on `q`
- [ ] 2.2 Add a one-second tick command and render a placeholder frame that updates on each tick
- [ ] 2.3 Unit-test `Update` (tick and `q` handling) and a `View` snapshot for the placeholder frame

## 3. agent-state (foundation)

- [ ] 3.1 Define the `Run` type with JSON tags for `id`, `backend`, `name`, `status`, `task`, `model`, `started_at`, `ended_at`, `tokens`, `cost`, `exit_code`, `error`
- [ ] 3.2 Implement unique `id` generation (`<backend>-<timestamp>-<rand>`) with the clock and randomness injected for deterministic tests
- [ ] 3.3 Implement state-directory resolution: default `./state`, override via `AKUAKU_STATE_DIR`
- [ ] 3.4 Implement atomic write (write to `<id>.json.tmp`, then `os.Rename`)
- [ ] 3.5 Implement directory scan/read that parses runs and skips unparseable files without error
- [ ] 3.6 Unit-test JSON round-trip, id generation, directory resolution, atomic write, and skip-invalid behavior at 100% coverage

## 4. backend-registry

- [ ] 4.1 Define the `Backend` interface (`Key`, `BuildCommand(task, model)`, `ParseOutput`) and the registry with lookup + unknown-key error
- [ ] 4.2 Implement the `claude` backend: build `claude -p --output-format json` and parse tokens and cost from `usage`/`total_cost_usd`
- [ ] 4.3 Implement the `codex` backend: build `codex exec --json` and parse tokens from the usage event, cost `0`
- [ ] 4.4 Implement the `ollama` backend: build `ollama run [--verbose]`, cost `0`, best-effort tokens
- [ ] 4.5 Implement model selection: apply the requested model, fall back to the backend default when omitted
- [ ] 4.6 Test command builders (model override and default) and parsers against captured output fixtures, including the unparseable-degrades-to-zero case, at 100% coverage

## 5. agent-launcher

- [ ] 5.1 Implement the run lifecycle: write a `running` file, exec the backend subprocess, then atomically update the same run to `done`/`error`
- [ ] 5.2 On failure, capture `exit_code` and stderr into `error`; treat a missing/unrunnable CLI as an `error` run
- [ ] 5.3 Parse `--model` and record it in the run while passing it to the backend command
- [ ] 5.4 Wire the `akuaku run <backend> "<task>" [--model]` CLI; reject an unregistered backend before any file is written
- [ ] 5.5 Test success, non-zero exit, missing CLI, and invalid-backend paths using a fake backend, asserting the final state file at 100% coverage

## 6. agent-monitor (complete against real state)

- [ ] 6.1 On each tick, scan the state directory via `internal/state` and store the runs in the model
- [ ] 6.2 Render the agent table (name, backend, status, duration, tokens, cost) with Lipgloss, computing duration live while `running` and frozen once terminal (clock injected)
- [ ] 6.3 Render the derived metrics panel (running count, `runs_ok`, `runs_err`, total tokens, total cost)
- [ ] 6.4 Add arrow-key navigation/selection of runs
- [ ] 6.5 Add the `l` keybinding to launch `akuaku run` as a detached, non-blocking process
- [ ] 6.6 Test tick/key `Update` transitions, live-vs-frozen duration, and `View` snapshots with mock runs at 100% coverage

## 7. Wire-up, docs & verification

- [ ] 7.1 Implement `cmd/akuaku` dispatch: no args → monitor, `run` → launcher, plus `--help`
- [ ] 7.2 Verify end-to-end: `akuaku run claude "..."` then `akuaku` shows the run transitioning to `done`
- [ ] 7.3 Write the README (English): what/why, install, usage, the open JSON contract, and the roadmap
- [ ] 7.4 Confirm the 100% coverage gate is green and `go vet`/lint are clean in CI
- [ ] 7.5 Run `openspec validate mvp-core --strict` and resolve any issues
