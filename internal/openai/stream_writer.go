package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type StreamWriter struct {
	w       http.ResponseWriter
	id      string
	created int64
	model   string
}

func NewStreamWriter(w http.ResponseWriter, id string, created int64, model string) *StreamWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	return &StreamWriter{w: w, id: id, created: created, model: model}
}

func (w *StreamWriter) WriteRole() error {
	return w.writeChunk(ChatCompletionChunk{
		ID:      w.id,
		Object:  "chat.completion.chunk",
		Created: w.created,
		Model:   w.model,
		Choices: []ChunkChoice{{Index: 0, Delta: map[string]any{"role": "assistant"}, FinishReason: nil}},
	})
}

func (w *StreamWriter) WriteTextDelta(text string) error {
	return w.writeChunk(ChatCompletionChunk{
		ID:      w.id,
		Object:  "chat.completion.chunk",
		Created: w.created,
		Model:   w.model,
		Choices: []ChunkChoice{{Index: 0, Delta: map[string]any{"content": text}, FinishReason: nil}},
	})
}

func (w *StreamWriter) WriteStop() error {
	stop := "stop"
	return w.writeChunk(ChatCompletionChunk{
		ID:      w.id,
		Object:  "chat.completion.chunk",
		Created: w.created,
		Model:   w.model,
		Choices: []ChunkChoice{{Index: 0, Delta: map[string]any{}, FinishReason: &stop}},
	})
}

func (w *StreamWriter) WriteUsage(usage any) error {
	if usage == nil {
		return nil
	}
	return w.writeChunk(ChatCompletionChunk{
		ID:      w.id,
		Object:  "chat.completion.chunk",
		Created: w.created,
		Model:   w.model,
		Choices: []ChunkChoice{},
		Usage:   usage,
	})
}

func (w *StreamWriter) WriteDone() error {
	if _, err := fmt.Fprint(w.w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	return flush(w.w)
}

func (w *StreamWriter) writeChunk(chunk ChatCompletionChunk) error {
	data, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.w, "data: %s\n\n", data); err != nil {
		return err
	}
	return flush(w.w)
}

type flushErrorer interface {
	FlushError() error
}

func flush(w http.ResponseWriter) error {
	if f, ok := w.(flushErrorer); ok {
		return f.FlushError()
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}
