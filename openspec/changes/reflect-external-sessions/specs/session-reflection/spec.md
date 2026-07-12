## ADDED Requirements

### Requirement: Reflect Claude sessions from hook events
The `akuaku hook <event>` command SHALL read a Claude Code hook payload as JSON from standard input and record the session as a run keyed by its `session_id`. It MUST always exit with status `0` so it never blocks or interferes with Claude, even when the input is missing or malformed.

#### Scenario: Session start creates a running run
- **WHEN** `akuaku hook` receives a `SessionStart` payload with a `session_id`
- **THEN** it writes a run whose id is that `session_id`, backend `claude`, status `running`, with a start time recorded

#### Scenario: Session end completes the run
- **WHEN** `akuaku hook` receives a `SessionEnd` payload for a `session_id` that has a running run
- **THEN** the run's status becomes `done` and its end time is recorded, preserving the original start time and task

#### Scenario: A prompt records the task
- **WHEN** `akuaku hook` receives a `UserPromptSubmit` payload and the run has no task yet
- **THEN** the run's task is set to the submitted prompt text

#### Scenario: Malformed input never blocks Claude
- **WHEN** `akuaku hook` receives input that is not a valid payload
- **THEN** it exits with status `0` and writes no run

### Requirement: Reflected runs are marked and usage-less
A run produced by a hook SHALL carry `source: "hook"`. Because Claude Code hook payloads do not expose token or cost data, reflected runs MUST NOT claim usage, and the monitor SHALL display their tokens and cost as `—` rather than zero.

#### Scenario: Reflected run is tagged
- **WHEN** a hook writes a run
- **THEN** the run's `source` is `"hook"` and its tokens and cost are absent

#### Scenario: Monitor shows a dash for reflected usage
- **WHEN** the monitor renders a run whose source is `"hook"`
- **THEN** the tokens and cost columns show `—`, not `0`

### Requirement: One-command hook installation
The `akuaku hook install` command SHALL add the required Claude Code hooks to the user's `~/.claude/settings.json`, merging with any existing content and never discarding unrelated settings. Running it again MUST NOT duplicate the hooks.

#### Scenario: Install into a fresh settings file
- **WHEN** `akuaku hook install` runs and no settings file exists
- **THEN** a settings file is created containing the Akuaku SessionStart, SessionEnd, and UserPromptSubmit hooks

#### Scenario: Install preserves existing settings
- **WHEN** the settings file already contains unrelated keys and hooks
- **THEN** after install those keys and hooks remain and the Akuaku hooks are added

#### Scenario: Install is idempotent
- **WHEN** `akuaku hook install` runs twice
- **THEN** the Akuaku hooks appear exactly once
