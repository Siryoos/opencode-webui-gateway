package httpapi_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

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
	promptAsync    int
	messageStatus  int
	messageBody    string
	sleep          time.Duration
}

func newTestServer(t *testing.T, password string) (*httptest.Server, *ocState) {
	t.Helper()
	state := &ocState{}
	state.messageStatus = http.StatusOK
	state.messageBody = `{"info":{"id":"msg_test","sessionID":"ses_test","role":"assistant","agent":"plan"},"parts":[{"type":"reasoning","text":"hidden"},{"type":"text","text":"hello"},{"type":"tool","text":"hidden"},{"type":"text","text":" world"}]}`
	oc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.authorizations = append(state.authorizations, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/event" || r.URL.Path == "/global/event":
			state.eventCalls++
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/global/health":
			_, _ = w.Write([]byte(`{"healthy":true,"version":"1.15.13"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/agent":
			_, _ = w.Write([]byte(`[{"name":"plan"},{"name":"build"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			state.sessions++
			_, _ = w.Write([]byte(`{"id":"ses_test_` + string(rune('0'+state.sessions)) + `","title":"test","version":"1.15.13","time":{"created":1,"updated":1}}`))
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
			http.NotFound(w, r)
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

func TestStreamingShim(t *testing.T) {
	oc, state := newTestServer(t, "")
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
	for _, want := range []string{"chat.completion.chunk", `"role":"assistant"`, `"content":"hello world"`, `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("missing %q in %s", want, bodyText)
		}
	}
	if state.eventCalls != 0 || state.promptAsync != 0 {
		t.Fatalf("used forbidden streaming endpoints: event=%d prompt_async=%d", state.eventCalls, state.promptAsync)
	}
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
