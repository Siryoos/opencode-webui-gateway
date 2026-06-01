package httpapi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adina/opencode-webui-gateway/internal/auth"
	"github.com/adina/opencode-webui-gateway/internal/httpapi"
	"github.com/adina/opencode-webui-gateway/internal/ledger"
	"github.com/adina/opencode-webui-gateway/internal/opencode"
)

type ocState struct {
	sessions       int
	messages       []map[string]any
	authorizations []string
	eventCalls     int
	eventStatus    int
	eventBody      string
	promptAsync    int
	promptStatus   int
	getMessages    int
	getStatus      int
	getBody        string
	messageStatus  int
	messageBody    string
	sleep          time.Duration
}

func newTestServer(t *testing.T, password string) (*httptest.Server, *ocState) {
	t.Helper()
	state := &ocState{}
	state.eventStatus = http.StatusNotFound
	state.promptStatus = http.StatusNotFound
	state.getStatus = http.StatusOK
	state.getBody = `[]`
	state.messageStatus = http.StatusOK
	state.messageBody = `{"info":{"id":"msg_test","sessionID":"ses_test","role":"assistant","agent":"plan"},"parts":[{"type":"reasoning","text":"hidden"},{"type":"text","text":"hello"},{"type":"tool","text":"hidden"},{"type":"text","text":" world"}]}`
	oc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.authorizations = append(state.authorizations, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/event" || r.URL.Path == "/global/event":
			state.eventCalls++
			if state.eventStatus != http.StatusOK {
				http.Error(w, "event unavailable", state.eventStatus)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(state.eventBody))
		case r.Method == http.MethodGet && r.URL.Path == "/global/health":
			_, _ = w.Write([]byte(`{"healthy":true,"version":"1.15.13"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/agent":
			_, _ = w.Write([]byte(`[{"name":"plan"},{"name":"build"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			state.sessions++
			_, _ = w.Write([]byte(`{"id":"ses_test_` + string(rune('0'+state.sessions)) + `","title":"test","version":"1.15.13","time":{"created":1,"updated":1}}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/session/") && strings.HasSuffix(r.URL.Path, "/message"):
			state.getMessages++
			w.WriteHeader(state.getStatus)
			_, _ = w.Write([]byte(state.getBody))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/session/") && strings.HasSuffix(r.URL.Path, "/message"):
			if state.sleep > 0 {
				time.Sleep(state.sleep)
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			state.messages = append(state.messages, body)
			w.WriteHeader(state.messageStatus)
			_, _ = w.Write([]byte(state.messageBody))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/prompt_async"):
			state.promptAsync++
			w.WriteHeader(state.promptStatus)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(oc.Close)
	return oc, state
}

func newGateway(t *testing.T, ocURL, password string) (http.Handler, *ledger.Ledger) {
	return newGatewayWithTimeout(t, ocURL, password, 5*time.Second)
}

func newGatewayWithTimeout(t *testing.T, ocURL, password string, timeout time.Duration) (http.Handler, *ledger.Ledger) {
	t.Helper()
	led, err := ledger.Open(t.TempDir() + "/gateway.sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = led.Close() })
	return httpapi.New(auth.NewValidator("secret"), false, opencode.New(ocURL, "opencode", password, timeout), led, slog.Default()), led
}

func doReq(handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func authHeaders() map[string]string {
	return map[string]string{"Authorization": "Bearer secret", "X-OpenWebUI-User-Id": "user-1", "X-OpenWebUI-Chat-Id": "chat-1"}
}

func TestAuth(t *testing.T) {
	oc, _ := newTestServer(t, "")
	handler, led := newGateway(t, oc.URL, "")
	if got := doReq(handler, http.MethodGet, "/v1/models", "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("missing bearer status = %d", got)
	}
	if got := doReq(handler, http.MethodGet, "/v1/models", "", map[string]string{"Authorization": "Bearer wrong"}).Code; got != http.StatusUnauthorized {
		t.Fatalf("wrong bearer status = %d", got)
	}
	if got := doReq(handler, http.MethodGet, "/v1/models", "", map[string]string{"Authorization": "Bearer secret"}).Code; got != http.StatusOK {
		t.Fatalf("valid bearer status = %d", got)
	}
	_ = doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, map[string]string{"Authorization": "Bearer wrong", "X-OpenWebUI-User-Id": "u", "X-OpenWebUI-Chat-Id": "c"})
	count, _ := led.Count(context.Background())
	if count != 0 {
		t.Fatalf("auth failure touched ledger, count=%d", count)
	}
}

func TestAuthFailureDoesNotCallOpenCode(t *testing.T) {
	oc, state := newTestServer(t, "")
	handler, led := newGateway(t, oc.URL, "")
	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{name: "missing bearer", headers: map[string]string{"X-OpenWebUI-User-Id": "u", "X-OpenWebUI-Chat-Id": "c"}},
		{name: "invalid bearer", headers: map[string]string{"Authorization": "Bearer wrong", "X-OpenWebUI-User-Id": "u", "X-OpenWebUI-Chat-Id": "c"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"hi"}]}`, tc.headers)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
	count, _ := led.Count(context.Background())
	if count != 0 {
		t.Fatalf("auth failure touched ledger, count=%d", count)
	}
	if len(state.authorizations) != 0 || state.sessions != 0 || state.eventCalls != 0 || state.promptAsync != 0 || len(state.messages) != 0 || state.getMessages != 0 {
		t.Fatalf("auth failure called OpenCode: auth=%d sessions=%d events=%d prompt=%d postMsg=%d getMsg=%d", len(state.authorizations), state.sessions, state.eventCalls, state.promptAsync, len(state.messages), state.getMessages)
	}
}

func TestModels(t *testing.T) {
	oc, _ := newTestServer(t, "")
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodGet, "/v1/models", "", map[string]string{"Authorization": "Bearer secret"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "adina-analysis") || !strings.Contains(body, "adina-execution") {
		t.Fatalf("missing public models: %s", body)
	}
	if strings.Contains(body, `"id":"plan"`) || strings.Contains(body, `"id":"build"`) {
		t.Fatalf("exposed raw agents: %s", body)
	}
}

func TestChatValidationDoesNotTouchLedger(t *testing.T) {
	oc, _ := newTestServer(t, "")
	handler, led := newGateway(t, oc.URL, "")
	cases := []struct {
		name, body string
		headers    map[string]string
		status     int
	}{
		{"missing model", `{"messages":[{"role":"user","content":"hi"}]}`, authHeaders(), 400},
		{"unknown model", `{"model":"nope","messages":[{"role":"user","content":"hi"}]}`, authHeaders(), 404},
		{"missing messages", `{"model":"adina-analysis"}`, authHeaders(), 400},
		{"empty messages", `{"model":"adina-analysis","messages":[]}`, authHeaders(), 400},
		{"final assistant", `{"model":"adina-analysis","messages":[{"role":"user","content":"old"},{"role":"assistant","content":"done"}]}`, authHeaders(), 400},
		{"final tool", `{"model":"adina-analysis","messages":[{"role":"tool","content":"x"}]}`, authHeaders(), 400},
		{"tools", `{"model":"adina-analysis","tools":[],"messages":[{"role":"user","content":"hi"}]}`, authHeaders(), 400},
		{"tool_choice", `{"model":"adina-analysis","tool_choice":"auto","messages":[{"role":"user","content":"hi"}]}`, authHeaders(), 400},
		{"json_object", `{"model":"adina-analysis","response_format":{"type":"json_object"},"messages":[{"role":"user","content":"hi"}]}`, authHeaders(), 400},
		{"json_schema", `{"model":"adina-analysis","response_format":{"type":"json_schema"},"messages":[{"role":"user","content":"hi"}]}`, authHeaders(), 400},
		{"n greater than one", `{"model":"adina-analysis","n":2,"messages":[{"role":"user","content":"hi"}]}`, authHeaders(), 400},
		{"logprobs true", `{"model":"adina-analysis","logprobs":true,"messages":[{"role":"user","content":"hi"}]}`, authHeaders(), 400},
		{"bad stream options", `{"model":"adina-analysis","stream":true,"stream_options":{"bad":true},"messages":[{"role":"user","content":"hi"}]}`, authHeaders(), 400},
		{"multimodal", `{"model":"adina-analysis","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x"}}]}]}`, authHeaders(), 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := doReq(handler, http.MethodPost, "/v1/chat/completions", tc.body, tc.headers).Code; got != tc.status {
				t.Fatalf("status = %d", got)
			}
			count, _ := led.Count(context.Background())
			if count != 0 {
				t.Fatalf("ledger touched, count=%d", count)
			}
		})
	}
	noUser := authHeaders()
	delete(noUser, "X-OpenWebUI-User-Id")
	if got := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, noUser).Code; got != 400 {
		t.Fatalf("missing user status=%d", got)
	}
	noChat := authHeaders()
	delete(noChat, "X-OpenWebUI-Chat-Id")
	if got := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, noChat).Code; got != 400 {
		t.Fatalf("missing chat status=%d", got)
	}
}

func TestChatLedgerAndTranslation(t *testing.T) {
	oc, state := newTestServer(t, "")
	handler, led := newGateway(t, oc.URL, "")
	body := `{"model":"adina-analysis","messages":[{"role":"system","content":"system"},{"role":"user","content":"old"},{"role":"assistant","content":"old answer"},{"role":"user","content":"latest"}]}`
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", body, authHeaders())
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	w = doReq(handler, http.MethodPost, "/v1/chat/completions", body, authHeaders())
	if w.Code != http.StatusOK {
		t.Fatalf("reuse status=%d", w.Code)
	}
	if state.sessions != 1 {
		t.Fatalf("sessions=%d", state.sessions)
	}
	if len(state.messages) != 2 {
		t.Fatalf("messages=%d", len(state.messages))
	}
	if state.messages[0]["agent"] != "plan" {
		t.Fatalf("agent=%v", state.messages[0]["agent"])
	}
	parts := state.messages[0]["parts"].([]any)
	if parts[0].(map[string]any)["text"] != "latest" {
		t.Fatalf("replayed history: %#v", parts)
	}
	count, _ := led.Count(context.Background())
	if count != 1 {
		t.Fatalf("ledger count=%d", count)
	}
	if strings.Contains(w.Body.String(), "hidden") || !strings.Contains(w.Body.String(), "hello world") {
		t.Fatalf("bad flattening: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "usage") {
		t.Fatalf("fabricated usage: %s", w.Body.String())
	}

	execBody := strings.Replace(body, "adina-analysis", "adina-execution", 1)
	if got := doReq(handler, http.MethodPost, "/v1/chat/completions", execBody, authHeaders()).Code; got != http.StatusOK {
		t.Fatalf("execution status=%d", got)
	}
	if state.messages[2]["agent"] != "build" {
		t.Fatalf("agent=%v", state.messages[2]["agent"])
	}
	otherUser := authHeaders()
	otherUser["X-OpenWebUI-User-Id"] = "user-2"
	if got := doReq(handler, http.MethodPost, "/v1/chat/completions", body, otherUser).Code; got != http.StatusOK {
		t.Fatalf("other user status=%d", got)
	}
	if state.sessions != 3 {
		t.Fatalf("sessions after partitions=%d", state.sessions)
	}
}

func TestNonStreamingVariants(t *testing.T) {
	oc, _ := newTestServer(t, "")
	handler, _ := newGateway(t, oc.URL, "")
	for _, body := range []string{
		`{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`,
		`{"model":"adina-analysis","stream":false,"messages":[{"role":"user","content":"hi"}]}`,
	} {
		w := doReq(handler, http.MethodPost, "/v1/chat/completions", body, authHeaders())
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"object":"chat.completion"`) || !strings.Contains(w.Body.String(), `"content":"hello world"`) {
			t.Fatalf("unexpected non-streaming response: %s", w.Body.String())
		}
	}
}

func TestStreamingUsesEventAndPromptAsync(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.eventBody = streamEvents(
		`{"type":"server.connected"}`,
		`{"type":"session.idle","properties":{"sessionID":"ses_test_1"}}`,
		`{"type":"message.part.delta","properties":{"sessionID":"ses_other","field":"text","delta":"leak"}}`,
		`{"type":"session.status","properties":{"sessionID":"ses_test_1","status":{"type":"busy"}}}`,
		`{"type":"message.part.delta","properties":{"sessionID":"ses_test_1","messageID":"msg_1","field":"text","delta":"hello"}}`,
		`{"type":"session.idle","properties":{"sessionID":"ses_test_1"}}`,
	)
	handler, _ := newGateway(t, oc.URL, "")
	body := `{"model":"adina-analysis","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", body, authHeaders())
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("content-type=%q", got)
	}
	bodyText := w.Body.String()
	for _, want := range []string{"chat.completion.chunk", `"role":"assistant"`, `"content":"hello"`, `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("missing %q in %s", want, bodyText)
		}
	}
	if strings.Contains(bodyText, "leak") || strings.Contains(bodyText, "hello world") {
		t.Fatalf("stream exposed unrelated or synchronous content: %s", bodyText)
	}
	if state.eventCalls != 1 || state.promptAsync != 1 || len(state.messages) != 0 {
		t.Fatalf("bad streaming endpoints: event=%d prompt_async=%d message=%d", state.eventCalls, state.promptAsync, len(state.messages))
	}
	if state.getMessages != 1 {
		t.Fatalf("include_usage requested one final message fetch, got %d", state.getMessages)
	}
}

func TestStreamOptionsIncludeUsageEmitsUsageFromFinalMessageTokens(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.eventBody = streamEvents(
		`{"type":"server.connected"}`,
		`{"type":"message.part.delta","properties":{"sessionID":"ses_test_1","field":"text","delta":"hello"}}`,
		`{"type":"session.idle","properties":{"sessionID":"ses_test_1"}}`,
	)
	state.getBody = `[{"info":{"id":"msg_1","sessionID":"ses_test_1","tokens":{"input":11,"output":7,"total":20,"reasoning":2}},"parts":[]}]`
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"choices":[]`, `"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":20,"completion_tokens_details":{"reasoning_tokens":2}}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	if strings.Index(body, `"usage"`) > strings.Index(body, "data: [DONE]") {
		t.Fatalf("usage emitted after DONE: %s", body)
	}
	if state.getMessages != 1 || len(state.messages) != 0 {
		t.Fatalf("bad message calls: get=%d post=%d", state.getMessages, len(state.messages))
	}
}

func TestStreamOptionsIncludeUsageUnavailableEmitsNoUsage(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.eventBody = streamEvents(
		`{"type":"server.connected"}`,
		`{"type":"message.part.delta","properties":{"sessionID":"ses_test_1","field":"text","delta":"hello"}}`,
		`{"type":"session.idle","properties":{"sessionID":"ses_test_1"}}`,
	)
	state.getBody = `[{"info":{"id":"msg_1","sessionID":"ses_test_1","tokens":{"input":11}},"parts":[]}]`
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"usage"`) || strings.Contains(w.Body.String(), `"prompt_tokens":0`) {
		t.Fatalf("fabricated usage: %s", w.Body.String())
	}
	if state.getMessages != 1 {
		t.Fatalf("expected final fetch when usage requested, got %d", state.getMessages)
	}
}

func TestStreamOptionsIncludeUsageFalseEmitsNoUsageAndNoFinalFetch(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.eventBody = streamEvents(
		`{"type":"server.connected"}`,
		`{"type":"message.part.delta","properties":{"sessionID":"ses_test_1","field":"text","delta":"hello"}}`,
		`{"type":"session.idle","properties":{"sessionID":"ses_test_1"}}`,
	)
	state.getBody = `[{"info":{"tokens":{"input":1,"output":2,"total":3}},"parts":[]}]`
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"stream_options":{"include_usage":false},"messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"usage"`) {
		t.Fatalf("unexpected usage: %s", w.Body.String())
	}
	if state.getMessages != 0 || len(state.messages) != 0 {
		t.Fatalf("unexpected message calls: get=%d post=%d", state.getMessages, len(state.messages))
	}
}

func TestStreamUsageTotalTokensFallbackIncludesReasoning(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.eventBody = streamEvents(
		`{"type":"server.connected"}`,
		`{"type":"message.part.delta","properties":{"sessionID":"ses_test_1","field":"text","delta":"hello"}}`,
		`{"type":"session.idle","properties":{"sessionID":"ses_test_1"}}`,
	)
	state.getBody = `[{"info":{"tokens":{"input":4,"output":5,"reasoning":6}},"parts":[]}]`
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"total_tokens":15`) || !strings.Contains(w.Body.String(), `"reasoning_tokens":6`) {
		t.Fatalf("bad fallback usage: %s", w.Body.String())
	}
}

func TestStreamUsageFinalFetchFailureDoesNotBreakSuccessfulStream(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.getStatus = http.StatusInternalServerError
	state.eventBody = streamEvents(
		`{"type":"server.connected"}`,
		`{"type":"message.part.delta","properties":{"sessionID":"ses_test_1","field":"text","delta":"hello"}}`,
		`{"type":"session.idle","properties":{"sessionID":"ses_test_1"}}`,
	)
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "data: [DONE]") {
		t.Fatalf("stream broke after usage fetch failure: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"usage"`) {
		t.Fatalf("fabricated usage after fetch failure: %s", w.Body.String())
	}
}

func TestStreamingHandlesOpenCodeEventUnavailable(t *testing.T) {
	oc, _ := newTestServer(t, "")
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "opencode_bad_gateway") {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
}

func TestStreamingHandlesPromptAsyncFailure(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusInternalServerError
	state.eventBody = streamEvents(`{"type":"server.connected"}`)
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "opencode_bad_gateway") {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	if state.promptAsync != 1 || len(state.messages) != 0 {
		t.Fatalf("bad calls after prompt failure: prompt=%d message=%d", state.promptAsync, len(state.messages))
	}
}

func TestStreamingMalformedEventJSONReturnsControlledError(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.eventBody = streamEvents(`{"type":"server.connected"}`, `{not-json-with-secret-Bearer secret}`)
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"sensitive prompt"}]}`, authHeaders())
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "data: [DONE]") {
		t.Fatalf("unexpected streamed malformed-json response: %d %s", w.Code, w.Body.String())
	}
	for _, forbidden := range []string{"Bearer secret", "sensitive prompt", "not-json-with-secret"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, w.Body.String())
		}
	}
}

func TestStreamingTimeoutBeforeHeadersReturnsSanitizedError(t *testing.T) {
	t.Setenv("STREAM_TIMEOUT_SECONDS", "1")
	oc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ses_timeout","title":"test"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer oc.Close()
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"secret prompt before timeout"}]}`, authHeaders())
	if w.Code != http.StatusGatewayTimeout || !strings.Contains(w.Body.String(), "opencode_timeout") {
		t.Fatalf("unexpected timeout response: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret prompt") {
		t.Fatalf("timeout response leaked prompt: %s", w.Body.String())
	}
}

