# Akuaku 🗿

A minimalist terminal UI to **monitor** and **launch** AI agents — using the CLIs you already log into, not API keys.

Akuaku subprocesses the tools you already have (Claude Code, Codex, Ollama) and shows every run in one live dashboard: status, duration, tokens, and cost. No API tokens, no hosted backend, no lock-in.

```
Akuaku 🗿

NAME          BACKEND  STATUS   DUR   TOKENS     COST
refactor auth claude   running  0:12    1200   $  0.04
fix tests     codex    done     1:30    3400   $  0.00
summarize     ollama   error    0:05       0   $  0.00

running: 1  ok: 1  err: 1  tokens: 4600  cost: $0.04
press q to quit
```

## Why

- **Use your existing logins.** Akuaku drives `claude`, `codex`, and `ollama` as subprocesses — it never talks to an API or handles your tokens.
- **One pane of glass.** See what every agent is doing, whether it succeeded, and what it cost.
- **Open by contract.** The state directory is the public interface: any process that writes a conforming JSON file shows up in the monitor. Nothing is closed.

## Install

```sh
go install github.com/akuaku-ai/akuaku/cmd/akuaku@latest
```

`go install` drops the binary in `$(go env GOPATH)/bin`, which is often not on your `PATH` — so the first run fixes that for you:

```sh
"$(go env GOPATH)/bin/akuaku" setup   # adds akuaku to your PATH, checks backends
```

Then restart your shell (or `source` your profile) and `akuaku` just works.

Or build from source:

```sh
git clone https://github.com/akuaku-ai/akuaku
cd akuaku
make build   # produces bin/akuaku
```

Requires Go 1.24+. To launch agents you also need the `claude`, `codex`, and/or `ollama` CLIs installed and logged in — `akuaku setup` tells you which are missing.

## Usage

Start the monitor:

```sh
akuaku
```

Launch an agent:

```sh
akuaku run claude "refactor the auth module"
akuaku run codex  "write tests for parser.go"
akuaku run ollama "summarize this design" --model llama3.1
```

Flags:

| Flag | Description |
| --- | --- |
| `-m`, `--model` | Model to use (required for `ollama`, optional otherwise) |
| `-n`, `--name` | Display name for the run |

Each run writes one JSON file to the state directory; the monitor reads them on a one-second tick.

## Reflect sessions from other terminals

Not every Claude session starts with `akuaku run`. When you launch Claude Code directly — in another terminal, an IDE, wherever — Akuaku can still surface it in the monitor. One command wires it up:

```sh
akuaku hook install
```

This merges three hooks into your `~/.claude/settings.json` (existing settings and hooks are preserved; re-running is a no-op). From then on, every Claude Code session appears in the monitor as it starts, updates with your first prompt, and flips to `done` when it ends:

```
NAME              BACKEND  STATUS   DUR   TOKENS     COST
refactor auth     claude   running  0:12    1200   $  0.04   ← akuaku run
review PR #42     claude   running  0:03       —        —    ← reflected via hook
```

Reflected runs show `—` for tokens and cost: Claude Code hooks don't expose usage, and Akuaku never reads transcripts. The mechanics are the same contract as everything else — `akuaku hook <event>` is the producer Claude Code calls, and it writes a `source: "hook"` run to the state directory.

## Backends

| Backend | Command | Tokens | Cost |
| --- | --- | --- | --- |
| `claude` | `claude -p … --output-format json` | ✅ | ✅ `total_cost_usd` |
| `codex` | `codex exec --json` | ✅ | — (not reported) |
| `ollama` | `ollama run <model> … --verbose` | ✅ | — (local) |

Token and cost parsing is best-effort: unrecognized output degrades to zero without failing the run.

## How it works

Akuaku is two decoupled halves joined by the filesystem:

- **`akuaku run`** spawns a backend subprocess and writes the run's lifecycle to `state/<id>.json` (`running` → `done`/`error`, with tokens, cost, and exit code). Writes are atomic (temp file + rename).
- **`akuaku`** (the monitor) only *reads* `state/*.json`. It never writes, and it derives every metric by scanning the directory.

Because the state JSON is the only channel, anything can be a producer — `akuaku run`, a Claude Code hook (see above), a script, a cron job, or a future integration — and it appears in the monitor with no code changes. Backends live behind a small interface, so adding one is a definition plus a parser.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `AKUAKU_STATE_DIR` | `~/.akuaku/state` | Directory where runs are read and written |

## Roadmap

- Reflect Codex and other agents started outside Akuaku (Claude Code sessions already work via `akuaku hook install`).
- An embedded, interactive Claude session inside the TUI.
- Alerts → webhooks → connectors.

## Development

```sh
make check   # fmt, vet, lint, test
make cover   # test with coverage
```

The internal packages are held at **100% test coverage**. The project uses [OpenSpec](https://github.com/Fission-AI/OpenSpec) for spec-driven development; specs live under `openspec/`.

## License

[MIT](LICENSE)
