package openai

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    string  `json:"code"`
}

type ChatRequest struct {
	Model               string          `json:"model"`
	Messages            []Message       `json:"messages"`
	Stream              *bool           `json:"stream,omitempty"`
	StreamOptions       map[string]any  `json:"stream_options,omitempty"`
	Tools               any             `json:"tools,omitempty"`
	ToolChoice          any             `json:"tool_choice,omitempty"`
	FunctionCall        any             `json:"function_call,omitempty"`
	Functions           any             `json:"functions,omitempty"`
	Audio               any             `json:"audio,omitempty"`
	Modalities          []string        `json:"modalities,omitempty"`
	ResponseFormat      *ResponseFormat `json:"response_format,omitempty"`
	N                   *int            `json:"n,omitempty"`
	Logprobs            *bool           `json:"logprobs,omitempty"`
	TopLogprobs         any             `json:"top_logprobs,omitempty"`
	WebSearchOptions    any             `json:"web_search_options,omitempty"`
	Prediction          any             `json:"prediction,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	MaxTokens           *int            `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	PresencePenalty     *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64        `json:"frequency_penalty,omitempty"`
	Stop                any             `json:"stop,omitempty"`
	Seed                *int            `json:"seed,omitempty"`
	Metadata            map[string]any  `json:"metadata,omitempty"`
	User                string          `json:"user,omitempty"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ChatCompletion struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   any      `json:"usage,omitempty"`
}

type Choice struct {
	Index        int             `json:"index"`
	Message      AssistantResult `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type AssistantResult struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ChatCompletionChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
}

type ChunkChoice struct {
	Index        int            `json:"index"`
	Delta        map[string]any `json:"delta"`
	FinishReason *string        `json:"finish_reason"`
}
