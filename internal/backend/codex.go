package backend

import (
	"bytes"
	"encoding/json"
)

// codexBackend drives the Codex CLI via `codex exec --json`.
type codexBackend struct{}

func (codexBackend) Key() string { return "codex" }

func (codexBackend) Command(task, model string) (string, []string) {
	args := []string{"exec", "--json", "--skip-git-repo-check"}
	if model != "" {
		args = append(args, "-m", model)
	}
	return "codex", append(args, task)
}

// Parse reads Codex's JSONL event stream: the answer is the text of the final
// `agent_message` item, and token usage comes from the `turn.completed` event.
// Codex does not report cost, so cost stays zero.
func (codexBackend) Parse(stdout, _ []byte) Output {
	var out Output
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event struct {
			Type string `json:"type"`
			Item *struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		switch {
		case event.Type == "item.completed" && event.Item != nil && event.Item.Type == "agent_message":
			out.Text = event.Item.Text
		case event.Type == "turn.completed" && event.Usage != nil:
			out.Tokens = event.Usage.InputTokens + event.Usage.OutputTokens
		}
	}
	return out
}
