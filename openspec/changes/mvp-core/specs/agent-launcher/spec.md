## ADDED Requirements

### Requirement: Launch command
The command `akuaku run <backend> "<task>" [--model <model>]` SHALL spawn the selected backend as a subprocess and manage the run's full lifecycle in a state file. On start it MUST write a `running` state file; on process exit it MUST update the same run to a terminal status.

#### Scenario: Launch writes a running state file
- **WHEN** `akuaku run` starts a backend subprocess
- **THEN** a state file for the run exists with `status` = `"running"` before the subprocess completes

#### Scenario: Successful subprocess produces a done run
- **WHEN** the backend subprocess exits with code 0
- **THEN** the run's state file is updated to `status` = `"done"` with `ended_at`, `exit_code` = 0, and parsed `tokens` and `cost`

#### Scenario: Failing subprocess produces an error run
- **WHEN** the backend subprocess exits with a non-zero code
- **THEN** the run's state file is updated to `status` = `"error"` with `exit_code` set and `error` containing the failure detail

#### Scenario: Missing backend CLI produces an error run
- **WHEN** the selected backend CLI is not installed or not runnable
- **THEN** the run's state file is set to `status` = `"error"` with an explanatory `error` message

#### Scenario: Invalid backend name fails fast
- **WHEN** `akuaku run` is invoked with an unregistered backend key
- **THEN** the command exits with an error and does not create a state file

### Requirement: Model selection at launch
When `--model <model>` is provided, the launcher SHALL record the model in the run's state file and pass it to the backend command.

#### Scenario: Model is recorded and passed through
- **WHEN** `akuaku run` is invoked with `--model`
- **THEN** the run's state file `model` field equals the provided model and the backend command targets it

### Requirement: Launcher independence from the monitor
The launcher SHALL NOT depend on the monitor running. The state file MUST be the only channel between them.

#### Scenario: Run completes with no monitor present
- **WHEN** `akuaku run` executes while no monitor is running
- **THEN** the run's state file still transitions from `running` to its terminal status
