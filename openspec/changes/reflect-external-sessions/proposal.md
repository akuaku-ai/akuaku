## Why

Today the monitor only shows agents launched with `akuaku run`. But developers already run Claude Code sessions all day across many terminals, panes, and editors — none of which go through Akuaku, so none appear. The highest-leverage adoption feature is **zero-friction visibility**: configure once, then every Claude session anywhere shows up live, with no change to how you work. This is the "single pane of glass" that makes Akuaku worth installing.

## What Changes

- Add `akuaku hook <event>` — a command Claude Code invokes via its hooks. It reads the hook event JSON from stdin and writes/updates one state file per session, keyed by the stable `session_id`. It always exits `0` so it never blocks or interferes with Claude.
  - `SessionStart` → a `running` run (backend `claude`, model, name from the session title).
  - `UserPromptSubmit` → records the session's first prompt as the task.
  - `SessionEnd` → transitions the run to `done`.
- Add `akuaku hook install` — writes the required hooks block into `~/.claude/settings.json`, merging with any existing settings. This one command is what makes reflection usable by everyone (no hand-editing JSON).
- Reflected sessions carry `source: "hook"`. Because Claude Code hook payloads do **not** expose token or cost data (and its transcript format is explicitly unstable), the monitor shows usage as `—` for reflected sessions rather than a misleading `$0.00`. Real usage remains available for `akuaku run` sessions.
- Add `state.Read(dir, id)` so a hook event can update the run it started.

## Capabilities

### New Capabilities
- `session-reflection`: reflecting externally-started Claude Code sessions into the state contract via hooks, the one-command installer, and the monitor's handling of usage-less reflected runs.

### Modified Capabilities
<!-- None at the spec level. The state schema gains an additive optional field and the monitor gains display handling, both covered under session-reflection. -->

## Impact

- **Code**: new `internal/hook` package and an `akuaku hook` subcommand; a `Source` field and a `Read` function in `internal/state`; a display tweak in `internal/tui`.
- **User config**: `akuaku hook install` edits `~/.claude/settings.json` (merging, never clobbering). Documented and reversible.
- **Non-goals**: token/cost for interactive sessions (requires OpenTelemetry ingestion — a later change); codex/ollama discovery (a later `akuaku watch`).
