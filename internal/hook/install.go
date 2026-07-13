package hook

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
)

const filePerm = 0o644

// Seams over the standard library so the otherwise-unreachable I/O error paths
// stay testable while the production behavior is a thin pass-through.
var (
	marshal = func(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }
	rename  = os.Rename
)

// installEvents are the Claude Code lifecycle events Akuaku reflects into the
// monitor. Keep in sync with the switch in Handle.
var installEvents = []string{"SessionStart", "UserPromptSubmit", "Notification", "Stop", "SessionEnd"}

// hookEntry is one Claude Code hook command. The fields mirror Claude Code's
// settings schema so unrelated hooks survive a round trip untouched.
type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// hookGroup is a matcher and the commands it triggers, as stored under each
// event in Claude Code's settings.
type hookGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

// Install merges Akuaku's session-reflection hooks into the Claude Code settings
// file at settingsPath. command is the Akuaku invocation prefix (e.g. "akuaku"),
// to which " hook <event>" is appended for each reflected event. Every unrelated
// setting and any pre-existing hook is preserved, and the merge is idempotent:
// running it twice never duplicates Akuaku's commands. The write is atomic, so a
// crash mid-install never corrupts the user's settings.
func Install(settingsPath, command string) error {
	data, err := os.ReadFile(settingsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// root keeps every unrelated setting; typed.Hooks is the view we merge into.
	root := map[string]any{}
	var typed struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return err
		}
		if err := json.Unmarshal(data, &typed); err != nil {
			return err
		}
	}

	hooks := typed.Hooks
	if hooks == nil {
		hooks = map[string][]hookGroup{}
	}
	for _, event := range installEvents {
		cmd := command + " hook " + event
		if !hasCommand(hooks[event], cmd) {
			hooks[event] = append(hooks[event], hookGroup{
				Hooks: []hookEntry{{Type: "command", Command: cmd}},
			})
		}
	}
	root["hooks"] = hooks

	return writeSettings(settingsPath, root)
}

// hasCommand reports whether any group already runs command, making Install
// idempotent and preserving hooks the user configured themselves.
func hasCommand(groups []hookGroup, command string) bool {
	for _, g := range groups {
		for _, entry := range g.Hooks {
			if entry.Command == command {
				return true
			}
		}
	}
	return false
}

// writeSettings serializes root to a temporary file and atomically renames it
// over settingsPath.
func writeSettings(settingsPath string, root map[string]any) error {
	data, err := marshal(root)
	if err != nil {
		return err
	}
	tmp := settingsPath + ".tmp"
	if err := os.WriteFile(tmp, data, filePerm); err != nil {
		return err
	}
	return rename(tmp, settingsPath)
}
