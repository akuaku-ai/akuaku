# Akuaku roadmap

## North star

Akuaku is the **home base for people who work with AI agents** — the first thing
you open instead of a terminal. From one place you see everything running, open
new agent sessions, jump back into live ones with full fidelity, search your
history, and move between projects. The monitor is the control plane; interactive
sessions are the workspace.

## Product model: two tiers

One OS constraint shapes the design — you can only embed or attach a terminal
session that Akuaku itself launched (you cannot hijack a process's tty from
another window). So sessions fall into two tiers:

| Tier | Sessions | Experience |
| --- | --- | --- |
| **akuaku-native** | launched from Akuaku, held in a PTY | full multiplexer: enter → the real agent TUI in a pane, detach to background, re-attach |
| **external** | started in other terminals (surfaced via hooks / discovery) | monitor only (status, tokens, cost); optionally `--resume` to open a fresh, full-history copy |

To get the rich experience you launch your agents *from* Akuaku, so they live in
Akuaku's PTYs. External sessions are still first-class in the monitor.

## v0.1.0 — the monitor (complete, pending tag)

- Live k9s-style dashboard with its own brand; running-first, status-colored rows.
- `akuaku run` launches an agent and prints its answer; `akuaku demo` shows a
  simulated fleet with zero setup.
- Reflect external Claude sessions via `akuaku hook install`; discover
  already-running agent processes (`:discovery`, on by default).
- Scope to the launch directory (`:local` / `:global`); filter (`/`); rename
  (`:rename`); kill (`k` with confirm, or `:kill`).
- Attention: a `waiting` state plus a bell and banner when an agent finishes,
  fails, or needs input.
- Branded `akuaku help`, `akuaku version`, friendlier errors.

## v0.1.1 — History view

- `h` opens a dedicated history screen (richer than the `a` toggle).
- Columns: **created** and **last message** (adds `LastMessageAt` to the state).
- Search box (extends the existing `/` filter) and a date filter.
- Delete history: `X`, terminal runs only, `y`/`n` confirm — never touches a
  running agent.

## v0.1.2 — Tags / namespaces

- Tag a run or conversation through an overlay (the same mechanism as
  `:rename`, so the run's own state is untouched).
- Group and filter by tag, so related sessions read as one piece of work.

## v0.2.0 — Interactive sessions (flagship)

Validated: `claude -p "…" --resume <session-id>` preserves conversation context
headlessly, so any session is resumable by id.

- **Enter a native session** → the *real* Claude Code, embedded in a pane below a
  metrics header, via a PTY + an in-memory terminal emulator
  (`charmbracelet/x/vt`). Full fidelity: slash commands, tool calls, everything.
- **Detach** with `Ctrl-a d` — Akuaku intercepts the prefix chord before the PTY
  (universal across terminals, the tmux/zellij approach). Optionally
  `Ctrl+Shift+D` where the terminal's modern keyboard protocol distinguishes it.
  Detach never signals the agent: Akuaku simply stops drawing the pane and shows
  the monitor; the process keeps running, even mid-response. Re-enter to resume
  watching. The binding is configurable.
- **Passthrough**: `q` and every other key go straight to the agent — nothing
  closes the session from inside it.
- **New chat**: launch `claude` in a chosen folder, straight from the monitor.
- **Folder navigation**: change the working scope to another project and start
  sessions there.

## Later

- Streaming answer preview for a running `akuaku run`.
- Read-only conversation view for external sessions (would read the session's
  transcript on demand — a deliberate change to today's "never reads
  transcripts" stance, gated on the user opening that session).
- More backends surfaced by discovery.
- Distribution: tagged releases → prebuilt binaries → a Homebrew tap and a
  `curl | sh` installer for a one-line install.

## Risks and the spike gate

The one real risk in v0.2 is embedding a full-screen agent TUI in a pane. Before
committing, a throwaway spike: run a `claude` PTY and render it in a Bubble Tea
pane via `x/vt`, with the `Ctrl-a d` detach. If it renders cleanly and detach
works, build it. If not, fall back to a streaming chat pane (Akuaku renders the
conversation itself instead of the agent's TUI).
