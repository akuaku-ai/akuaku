## Why

Developers increasingly run multiple AI coding agents (Claude Code, Codex, Ollama) across many terminals, but have no single, fast place to see what is running, whether it succeeded, and what it cost. Existing tools assume API keys and hosted backends. Akuaku instead drives the CLIs a developer already logs into via subprocess — no API, no tokens, no vendor lock-in. This change delivers the MVP core: a minimalist terminal UI to **monitor** agent runs and a command to **launch** them across three backends, built on an open, decoupled contract so any future source can plug in without changing the monitor.

## What Changes

- Introduce a single `akuaku` binary with two modes:
  - `akuaku` (no args) launches the TUI **monitor** that reads `state/*.json` and renders a live table plus a derived metrics panel, refreshing on a 1s tick. The monitor is strictly read-only.
  - `akuaku run <backend> "<task>" [--model <model>]` **launches** an agent: it spawns the backend CLI as a subprocess and writes the run's lifecycle to a state JSON file (`running` → `done`/`error`), capturing tokens and cost on a best-effort basis.
- Define the **state JSON contract** as the project's public interface: one file per run, atomically written, discoverable by any process. This is the extension point — anything that writes conforming JSON appears in the monitor.
- Provide an **extensible backend registry** covering `claude`, `codex`, and `ollama`, where each backend supplies a command builder (including model selection) and an output parser. Adding a backend is data plus a parser, not a rewrite.
- Derive aggregate metrics (`runs_ok`, `runs_err`, running count, total tokens, total cost) in the monitor by scanning state files — no shared mutable state between launcher and monitor.

Non-Goals (explicitly out of scope for this change; the architecture is designed to support them later without breaking the contract):

- Reflecting agent sessions started outside `akuaku run` in other terminals (future: an `akuaku hook` fed by Claude Code hooks, then process discovery for codex/ollama).
- An embedded interactive Claude REPL inside the TUI (PTY/streaming session).
- Alerts, webhooks, connectors, and any multi-tenant/RBAC/SSO features.

## Capabilities

### New Capabilities
- `agent-state`: the state JSON schema, file naming, storage location, and atomic-write contract that decouples every producer from the monitor.
- `backend-registry`: the extensible set of supported backends (claude, codex, ollama), each defining how to build its command (with model selection) and parse its output for tokens/cost.
- `agent-launcher`: the `akuaku run` command that spawns a backend subprocess and writes the run lifecycle to a state file.
- `agent-monitor`: the read-only TUI that scans state files on a tick and renders the agent table plus a derived metrics panel.

### Modified Capabilities
<!-- None. This is the greenfield MVP; no existing specs change. -->

## Impact

- **Code**: new Go packages under the `github.com/akuaku-ai/akuaku` module — a `cmd`/entrypoint plus internal packages for state, backends, launching, and the TUI. No source exists yet.
- **Dependencies**: Charm stack (Bubble Tea, Lipgloss, Bubbles) for the TUI; standard library for subprocess, JSON, and filesystem.
- **Runtime**: reads/writes a `state/` directory (default `./state`, overridable via `AKUAKU_STATE_DIR`). Requires the `claude`, `codex`, and `ollama` CLIs to be installed and logged in for launching; the monitor works with any state files regardless of source.
- **Tooling**: repository standards apply — 100% English artifacts, SOLID/clean code, conventional commits, and 100% test coverage enforced in CI.
