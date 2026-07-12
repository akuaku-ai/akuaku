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

// Parse reads Claude's single JSON result object: the answer is the `result`
// field, tokens are the sum of input and output tokens, and cost is the reported
// total in USD.
func (claudeBackend) Parse(stdout, _ []byte) Output {
	var result struct {
		Result       string  `json:"result"`
		TotalCostUSD float64 `json:"total_cost_usd"`
		Usage        struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		return Output{}
	}
	return Output{
		Text:   result.Result,
		Tokens: result.Usage.InputTokens + result.Usage.OutputTokens,
		Cost:   result.TotalCostUSD,
	}
}
