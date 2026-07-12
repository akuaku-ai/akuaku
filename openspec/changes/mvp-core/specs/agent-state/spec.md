## ADDED Requirements

### Requirement: Run state file schema
Each agent run SHALL be represented by exactly one JSON file describing that run. The file MUST contain the fields: `id`, `backend`, `name`, `status`, `task`, `model`, `started_at`, `ended_at`, `tokens`, `cost`, `exit_code`, and `error`. Timestamps MUST be RFC 3339 strings. While a run is in progress, `ended_at`, `exit_code`, and `error` MUST be null.

#### Scenario: Newly launched run is serialized with running fields
- **WHEN** a run is created
- **THEN** its state file contains `status` = `"running"`, a non-empty `id`, `backend`, `task`, and `started_at`, and `ended_at`, `exit_code`, and `error` are null

#### Scenario: Completed run freezes its terminal fields
- **WHEN** a run reaches a terminal status
- **THEN** its state file contains a non-null `ended_at` and `exit_code`, and `status` is `"done"` or `"error"`

### Requirement: File naming and storage location
A run's state file SHALL be named `<id>.json`, where `id` is composed of the backend key, a timestamp, and a random suffix, guaranteeing uniqueness across concurrent runs. State files SHALL live in the state directory, which defaults to `./state` and is overridable via the `AKUAKU_STATE_DIR` environment variable.

#### Scenario: Default state directory
- **WHEN** `AKUAKU_STATE_DIR` is not set
- **THEN** state files are read from and written to `./state`

#### Scenario: Overridden state directory
- **WHEN** `AKUAKU_STATE_DIR` is set to a path
- **THEN** state files are read from and written to that path instead of `./state`

#### Scenario: Concurrent runs produce distinct files
- **WHEN** two runs are launched at the same time
- **THEN** they produce two state files with different `id` values that do not overwrite each other

### Requirement: Atomic writes
A producer SHALL write a state file by writing to a temporary file and atomically renaming it into place, so that a reader never observes a partially written file.

#### Scenario: Reader never sees partial JSON
- **WHEN** a producer is midway through writing a state file
- **THEN** a concurrent reader either sees the previous complete file or the new complete file, never a truncated one

### Requirement: Status lifecycle
A run's `status` SHALL be one of `running`, `done`, or `error`. A run MUST start as `running` and transition exactly once to a terminal status: `done` on success or `error` on failure.

#### Scenario: Successful run transitions to done
- **WHEN** a run's underlying process exits with code 0
- **THEN** its `status` becomes `"done"` and `exit_code` is 0

#### Scenario: Failed run transitions to error
- **WHEN** a run's underlying process exits with a non-zero code
- **THEN** its `status` becomes `"error"`, `exit_code` is that code, and `error` is a non-empty message

### Requirement: Open producer contract
Any process SHALL be able to surface a run by writing a conforming state file. Consumers MUST NOT require a state file to have been produced by `akuaku run`.

#### Scenario: Third-party writer appears to consumers
- **WHEN** a process other than `akuaku run` writes a conforming state file into the state directory
- **THEN** consumers of the state directory treat it as a valid run identical to one produced by `akuaku run`