func TestStreamingTimeoutAfterHeadersEmitsSanitizedDone(t *testing.T) {
	t.Setenv("STREAM_TIMEOUT_SECONDS", "1")
	oc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ses_timeout","title":"test"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"server.connected\"}\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/prompt_async"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer oc.Close()
	handler, _ := newGateway(t, oc.URL, "")
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"secret prompt after timeout"}]}`, authHeaders())
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "data: [DONE]") || !strings.Contains(w.Body.String(), "OpenCode streaming stopped before completion") {
		t.Fatalf("unexpected streamed timeout response: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret prompt") {
		t.Fatalf("streamed timeout leaked prompt: %s", w.Body.String())
	}
}

func TestStreamingCancellationReleasesSessionLock(t *testing.T) {
	eventStarted := make(chan struct{})
	releaseEvent := make(chan struct{})
	var sessions atomic.Int32
	oc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			sessions.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ses_cancel","title":"test"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"server.connected\"}\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-eventStarted:
			default:
				close(eventStarted)
			}
			<-releaseEvent
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/prompt_async"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer oc.Close()
	handler, _ := newGateway(t, oc.URL, "")
	gateway := httptest.NewServer(handler)
	defer gateway.Close()

	body := `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+"/v1/chat/completions", strings.NewReader(body))
	for k, v := range authHeaders() {
		req.Header.Set(k, v)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			_, _ = io.ReadAll(resp.Body)
		}
	}()
	select {
	case <-eventStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not start")
	}
	cancel()
	close(releaseEvent)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled stream did not finish")
	}

	// Reuse the same handler/session: if the lock was not released this request returns session_busy.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w := doReq(handler, http.MethodPost, "/v1/chat/completions", body, authHeaders())
		if w.Code != http.StatusConflict && !strings.Contains(w.Body.String(), "session_busy") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("lock was not released after cancellation")
}

