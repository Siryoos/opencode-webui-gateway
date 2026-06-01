package opencode

import (
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
		var netErr net.Error
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			return context.DeadlineExceeded
		}
		return ErrUnavailable
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

func FlattenText(resp MessageResponse) string {
	var b strings.Builder
	for _, part := range resp.Parts {
		if part.Type == "text" {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}
