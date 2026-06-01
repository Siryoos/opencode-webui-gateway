package openai

import "encoding/json"

type Usage struct {
	PromptTokens            int           `json:"prompt_tokens"`
	CompletionTokens        int           `json:"completion_tokens"`
	TotalTokens             int           `json:"total_tokens"`
	CompletionTokensDetails *UsageDetails `json:"completion_tokens_details,omitempty"`
}

type UsageDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

func UsageFromOpenCodeTokens(tokens map[string]any) (Usage, bool) {
	input, okInput := tokenInt(tokens, "input")
	output, okOutput := tokenInt(tokens, "output")
	if !okInput || !okOutput {
		return Usage{}, false
	}
	reasoning, okReasoning := tokenInt(tokens, "reasoning")
	total, okTotal := tokenInt(tokens, "total")
	if !okTotal {
		total = input + output
		if okReasoning {
			total += reasoning
		}
	}
	usage := Usage{PromptTokens: input, CompletionTokens: output, TotalTokens: total}
	if okReasoning {
		usage.CompletionTokensDetails = &UsageDetails{ReasoningTokens: reasoning}
	}
	return usage, true
}

func tokenInt(tokens map[string]any, key string) (int, bool) {
	if tokens == nil {
		return 0, false
	}
	switch value := tokens[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		if value < 0 || value != float64(int(value)) {
			return 0, false
		}
		return int(value), true
	case json.Number:
		parsed, err := value.Int64()
		if err != nil || parsed < 0 {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}
