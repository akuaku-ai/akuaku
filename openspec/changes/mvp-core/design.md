## Context

Akuaku is a greenfield Go project (module `github.com/akuaku-ai/akuaku`, Go 1.26) with no source code yet. It targets developers who run AI coding agents through CLIs they already log into (Claude Code, Codex, Ollama) and want a single, fast terminal view of what is running, its outcome, and its cost — without API keys or hosted backends.

Hard constraints:
- **No API/tokens.** Agents are driven only by subprocessing existing CLIs.
- **Open by contract.** The monitor must never be the only producer of data; any process must be able to surface a run.
- **Senior bar.** SOLID, clean code, 100% English, 100% test coverage, high performance.

The MVP core (this change) is deliberately small so the value loop — launch, observe, learn cost — ships in days. The proposal defines the four capabilities: `agent-state`, `backend-registry`, `agent-launcher`, `agent-monitor`.

## Goals / Non-Goals

**Goals:**
- A single `akuaku` binary with a read-only TUI monitor and an `akuaku run` launcher.
- A file-per-run JSON contract that fully decouples producers from the monitor.
- An extensible backend registry (claude, codex, ollama) where adding a backend is data plus a parser.
- Model selection at launch time via `--model`.
- Correctness and clarity provable by tests at 100% coverage.

**Non-Goals:**
- Reflecting sessions launched outside `akuaku run` (future `akuaku hook` / `akuaku watch`).
- Embedded interactive Claude REPL (PTY/streaming).
- Alerts, webhooks, connectors, multi-tenant/RBAC/SSO.
- State-file retention/cleanup policy.

## Decisions

### D1: Single binary, two modes (monitor + `run` subcommand)
`akuaku` with no args starts the TUI; `akuaku run <backend> "<task>"` launches. **Why:** one install, one mental model, and the two concerns still live in isolated packages. **Alternatives:** two separate binaries (more install friction, same code); an in-process TUI launcher that spawns and tracks children in memory (couples launching to a running TUI and loses runs when it closes — rejected, violates the open contract).

### D2: File-per-run JSON as the public interface, written atomically
Each run is one `state/<id>.json` file. Writers write to `<id>.json.tmp` then `os.Rename` into place. **Why:** the filesystem is the simplest possible decoupled bus; rename is atomic on the same filesystem, so a reader never sees half-written JSON. **Alternatives:** a single shared state file (write contention, whole-file rewrites), a database (heavy, closed, kills the "any process can write" property) — both rejected.

### D3: Aggregates derived in the monitor, no shared mutable state
`runs_ok`, `runs_err`, running count, and totals are computed by scanning state files on each tick. **Why:** the launcher and monitor never share memory or coordinate; the monitor is a pure function of the directory contents. **Alternative:** launcher maintains counters in a shared file (read-modify-write races, another write path to get wrong) — rejected.

### D4: Extensible backend registry behind a small interface
A backend is defined by (key, command builder, output parser). The registry maps a key to its definition; the launcher depends only on the interface. **Why:** SOLID open/closed — adding a backend touches only its own definition, never the launcher or monitor. **Alternative:** a `switch backend` in the launcher (every new backend edits shared code) — rejected.

### D5: State directory default `./state`, override `AKUAKU_STATE_DIR`
**Why:** `./state` matches the repo dev loop and the original design; the env var makes both modes agree from any working directory. **Alternative:** a fixed `~/.akuaku/state` (better for real use, but diverges from the dev loop now) — deferred; the env var already unlocks it later without a code change.

### D6: Status enum `running | done | error` (drop `idle`)
A synchronous run is born `running` and transitions once to `done` or `error`. **Why:** `idle` has no producer in the MVP. **Alternative:** keep `idle` reserved (dead enum value, untestable) — rejected; reintroduce when a producer exists.

### D7: Best-effort tokens/cost per backend
claude (`-p --output-format json`) yields tokens and cost; codex (`exec --json` JSONL) yields tokens, cost 0; ollama is local (cost 0, tokens best-effort). Parse failures degrade to zeros without failing the run. **Why:** the three CLIs genuinely differ; the contract must not pretend uniformity. Parsers are isolated and fixture-tested so drift is contained.

### D8: Launch-from-TUI is a detached exec of `akuaku run`
The `l` key spawns `akuaku run` as a detached process and returns immediately. **Why:** keeps the TUI responsive and reuses the exact launch path, so there is one lifecycle-writing code path. **Alternative:** launch in-process (see D1) — rejected.

## Risks / Trade-offs

- **CLI output format drift breaks a parser** → isolate each parser behind the backend interface, cover it with fixtures captured from real output, and degrade to zero tokens/cost rather than failing the run (D7).
- **Reader observes a partial file** → atomic tmp+rename writes (D2); the monitor additionally skips any file it cannot parse and continues.
- **State files accumulate unbounded** → out of scope here; noted as a future `akuaku clean`. The monitor tolerates large directories via a cheap per-tick scan.
- **Detached child orphaning on launch** → the launcher owns the full lifecycle write even when detached; the TUI never tracks child PIDs, so a closed TUI cannot orphan tracking state.
- **100% coverage on TUI rendering** → keep `View` a pure function of model state and cover it with snapshot/table tests; isolate all I/O (filesystem, clock, subprocess) behind interfaces so units are deterministic.
