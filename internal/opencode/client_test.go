package opencode

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPromptAsyncCallsEndpointAndExpects204(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(server.URL, "opencode", "", time.Second)
	err := client.PromptAsync(context.Background(), "ses_123", SendMessageRequest{Agent: "plan", Parts: []TextInput{{Type: "text", Text: "hi"}}})
	if err != nil {
		t.Fatalf("PromptAsync returned error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/session/ses_123/prompt_async" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
}

func TestPromptAsyncRejectsNon204(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(server.URL, "opencode", "", time.Second)
	err := client.PromptAsync(context.Background(), "ses_123", SendMessageRequest{Parts: []TextInput{{Type: "text", Text: "hi"}}})
	if !errors.Is(err, ErrBadGateway) {
		t.Fatalf("expected ErrBadGateway for non-204, got %v", err)
	}
}

func TestSubscribeEventsCallsEvent(t *testing.T) {
	var gotMethod, gotPath, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"server.connected\"}\n\n")
	}))
	defer server.Close()

	client := New(server.URL, "opencode", "", time.Second)
	stream, err := client.SubscribeEvents(context.Background())
	if err != nil {
		t.Fatalf("SubscribeEvents returned error: %v", err)
	}
	defer stream.Close()
	if gotMethod != http.MethodGet || gotPath != "/event" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("unexpected accept header %q", gotAccept)
	}
}

func TestAuthSentOnlyWhenPasswordConfigured(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantAuth bool
	}{
		{name: "without password", password: "", wantAuth: false},
		{name: "with password", password: "secret", wantAuth: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var auth string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			client := New(server.URL, "opencode", tt.password, time.Second)
			err := client.PromptAsync(context.Background(), "ses_123", SendMessageRequest{Parts: []TextInput{{Type: "text", Text: "hi"}}})
			if err != nil {
				t.Fatalf("PromptAsync returned error: %v", err)
			}
			if tt.wantAuth && !strings.HasPrefix(auth, "Basic ") {
				t.Fatalf("expected Basic Auth header, got %q", auth)
			}
			if !tt.wantAuth && auth != "" {
				t.Fatalf("expected no Authorization header, got %q", auth)
			}
		})
	}
}

func TestSubscribeEventsAuthSentOnlyWhenPasswordConfigured(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"server.connected\"}\n\n")
	}))
	defer server.Close()

	client := New(server.URL, "opencode", "", time.Second)
	stream, err := client.SubscribeEvents(context.Background())
	if err != nil {
		t.Fatalf("SubscribeEvents returned error: %v", err)
	}
	_ = stream.Close()
	if auth != "" {
		t.Fatalf("expected no Authorization header without password, got %q", auth)
	}

	client = New(server.URL, "opencode", "secret", time.Second)
	stream, err = client.SubscribeEvents(context.Background())
	if err != nil {
		t.Fatalf("SubscribeEvents returned error: %v", err)
	}
	_ = stream.Close()
	if !strings.HasPrefix(auth, "Basic ") {
		t.Fatalf("expected Basic Auth header with password, got %q", auth)
	}
}

func TestUpstreamBearerAuthorizationIsNotForwarded(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	ctx := context.WithValue(context.Background(), "Authorization", "Bearer upstream-token")
	client := New(server.URL, "opencode", "", time.Second)
	err := client.PromptAsync(ctx, "ses_123", SendMessageRequest{Parts: []TextInput{{Type: "text", Text: "hi"}}})
	if err != nil {
		t.Fatalf("PromptAsync returned error: %v", err)
	}
	if auth != "" {
		t.Fatalf("expected upstream bearer not to be forwarded, got %q", auth)
	}
}

func TestSSEFragmentedFramesParseCorrectly(t *testing.T) {
	reader := newSSEReader(strings.NewReader("data: {\"type\":\"message.part.delta\",\"sessionID\":\"ses_1\",\"messageID\":\"msg_1\"}\n\n"))
	event, err := reader.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if event.Type != "message.part.delta" || event.SessionID != "ses_1" || event.MessageID != "msg_1" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestSSEMultipleFramesPerReadParseCorrectly(t *testing.T) {
	reader := newSSEReader(strings.NewReader("data: {\"type\":\"one\"}\n\ndata: {\"type\":\"two\"}\n\n"))
	first, err := reader.Next()
	if err != nil {
		t.Fatalf("first Next returned error: %v", err)
	}
	second, err := reader.Next()
	if err != nil {
		t.Fatalf("second Next returned error: %v", err)
	}
	if first.Type != "one" || second.Type != "two" {
		t.Fatalf("unexpected events: %+v %+v", first, second)
	}
}

func TestSSEMultipleDataLinesAreJoined(t *testing.T) {
	reader := newSSEReader(strings.NewReader("data: {\"type\":\"joined\",\ndata: \"properties\":{\"sessionID\":\"ses_1\",\"messageID\":\"msg_1\"}}\n\n"))
	event, err := reader.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if event.Type != "joined" || event.SessionID != "ses_1" || event.MessageID != "msg_1" {
		t.Fatalf("unexpected joined event: %+v", event)
	}
}

func TestSSECommentsAndEmptyFramesAreIgnored(t *testing.T) {
	reader := newSSEReader(strings.NewReader(": keepalive\n\n\n\ndata: {\"type\":\"real\"}\n\n"))
	event, err := reader.Next()
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if event.Type != "real" {
		t.Fatalf("unexpected event type %q", event.Type)
	}
}

func TestSSEMalformedJSONReturnsControlledError(t *testing.T) {
	reader := newSSEReader(strings.NewReader("data: {not-json}\n\n"))
	_, err := reader.Next()
	if !errors.Is(err, ErrSSEMalformedJSON) {
		t.Fatalf("expected ErrSSEMalformedJSON, got %v", err)
	}
}

func TestSubscribeEventsContextCancellationStopsReading(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := New(server.URL, "opencode", "", 0)
	stream, err := client.SubscribeEvents(ctx)
	if err != nil {
		t.Fatalf("SubscribeEvents returned error: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := stream.Next()
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Next did not stop after context cancellation")
	}
}
