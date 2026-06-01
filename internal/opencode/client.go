package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("opencode unavailable")
var ErrBadGateway = errors.New("opencode bad gateway")
var ErrAuthFailed = errors.New("opencode auth failed")
var ErrSSEMalformedJSON = errors.New("opencode sse malformed json")
var ErrSSEFrameTooLarge = errors.New("opencode sse frame too large")

const maxSSEFrameBytes = 4 * 1024 * 1024

type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

type Health struct {
	Healthy bool   `json:"healthy"`
	Version string `json:"version"`
}

type Agent struct {
	Name string `json:"name"`
}

type Session struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Version   string         `json:"version"`
	Directory string         `json:"directory"`
	Tokens    map[string]any `json:"tokens"`
	Time      map[string]any `json:"time"`
}

type SendMessageRequest struct {
	Agent  string      `json:"agent"`
	System string      `json:"system,omitempty"`
	Parts  []TextInput `json:"parts"`
}

type TextInput struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MessageResponse struct {
	Info  AssistantInfo `json:"info"`
	Parts []Part        `json:"parts"`
}

type AssistantInfo struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"sessionID"`
	Role       string         `json:"role"`
	Agent      string         `json:"agent"`
	Finish     string         `json:"finish"`
	Tokens     map[string]any `json:"tokens"`
	Error      any            `json:"error,omitempty"`
	ProviderID string         `json:"providerID"`
	ModelID    string         `json:"modelID"`
}

type Part struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Event struct {
	Type      string
	SessionID string
	MessageID string
	Data      json.RawMessage
}

type EventStream struct {
	ctx    context.Context
	body   io.ReadCloser
	reader *sseReader
}

func New(baseURL, username, password string, timeout time.Duration) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), username: username, password: password, http: &http.Client{Timeout: timeout}}
}

func (c *Client) Health(ctx context.Context) (Health, error) {
	var out Health
	err := c.do(ctx, http.MethodGet, "/global/health", nil, &out)
	return out, err
}

func (c *Client) ListAgents(ctx context.Context) ([]Agent, error) {
	var out []Agent
	err := c.do(ctx, http.MethodGet, "/agent", nil, &out)
	return out, err
}

func (c *Client) CreateSession(ctx context.Context, title string) (Session, error) {
	var out Session
	err := c.do(ctx, http.MethodPost, "/session", map[string]string{"title": title}, &out)
	if err == nil && out.ID == "" {
		return Session{}, ErrBadGateway
	}
	return out, err
}

func (c *Client) SendMessage(ctx context.Context, sessionID string, req SendMessageRequest) (MessageResponse, error) {
	var out MessageResponse
	err := c.do(ctx, http.MethodPost, "/session/"+sessionID+"/message", req, &out)
	if err == nil && out.Info.ID == "" {
		return MessageResponse{}, ErrBadGateway
	}
	return out, err
}

func (c *Client) ListMessages(ctx context.Context, sessionID string) ([]MessageResponse, error) {
	var out []MessageResponse
	err := c.do(ctx, http.MethodGet, "/session/"+sessionID+"/message", nil, &out)
	return out, err
}

func (c *Client) PromptAsync(ctx context.Context, sessionID string, req SendMessageRequest) error {
	return c.doStatus(ctx, http.MethodPost, "/session/"+sessionID+"/prompt_async", req, http.StatusNoContent)
}

func (c *Client) SubscribeEvents(ctx context.Context) (*EventStream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/event", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, mapHTTPError(ctx, err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, ErrAuthFailed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d", ErrBadGateway, resp.StatusCode)
	}
	return &EventStream{ctx: ctx, body: resp.Body, reader: newSSEReader(resp.Body)}, nil
}

func (s *EventStream) Next() (Event, error) {
	select {
	case <-s.ctx.Done():
		return Event{}, s.ctx.Err()
	default:
	}
	event, err := s.reader.Next()
	if err != nil {
		if s.ctx.Err() != nil {
			return Event{}, s.ctx.Err()
		}
		return Event{}, err
	}
	return event, nil
}

func (s *EventStream) Close() error {
	return s.body.Close()
}

func (c *Client) doStatus(ctx context.Context, method, path string, body any, want int) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return mapHTTPError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrAuthFailed
	}
	if resp.StatusCode != want {
		return fmt.Errorf("%w: status %d", ErrBadGateway, resp.StatusCode)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return mapHTTPError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrAuthFailed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrBadGateway, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return ErrBadGateway
	}
	return nil
}

func mapHTTPError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return context.DeadlineExceeded
	}
	return ErrUnavailable
}

type sseReader struct {
	reader *bufio.Reader
}

func newSSEReader(reader io.Reader) *sseReader {
	return &sseReader{reader: bufio.NewReader(reader)}
}

func (r *sseReader) Next() (Event, error) {
	var dataLines []string
	var eventType string
	frameBytes := 0
	for {
		line, err := r.reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return Event{}, err
		}
		frameBytes += len(line)
		if frameBytes > maxSSEFrameBytes {
			return Event{}, ErrSSEFrameTooLarge
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			if len(dataLines) == 0 {
				frameBytes = 0
				if err != nil {
					return Event{}, err
				}
				continue
			}
			return parseSSEEvent(eventType, strings.Join(dataLines, "\n"))
		}
		if strings.HasPrefix(line, ":") {
			if err != nil {
				return Event{}, err
			}
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if ok && strings.HasPrefix(value, " ") {
			value = strings.TrimPrefix(value, " ")
		}
		if !ok {
			field = line
			value = ""
		}
		switch field {
		case "event":
			eventType = value
		case "data":
			dataLines = append(dataLines, value)
		}
		if err != nil {
			if len(dataLines) == 0 {
				return Event{}, err
			}
			return parseSSEEvent(eventType, strings.Join(dataLines, "\n"))
		}
	}
}

func parseSSEEvent(eventType, data string) (Event, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrSSEMalformedJSON, err)
	}
	if typ, ok := stringField(payload, "type"); ok {
		eventType = typ
	}
	sessionID, _ := stringField(payload, "sessionID")
	messageID, _ := stringField(payload, "messageID")
	if props, ok := payload["properties"].(map[string]any); ok {
		if sessionID == "" {
			sessionID, _ = stringField(props, "sessionID")
		}
		if messageID == "" {
			messageID, _ = stringField(props, "messageID")
		}
	}
	return Event{Type: eventType, SessionID: sessionID, MessageID: messageID, Data: json.RawMessage(data)}, nil
}

func stringField(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key].(string)
	return value, ok
}

func FlattenText(resp MessageResponse) string {
	var b strings.Builder
	for _, part := range resp.Parts {
		if part.Type == "text" {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}
