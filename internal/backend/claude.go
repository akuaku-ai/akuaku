package backend

import "encoding/json"

// claudeBackend drives the Claude Code CLI via `claude -p ... --output-format json`.
type claudeBackend struct{}

func (claudeBackend) Key() string { return "claude" }

func (claudeBackend) Command(task, model string) (string, []string) {
	args := []string{"-p", task, "--output-format", "json"}
	if model != "" {
		args = append(args, "--model", model)
	}
	return "claude", args
}

// Parse reads Claude's single JSON result object. Tokens are the sum of input
// and output tokens; cost is the reported total in USD.
func (claudeBackend) Parse(stdout, _ []byte) (int, float64) {
	var result struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
		Usage        struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		return 0, 0
	}
	return result.Usage.InputTokens + result.Usage.OutputTokens, result.TotalCostUSD
}
