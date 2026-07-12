## ADDED Requirements

### Requirement: Supported backends
The backend registry SHALL provide the backends `claude`, `codex`, and `ollama`, each identified by a unique key. Requesting an unknown backend key MUST be rejected.

#### Scenario: Registry exposes the three MVP backends
- **WHEN** the registry is queried for available backends
- **THEN** it returns definitions keyed `claude`, `codex`, and `ollama`

#### Scenario: Unknown backend is rejected
- **WHEN** a backend key that is not registered is requested
- **THEN** the registry returns an error and no backend definition

### Requirement: Command construction
Each backend SHALL build a non-interactive command from a task and an optional model, producing machine-readable output. When a model is provided it MUST be included in the command; when omitted, the backend's default model MUST be used.

#### Scenario: Claude builds a non-interactive JSON command
- **WHEN** the `claude` backend builds a command for a task
- **THEN** the command runs `claude` in print mode with JSON output for that task

#### Scenario: Model override is applied
- **WHEN** a backend builds a command with an explicit model
- **THEN** the built command targets that model

#### Scenario: Omitted model uses the backend default
- **WHEN** a backend builds a command without a model
- **THEN** the built command does not force a model and the backend default is used

### Requirement: Output parsing
Each backend SHALL parse its process output into token count and cost on a best-effort basis. If output cannot be parsed, tokens and cost MUST default to zero and the run MUST remain valid.

#### Scenario: Claude output yields tokens and cost
- **WHEN** the `claude` backend parses its JSON output containing usage and cost
- **THEN** it returns the reported token count and cost

#### Scenario: Codex output yields tokens with zero cost
- **WHEN** the `codex` backend parses its JSONL output
- **THEN** it returns the reported token count and a cost of zero

#### Scenario: Ollama reports zero cost
- **WHEN** the `ollama` backend parses its output
- **THEN** it returns a cost of zero and a best-effort token count

#### Scenario: Unparseable output degrades to zeros
- **WHEN** a backend receives output it cannot parse
- **THEN** it returns zero tokens and zero cost without producing an error

### Requirement: Backend extensibility
Adding a new backend SHALL require only registering a new backend definition. It MUST NOT require changes to the launcher or the monitor.

#### Scenario: New backend is usable without touching other components
- **WHEN** a new backend definition is registered
- **THEN** it can be launched and monitored without modifying launcher or monitor code