func TestStreamingClientContextCancellation(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.eventBody = streamEvents(`{"type":"server.connected"}`)
	handler, _ := newGateway(t, oc.URL, "")
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	for k, v := range authHeaders() {
		req.Header.Set(k, v)
	}
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req.WithContext(ctx))
	if w.Code != http.StatusGatewayTimeout && w.Code != http.StatusServiceUnavailable && w.Code != http.StatusBadGateway {
		t.Fatalf("unexpected cancellation response: %d %s", w.Code, w.Body.String())
	}
}

func TestStreamingSameSessionBusy(t *testing.T) {
	eventStarted := make(chan struct{})
	releaseEvent := make(chan struct{})
	var prompts atomic.Int32
	oc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ses_busy","title":"test"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"server.connected\"}\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-eventStarted:
			default:
				close(eventStarted)
			}
			<-releaseEvent
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/prompt_async"):
			prompts.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer oc.Close()
	handler, _ := newGateway(t, oc.URL, "")
	gateway := httptest.NewServer(handler)
	defer gateway.Close()

	body := `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		req, _ := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", strings.NewReader(body))
		for k, v := range authHeaders() {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		<-releaseEvent
	}()

	select {
	case <-eventStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first stream did not start")
	}
	for prompts.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}

	req, _ := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", strings.NewReader(body))
	for k, v := range authHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 session_busy, got %d", resp.StatusCode)
	}
	close(releaseEvent)
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first stream did not finish after release")
	}
}

func streamEvents(payloads ...string) string {
	var b strings.Builder
	for _, payload := range payloads {
		b.WriteString("data: ")
		b.WriteString(payload)
		b.WriteString("\n\n")
	}
	return b.String()
}

func TestDownstreamAuthorization(t *testing.T) {
	oc, state := newTestServer(t, "")
	handler, _ := newGateway(t, oc.URL, "")
	_ = doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	for _, h := range state.authorizations {
		if h != "" {
			t.Fatalf("unsecured downstream sent authorization %q", h)
		}
	}

	oc2, state2 := newTestServer(t, "pw")
	handler2, _ := newGateway(t, oc2.URL, "pw")
	_ = doReq(handler2, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("opencode:pw"))
	found := false
	for _, h := range state2.authorizations {
		if h == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("basic auth not sent, got %#v", state2.authorizations)
	}
}

func TestStreamingDownstreamAuthorization(t *testing.T) {
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.getBody = `[{"info":{"tokens":{"input":1,"output":2,"total":3}},"parts":[]}]`
	state.eventBody = streamEvents(
		`{"type":"server.connected"}`,
		`{"type":"message.part.delta","properties":{"sessionID":"ses_test_1","field":"text","delta":"hello"}}`,
		`{"type":"session.idle","properties":{"sessionID":"ses_test_1"}}`,
	)
	handler, _ := newGateway(t, oc.URL, "")
	_ = doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	for _, h := range state.authorizations {
		if h != "" {
			t.Fatalf("unsecured streaming downstream sent authorization %q", h)
		}
	}

	oc2, state2 := newTestServer(t, "pw")
	state2.eventStatus = http.StatusOK
	state2.promptStatus = http.StatusNoContent
	state2.eventBody = streamEvents(
		`{"type":"server.connected"}`,
		`{"type":"message.part.delta","properties":{"sessionID":"ses_test_1","field":"text","delta":"hello"}}`,
		`{"type":"session.idle","properties":{"sessionID":"ses_test_1"}}`,
	)
	handler2, _ := newGateway(t, oc2.URL, "pw")
	_ = doReq(handler2, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"hi"}]}`, authHeaders())
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("opencode:pw"))
	for _, h := range state2.authorizations {
		if h != want {
			t.Fatalf("expected only configured Basic Auth downstream, got %#v", state2.authorizations)
		}
	}
}

