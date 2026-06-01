package openai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type streamRecorder struct {
	header  http.Header
	body    bytes.Buffer
	flushes int
	status  int
}

func newStreamRecorder() *streamRecorder {
	return &streamRecorder{header: make(http.Header)}
}

func (r *streamRecorder) Header() http.Header {
	return r.header
}

func (r *streamRecorder) Write(data []byte) (int, error) {
	return r.body.Write(data)
}

func (r *streamRecorder) WriteHeader(status int) {
	r.status = status
}

func (r *streamRecorder) Flush() {
	r.flushes++
}

func TestStreamWriterEmitsInitialRoleChunk(t *testing.T) {
	rec := newStreamRecorder()
	writer := NewStreamWriter(rec, "chatcmpl-test", 123, "adina-analysis")
	if err := writer.WriteRole(); err != nil {
		t.Fatalf("WriteRole returned error: %v", err)
	}
	chunk := parseChunks(t, rec.body.String())[0]
	if got := chunk["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["role"]; got != "assistant" {
		t.Fatalf("expected assistant role delta, got %v", got)
	}
}

func TestStreamWriterEmitsTextDeltaChunk(t *testing.T) {
	rec := newStreamRecorder()
	writer := NewStreamWriter(rec, "chatcmpl-test", 123, "adina-analysis")
	if err := writer.WriteTextDelta("hello"); err != nil {
		t.Fatalf("WriteTextDelta returned error: %v", err)
	}
	chunk := parseChunks(t, rec.body.String())[0]
	if got := chunk["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["content"]; got != "hello" {
		t.Fatalf("expected text delta, got %v", got)
	}
}

func TestStreamWriterEmitsFinalStopChunkAndDone(t *testing.T) {
	rec := newStreamRecorder()
	writer := NewStreamWriter(rec, "chatcmpl-test", 123, "adina-analysis")
	if err := writer.WriteStop(); err != nil {
		t.Fatalf("WriteStop returned error: %v", err)
	}
	if err := writer.WriteDone(); err != nil {
		t.Fatalf("WriteDone returned error: %v", err)
	}
	body := rec.body.String()
	chunk := parseChunks(t, body)[0]
	choice := chunk["choices"].([]any)[0].(map[string]any)
	if got := choice["finish_reason"]; got != "stop" {
		t.Fatalf("expected stop finish_reason, got %v", got)
	}
	if !strings.HasSuffix(body, "data: [DONE]\n\n") {
		t.Fatalf("expected final DONE frame, got %q", body)
	}
}

func TestStreamWriterUsageAppearsBeforeDoneWhenAvailable(t *testing.T) {
	rec := newStreamRecorder()
	writer := NewStreamWriter(rec, "chatcmpl-test", 123, "adina-analysis")
	if err := writer.WriteRole(); err != nil {
		t.Fatalf("WriteRole returned error: %v", err)
	}
	if err := writer.WriteUsage(map[string]any{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5}); err != nil {
		t.Fatalf("WriteUsage returned error: %v", err)
	}
	if err := writer.WriteDone(); err != nil {
		t.Fatalf("WriteDone returned error: %v", err)
	}
	body := rec.body.String()
	if strings.Index(body, `"usage"`) < 0 {
		t.Fatalf("expected usage chunk in body %q", body)
	}
	if strings.Index(body, `"usage"`) > strings.Index(body, "data: [DONE]") {
		t.Fatalf("expected usage before DONE, got %q", body)
	}
	chunks := parseChunks(t, body)
	usageChunk := chunks[1]
	if len(usageChunk["choices"].([]any)) != 0 {
		t.Fatalf("expected usage chunk choices to be empty, got %v", usageChunk["choices"])
	}
}

func TestStreamWriterNoUsageChunkWhenNil(t *testing.T) {
	rec := newStreamRecorder()
	writer := NewStreamWriter(rec, "chatcmpl-test", 123, "adina-analysis")
	if err := writer.WriteRole(); err != nil {
		t.Fatalf("WriteRole returned error: %v", err)
	}
	if err := writer.WriteUsage(nil); err != nil {
		t.Fatalf("WriteUsage nil returned error: %v", err)
	}
	if err := writer.WriteDone(); err != nil {
		t.Fatalf("WriteDone returned error: %v", err)
	}
	if strings.Contains(rec.body.String(), `"usage"`) {
		t.Fatalf("did not expect usage chunk, got %q", rec.body.String())
	}
}

func TestStreamWriterAllChunksShareIDCreatedModel(t *testing.T) {
	rec := newStreamRecorder()
	writer := NewStreamWriter(rec, "chatcmpl-shared", 456, "adina-execution")
	for _, call := range []func() error{
		writer.WriteRole,
		func() error { return writer.WriteTextDelta("hello") },
		writer.WriteStop,
		func() error { return writer.WriteUsage(map[string]any{"total_tokens": 1}) },
	} {
		if err := call(); err != nil {
			t.Fatalf("write returned error: %v", err)
		}
	}
	for _, chunk := range parseChunks(t, rec.body.String()) {
		if chunk["id"] != "chatcmpl-shared" || chunk["created"] != float64(456) || chunk["model"] != "adina-execution" {
			t.Fatalf("chunk identity mismatch: %v", chunk)
		}
		if chunk["object"] != "chat.completion.chunk" {
			t.Fatalf("unexpected object: %v", chunk["object"])
		}
	}
}

func TestStreamWriterSetsSSEHeaders(t *testing.T) {
	rec := newStreamRecorder()
	_ = NewStreamWriter(rec, "chatcmpl-test", 123, "adina-analysis")
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("unexpected Content-Type %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("unexpected Cache-Control %q", got)
	}
}

func TestStreamWriterFlushesAfterEachFrame(t *testing.T) {
	rec := newStreamRecorder()
	writer := NewStreamWriter(rec, "chatcmpl-test", 123, "adina-analysis")
	if err := writer.WriteRole(); err != nil {
		t.Fatalf("WriteRole returned error: %v", err)
	}
	if err := writer.WriteTextDelta("hello"); err != nil {
		t.Fatalf("WriteTextDelta returned error: %v", err)
	}
	if err := writer.WriteStop(); err != nil {
		t.Fatalf("WriteStop returned error: %v", err)
	}
	if err := writer.WriteDone(); err != nil {
		t.Fatalf("WriteDone returned error: %v", err)
	}
	if rec.flushes != 4 {
		t.Fatalf("expected 4 flushes, got %d", rec.flushes)
	}
}

func parseChunks(t *testing.T, body string) []map[string]any {
	t.Helper()
	var chunks []map[string]any
	for _, frame := range strings.Split(body, "\n\n") {
		if frame == "" || frame == "data: [DONE]" {
			continue
		}
		if !strings.HasPrefix(frame, "data: ") {
			t.Fatalf("unexpected frame %q", frame)
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &chunk); err != nil {
			t.Fatalf("invalid chunk JSON %q: %v", frame, err)
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}
