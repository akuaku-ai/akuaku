## ADDED Requirements

### Requirement: Read-only state scanning
The monitor SHALL read state files from the state directory on a one-second tick and MUST NOT write to the state directory. A state file that cannot be parsed MUST be skipped without crashing the monitor.

#### Scenario: Monitor reloads on each tick
- **WHEN** a tick occurs
- **THEN** the monitor re-reads the state directory and reflects the current set of runs

#### Scenario: New run appears without restart
- **WHEN** a new state file is added to the state directory
- **THEN** the next tick shows the new run in the monitor

#### Scenario: Unparseable file is skipped
- **WHEN** the state directory contains a file that is not valid run JSON
- **THEN** the monitor ignores that file and continues rendering the valid runs

### Requirement: Agent table
The monitor SHALL render one row per run showing at least name, backend, status, duration, tokens, and cost. Duration MUST be computed live as now minus `started_at` while the run is `running`, and as `ended_at` minus `started_at` once the run is terminal.

#### Scenario: Rows are rendered for runs
- **WHEN** the state directory contains runs
- **THEN** the monitor renders one table row per run with its name, backend, status, duration, tokens, and cost

#### Scenario: Live duration for running runs
- **WHEN** a run has `status` = `"running"`
- **THEN** its displayed duration increases on each tick as now minus `started_at`

#### Scenario: Frozen duration for terminal runs
- **WHEN** a run has a terminal status
- **THEN** its displayed duration is fixed at `ended_at` minus `started_at`

### Requirement: Derived metrics panel
The monitor SHALL display a metrics panel computed solely from the scanned state files, including the running count, `runs_ok` (count of `done`), `runs_err` (count of `error`), total tokens, and total cost.

#### Scenario: Aggregates reflect the scanned runs
- **WHEN** the state directory contains runs with mixed statuses
- **THEN** the panel shows the running count, `runs_ok` equal to the number of `done` runs, `runs_err` equal to the number of `error` runs, and the summed tokens and cost

### Requirement: Keybindings
The monitor SHALL support quitting with `q`, navigating runs with the arrow keys, and launching an agent with `l`. Launching MUST execute `akuaku run` as a detached process so the monitor remains responsive.

#### Scenario: Quit exits the monitor
- **WHEN** the user presses `q`
- **THEN** the monitor exits

#### Scenario: Launch is non-blocking
- **WHEN** the user triggers a launch with `l`
- **THEN** the monitor spawns `akuaku run` detached and remains responsive without waiting for the run to finish