func TestSessionBusyErrorIsSanitized(t *testing.T) {
	eventStarted := make(chan struct{})
	releaseEvent := make(chan struct{})
	var prompts atomic.Int32
	oc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ses_busy_secure","title":"test"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"server.connected\"}\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-eventStarted:
			default:
				close(eventStarted)
			}
			<-releaseEvent
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/prompt_async"):
			prompts.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer oc.Close()
	handler, _ := newGateway(t, oc.URL, "")
	gateway := httptest.NewServer(handler)
	defer gateway.Close()
	body := `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"sensitive prompt"}]}`
	go func() {
		req, _ := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", strings.NewReader(body))
		for k, v := range authHeaders() {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			<-releaseEvent
		}
	}()
	select {
	case <-eventStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first stream did not start")
	}
	for prompts.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	req, _ := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", strings.NewReader(body))
	for k, v := range authHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	close(releaseEvent)
	bodyText := string(data)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(bodyText, "session_busy") {
		t.Fatalf("unexpected busy response: %d %s", resp.StatusCode, bodyText)
	}
	for _, forbidden := range []string{"Bearer", "Basic", "sensitive prompt", "ses_busy_secure"} {
		if strings.Contains(bodyText, forbidden) {
			t.Fatalf("session_busy response leaked %q: %s", forbidden, bodyText)
		}
	}
}

