// Package hook reflects Claude Code sessions started outside Akuaku into the run
// state contract. Claude Code invokes `akuaku hook <event>` and streams a JSON
// payload on stdin; Handle maps each event to a create-or-update of the run keyed
// by session_id, so sessions launched in any terminal or IDE surface in the
// monitor alongside runs started with `akuaku run`.
package hook

import (
	"encoding/json"
	"io"
	"time"

	"github.com/akuaku-ai/akuaku/internal/state"
)

// backendClaude labels reflected runs; only Claude Code emits these hooks today.
const backendClaude = "claude"

// sourceHook marks a run as reflected from an external session rather than
// launched by Akuaku, so the monitor knows usage metrics are unavailable.
const sourceHook = "hook"

// defaultName is used when a session start carries no title.
const defaultName = "claude session"

// payload is the subset of a Claude Code hook payload that Akuaku records. Fields
// absent from a given event simply stay empty.
type payload struct {
	SessionID    string `json:"session_id"`
	Model        string `json:"model"`
	SessionTitle string `json:"session_title"`
	UserInput    string `json:"user_input"`
	Cwd          string `json:"cwd"`
}

// Handle maps a Claude Code hook event, whose JSON payload is read from r, to a
// change in the run state at dir. Unrecognized events and payloads that are
// malformed or missing a session_id are no-ops, so a hook never blocks the host
// session. now is injected for deterministic timestamps.
func Handle(event string, r io.Reader, dir string, now time.Time) error {
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil || p.SessionID == "" {
		return nil
	}

	switch event {
	case "SessionStart":
		return state.Write(dir, state.Run{
			ID:        p.SessionID,
			Backend:   backendClaude,
			Name:      sessionName(p),
			Status:    state.StatusRunning,
			Model:     p.Model,
			Source:    sourceHook,
			Dir:       p.Cwd,
			StartedAt: now,
		})
	case "UserPromptSubmit":
		return mutate(dir, p.SessionID, func(run *state.Run) {
			if run.Task == "" {
				run.Task = p.UserInput
			}
		})
	case "SessionEnd":
		return mutate(dir, p.SessionID, func(run *state.Run) {
			run.Status = state.StatusDone
			run.EndedAt = &now
		})
	}
	return nil
}

// mutate applies f to the run identified by id and writes it back. A missing run
// is a no-op; a corrupt existing run surfaces its read error.
func mutate(dir, id string, f func(*state.Run)) error {
	run, found, err := state.Read(dir, id)
	if err != nil || !found {
		return err
	}
	f(&run)
	return state.Write(dir, run)
}

// sessionName prefers the session's title, falling back to a generic label.
func sessionName(p payload) string {
	if p.SessionTitle != "" {
		return p.SessionTitle
	}
	return defaultName
}
