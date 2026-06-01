package openai

import (
	"errors"
	"fmt"
)

type ValidationError struct {
	Status  int
	Message string
	Type    string
	Param   string
	Code    string
}

func (e ValidationError) Error() string { return e.Message }

func ExtractLatest(req ChatRequest) (system string, latest string, err error) {
	if req.Model == "" {
		return "", "", invalid("model", "missing_required_parameter", "model is required")
	}
	if len(req.Messages) == 0 {
		return "", "", invalid("messages", "missing_required_parameter", "messages is required")
	}
	if err := validateControls(req); err != nil {
		return "", "", err
	}
	if req.Tools != nil {
		return "", "", invalid("tools", "unsupported_parameter", "tools are not supported")
	}
	if req.ToolChoice != nil {
		return "", "", invalid("tool_choice", "unsupported_parameter", "tool_choice is not supported")
	}
	if req.FunctionCall != nil {
		return "", "", invalid("function_call", "unsupported_parameter", "function_call is not supported")
	}
	if req.Functions != nil {
		return "", "", invalid("functions", "unsupported_parameter", "functions are not supported")
	}
	if req.Audio != nil {
		return "", "", invalid("audio", "unsupported_parameter", "audio is not supported")
	}
	for _, modality := range req.Modalities {
		if modality != "text" {
			return "", "", invalid("modalities", "unsupported_parameter", "only text modality is supported")
		}
	}
	if req.N != nil && *req.N != 1 {
		return "", "", invalid("n", "unsupported_parameter", "n must be 1")
	}
	if req.Logprobs != nil && *req.Logprobs {
		return "", "", invalid("logprobs", "unsupported_parameter", "logprobs are not supported")
	}
	if req.TopLogprobs != nil {
		return "", "", invalid("top_logprobs", "unsupported_parameter", "top_logprobs is not supported")
	}
	if req.WebSearchOptions != nil {
		return "", "", invalid("web_search_options", "unsupported_parameter", "web_search_options is not supported")
	}
	if req.Prediction != nil {
		return "", "", invalid("prediction", "unsupported_parameter", "prediction is not supported")
	}
	if req.ParallelToolCalls != nil && *req.ParallelToolCalls {
		return "", "", invalid("parallel_tool_calls", "unsupported_parameter", "parallel_tool_calls is not supported")
	}
	for key := range req.StreamOptions {
		if key != "include_usage" {
			return "", "", invalid("stream_options", "unsupported_parameter", "unsupported stream_options field")
		}
	}
	if req.ResponseFormat != nil && (req.ResponseFormat.Type == "json_object" || req.ResponseFormat.Type == "json_schema") {
		return "", "", invalid("response_format", "unsupported_parameter", "JSON response formats are not supported")
	}
	if len(req.Messages) == 0 || req.Messages[len(req.Messages)-1].Role != "user" {
		return "", "", invalid("messages", "invalid_request_error", "final message must be a user message")
	}

	for i, msg := range req.Messages {
		switch msg.Role {
		case "system":
			text, ok := textOnly(msg.Content)
			if !ok {
				return "", "", invalid("messages", "unsupported_parameter", "system content must be text-only")
			}
			if system != "" && text != "" {
				system += "\n\n"
			}
			system += text
		case "user":
			if i != len(req.Messages)-1 {
				continue
			}
			text, ok := textOnly(msg.Content)
			if !ok {
				return "", "", invalid("messages", "unsupported_parameter", "user content must be text-only")
			}
			latest = text
		case "assistant":
			continue
		case "tool", "function", "developer":
			return "", "", invalid("messages", "unsupported_parameter", fmt.Sprintf("role %q is not supported", msg.Role))
		default:
			return "", "", invalid("messages", "unsupported_parameter", fmt.Sprintf("role %q is not supported", msg.Role))
		}
	}
	if latest == "" {
		return "", "", invalid("messages", "invalid_request_error", "latest user message is required")
	}
	return system, latest, nil
}

func validateControls(req ChatRequest) error {
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2) {
		return invalid("temperature", "invalid_value", "temperature must be between 0 and 2")
	}
	if req.TopP != nil && (*req.TopP < 0 || *req.TopP > 1) {
		return invalid("top_p", "invalid_value", "top_p must be between 0 and 1")
	}
	if req.MaxTokens != nil && *req.MaxTokens < 0 {
		return invalid("max_tokens", "invalid_value", "max_tokens must be non-negative")
	}
	if req.MaxCompletionTokens != nil && *req.MaxCompletionTokens < 0 {
		return invalid("max_completion_tokens", "invalid_value", "max_completion_tokens must be non-negative")
	}
	if req.PresencePenalty != nil && (*req.PresencePenalty < -2 || *req.PresencePenalty > 2) {
		return invalid("presence_penalty", "invalid_value", "presence_penalty must be between -2 and 2")
	}
	if req.FrequencyPenalty != nil && (*req.FrequencyPenalty < -2 || *req.FrequencyPenalty > 2) {
		return invalid("frequency_penalty", "invalid_value", "frequency_penalty must be between -2 and 2")
	}
	if req.Stop != nil && !validStop(req.Stop) {
		return invalid("stop", "invalid_value", "stop must be a string or an array of up to four strings")
	}
	return nil
}

func validStop(value any) bool {
	switch stop := value.(type) {
	case string:
		return true
	case []any:
		if len(stop) > 4 {
			return false
		}
		for _, item := range stop {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func invalid(param, code, message string) ValidationError {
	return ValidationError{Status: 400, Message: message, Type: "invalid_request_error", Param: param, Code: code}
}

func textOnly(value any) (string, bool) {
	switch content := value.(type) {
	case string:
		return content, true
	case []any:
		out := ""
		for _, item := range content {
			m, ok := item.(map[string]any)
			if !ok || m["type"] != "text" {
				return "", false
			}
			text, ok := m["text"].(string)
			if !ok {
				return "", false
			}
			out += text
		}
		return out, true
	case nil:
		return "", false
	default:
		return "", false
	}
}

func IsValidation(err error) (ValidationError, bool) {
	var target ValidationError
	if errors.As(err, &target) {
		return target, true
	}
	return ValidationError{}, false
}
