# Akuaku architecture

A reference for how Akuaku is built and why. For where it is going, see
[ROADMAP.md](ROADMAP.md).

## Principles

- **Drive the CLIs, not an API.** Akuaku runs `claude`, `codex`, and `ollama` as
  subprocesses — the same binaries you already log into. It never holds an API
  key or talks to a hosted backend.
- **Open by contract.** The state directory is the only channel between the parts
  of Akuaku. Any process that writes a conforming JSON file appears in the
  monitor — no code change required. This is the extension point.
- **The monitor only reads.** It never writes state; it derives every number by
  scanning the directory on a tick. Producers write; the monitor reads.
- **Never reads transcripts.** Akuaku records only what a tool or the OS exposes
  (command, PID, directory, times, and a launched run's own output). It does not
  open Claude Code transcript files.
- **100% test coverage** on `internal/...`, enforced in CI. Side effects live
  behind seams so logic stays pure and testable; `cmd/akuaku` is the only
  untested glue.

## The state contract

One run is one JSON file at `<state-dir>/<id>.json`. The state directory is
`~/.akuaku/state` by default, overridable with `AKUAKU_STATE_DIR`. Writes are
atomic (temp file + rename) so a reader never sees a half-written file.

The `state.Run` schema (`internal/state/run.go`):

| Field | Meaning |
| --- | --- |
| `id` | unique run id; the file name |
| `backend` | `claude` \| `codex` \| `ollama` |
| `name` | display label (task, or an explicit name) |
| `status` | `running` → `waiting` ⇄ `running` → `done` \| `error` |
| `task` | the prompt / first user input |
| `model` | model name, when known |
| `source` | producer: empty (`akuaku run`), `hook`, or `process` |
| `pid` | process id, when Akuaku can signal it |
| `dir` | the run's working directory, for scope |
| `started_at` | creation time |
| `last_message_at` | last activity, for the history view |
| `ended_at`, `exit_code`, `error` | terminal outcome |
| `tokens`, `cost` | usage, best-effort |
| `output` | the launched run's captured answer |

A monitor-owned overlay, `names.json`, holds custom `:rename` labels so a run's
own file stays producer-owned.

## Parts

Akuaku is one binary, `cmd/akuaku`, dispatching to internal packages:

| Package | Role |
| --- | --- |
| `state` | the run schema, atomic read/write, the names overlay |
| `backend` | registry: each backend supplies a command builder and an output parser |
| `launcher` | `akuaku run` — spawns a backend, records lifecycle, prints the answer |
| `hook` | reflects Claude Code sessions from their lifecycle hooks; installs them |
| `discover` | maps a process-table snapshot to running runs (argv-based) |
| `demo` | a deterministic, looping simulated fleet for `akuaku demo` |
| `tui` | the monitor — a Bubble Tea model that only reads state |
| `cli` | argument parsing and dispatch (fully testable; side effects injected) |
| `setup`, `updater` | PATH setup / backend check, and self-update |
| `brand` | the tiki mask, wordmark, and accent — one visual identity |

### Producers

- **`akuaku run <backend> "<task>"`** spawns the backend subprocess, records
  `running` (with the PID and working directory), then the terminal state with
  tokens, cost, and the captured answer.
- **Claude Code hooks** (`akuaku hook <event>`, installed by `akuaku hook
  install`) reflect external sessions: `SessionStart` → `running`,
  `UserPromptSubmit` → sets the task and returns to `running`,
  `Notification`/`Stop` → `waiting`, `SessionEnd` → `done`. Every event updates
  `last_message_at`. Reflected runs show `—` for usage.
- **Discovery** scans the process table each tick and surfaces any running agent
  CLI, identifying it by the base name of `argv[0]` (so Claude Code's
  version-stamped executable is still recognized as `claude`, and the desktop app
  and `ollama serve` are excluded). Discovered runs are merged into the view,
  deduped by PID; they are ephemeral and vanish when the process exits.

### The monitor

A Bubble Tea `Model` that reads `state/*.json` on a one-second tick and animates
a spinner on a faster tick. Views and keys:

- **Dashboard** (default) — running and waiting agents, running first,
  status-colored, over a k9s-style overview strip (counts, total tokens/cost).
- **History** (`h`) — every run, most-recent first, with created and
  last-message dates.
- **Detail** (`Enter`) — the selected run framed as a conversation.
- **Scope** — local by default (runs in the launch directory or below);
  `:global` / `:local` toggle; shown in the footer.
- **Attention** — a run entering `done`, `error`, or `waiting` rings the terminal
  bell and shows a banner, so the monitor is worth leaving open.
- **Keys** — `↑/↓` move, `Enter` open, `k` kill (with `y`/`n` confirm), `/`
  filter (`-n`/`-m` scoped), `:` command (`:rename`, `:kill`, `:discovery`,
  `:local`, `:global`), `a` all, `h` history, `q` quit.

## Backends

Each backend is a command builder plus a best-effort parser. Adding one is data
and a parser, not a rewrite.

| Backend | Command | Tokens | Cost |
| --- | --- | --- | --- |
| `claude` | `claude -p … --output-format json` | ✅ | ✅ `total_cost_usd` |
| `codex` | `codex exec --json` | ✅ | — |
| `ollama` | `ollama run <model> … --verbose` | ✅ | — (local) |

## Testing

`make check` runs fmt, vet, `golangci-lint`, and the race test suite. `internal/`
packages hold at 100% statement coverage; CI enforces it. Determinism comes from
injected seams — the clock, randomness, the command runner, the process source,
the bell, `getwd` — so tests never touch the real clock, network, or terminal.

## Historical specs

The `openspec/` directory holds the OpenSpec change proposals for the first two
milestones (`mvp-core`, `reflect-external-sessions`). This document is the
current, consolidated reference; the roadmap tracks what is next.