func TestStreamLogsSanitizedErrors(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	led, err := ledger.Open(t.TempDir() + "/gateway.sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	defer led.Close()
	oc, state := newTestServer(t, "")
	state.eventStatus = http.StatusOK
	state.promptStatus = http.StatusNoContent
	state.eventBody = streamEvents(`{"type":"server.connected"}`, `{not-json Bearer secret full prompt patch}`)
	handler := httpapi.New(auth.NewValidator("secret"), false, opencode.New(oc.URL, "opencode", "", time.Second), led, logger)
	w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","stream":true,"messages":[{"role":"user","content":"full prompt"}]}`, authHeaders())
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	logText := logs.String()
	for _, forbidden := range []string{"Bearer secret", "full prompt", "not-json", "patch"} {
		if strings.Contains(logText, forbidden) {
			t.Fatalf("logs leaked %q: %s", forbidden, logText)
		}
	}
}

func TestOpenCodeErrorHandling(t *testing.T) {
	t.Run("info error", func(t *testing.T) {
		oc, state := newTestServer(t, "")
		state.messageBody = `{"info":{"id":"msg_test","sessionID":"ses_test","role":"assistant","agent":"plan","error":{"name":"UnknownError","data":{"message":"Executable not found"}}},"parts":[]}`
		handler, _ := newGateway(t, oc.URL, "")
		w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, authHeaders())
		if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "opencode_execution_error") {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("empty parts", func(t *testing.T) {
		oc, state := newTestServer(t, "")
		state.messageBody = `{"info":{"id":"msg_test","sessionID":"ses_test","role":"assistant","agent":"plan"},"parts":[]}`
		handler, _ := newGateway(t, oc.URL, "")
		w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, authHeaders())
		if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "opencode_no_text_content") {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("auth failed", func(t *testing.T) {
		oc, state := newTestServer(t, "")
		state.messageStatus = http.StatusUnauthorized
		state.messageBody = `{}`
		handler, _ := newGateway(t, oc.URL, "")
		w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, authHeaders())
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "opencode_auth_failed") {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("timeout", func(t *testing.T) {
		oc, state := newTestServer(t, "")
		state.sleep = 50 * time.Millisecond
		handler, _ := newGatewayWithTimeout(t, oc.URL, "", 1*time.Millisecond)
		w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, authHeaders())
		if w.Code != http.StatusGatewayTimeout || !strings.Contains(w.Body.String(), "opencode_timeout") {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		handler, _ := newGateway(t, "http://127.0.0.1:1", "")
		w := doReq(handler, http.MethodPost, "/v1/chat/completions", `{"model":"adina-analysis","messages":[{"role":"user","content":"hi"}]}`, authHeaders())
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "opencode_unavailable") {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
	})
}
