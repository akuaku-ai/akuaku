<!--
Build order: extend the state contract, map hook events, wire the CLI, add the
installer, teach the monitor to show reflected usage as a dash, then document
and verify end-to-end. Every group ends with tests at 100% coverage.
-->

## 1. State support

- [x] 1.1 Add a `Source` field to `state.Run` (`"run"` by default, `"hook"` for reflected sessions), omitempty
- [x] 1.2 Add `state.Read(dir, id)` to read one run by id (missing → not found, no error)
- [x] 1.3 Tests for `Read` (found, missing, unreadable, unparseable) at 100%

## 2. Hook event mapping

- [x] 2.1 Add `internal/hook` mapping a payload + event name to a run keyed by `session_id`
- [x] 2.2 SessionStart → running; UserPromptSubmit → set task if empty; SessionEnd → done, preserving start
- [x] 2.3 Read-modify-write via `state.Read`/`state.Write`; malformed input is a no-op
- [x] 2.4 Tests (each event, unknown event, bad JSON, start→end correlation) at 100%

## 3. CLI dispatch

- [x] 3.1 Add `akuaku hook <event>` to `internal/cli`, reading stdin and always exiting 0
- [x] 3.2 Tests for dispatch and the always-zero exit at 100%

## 4. Installer

- [x] 4.1 Add `akuaku hook install`: load `~/.claude/settings.json`, merge the Akuaku hooks, write back
- [x] 4.2 Preserve unrelated settings; make it idempotent
- [x] 4.3 Tests (fresh file, existing settings, idempotency, unreadable/unwritable) at 100%

## 5. Monitor display

- [x] 5.1 Render tokens and cost as `—` when a run's source is `"hook"`
- [x] 5.2 Tests for the reflected-run rendering at 100%

## 6. Docs & verification

- [x] 6.1 Document `akuaku hook` and `akuaku hook install` in the README
- [x] 6.2 Verify end-to-end: install the hook, start a Claude session, confirm it appears in the monitor
- [x] 6.3 Run `openspec validate reflect-external-sessions --strict`
