package streaming

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/adina/opencode-webui-gateway/internal/opencode"
)

var ErrSessionBusy = errors.New("session_busy")

type ActionKind string

const (
	ActionConnected ActionKind = "connected"
	ActionIgnore    ActionKind = "ignore"
	ActionTextDelta ActionKind = "assistant_text_delta"
	ActionProgress  ActionKind = "safe_progress_markdown"
	ActionComplete  ActionKind = "complete"
	ActionError     ActionKind = "controlled_error"
)

type Action struct {
	Kind ActionKind
	Text string
	Err  error
}

type Mapper struct {
	sessionID       string
	promptSubmitted bool
	targetActivity  bool
	diffProgress    bool
	progressSeen    map[string]bool
	progressCount   int
	maxProgress     int
}

func NewMapper(sessionID string) *Mapper {
	return &Mapper{sessionID: sessionID, progressSeen: make(map[string]bool), maxProgress: 4}
}

func (m *Mapper) MarkPromptSubmitted() {
	m.promptSubmitted = true
}

func (m *Mapper) ConfirmFinalMessage(ok bool, err error) Action {
	if err != nil {
		return Action{Kind: ActionError, Err: err}
	}
	if ok && m.targetActivity {
		return Action{Kind: ActionComplete}
	}
	return Action{Kind: ActionIgnore}
}

func (m *Mapper) Handle(event opencode.Event) Action {
	switch event.Type {
	case "server.connected":
		return Action{Kind: ActionConnected}
	case "server.heartbeat":
		return Action{Kind: ActionIgnore}
	}

	payload, err := decodePayload(event)
	if err != nil {
		return Action{Kind: ActionError, Err: err}
	}
	props, ok := objectField(payload, "properties")
	if !ok || stringValue(props, "sessionID") != m.sessionID {
		return Action{Kind: ActionIgnore}
	}

	switch event.Type {
	case "session.status", "session.idle":
		status := sessionStatus(event.Type, props)
		if status == "busy" && m.promptSubmitted {
			m.targetActivity = true
			return Action{Kind: ActionIgnore}
		}
		if status == "idle" && m.promptSubmitted && m.targetActivity {
			return Action{Kind: ActionComplete}
		}
		return Action{Kind: ActionIgnore}
	case "message.part.delta":
		if stringValue(props, "field") != "text" {
			return Action{Kind: ActionIgnore}
		}
		delta := deltaText(payload, props)
		if delta == "" {
			return Action{Kind: ActionIgnore}
		}
		m.targetActivity = true
		return Action{Kind: ActionTextDelta, Text: delta}
	case "message.part.updated":
		progress := m.progressForPartUpdated(payload, props)
		if progress == "" {
			return Action{Kind: ActionIgnore}
		}
		m.targetActivity = true
		return Action{Kind: ActionProgress, Text: progress}
	case "session.diff":
		if m.diffProgress {
			return Action{Kind: ActionIgnore}
		}
		progress := m.dedupeProgress("\n\n_Workspace diff updated._\n\n")
		if progress == "" {
			return Action{Kind: ActionIgnore}
		}
		m.diffProgress = true
		m.targetActivity = true
		return Action{Kind: ActionProgress, Text: progress}
	default:
		return Action{Kind: ActionIgnore}
	}
}

func (m *Mapper) progressForPartUpdated(payload map[string]any, props map[string]any) string {
	partType := strings.ToLower(stringValue(props, "partType"))
	if partType == "" {
		if part, ok := objectField(props, "part"); ok {
			partType = strings.ToLower(stringValue(part, "type"))
		}
	}
	if partType == "" {
		if part, ok := objectField(payload, "part"); ok {
			partType = strings.ToLower(stringValue(part, "type"))
		}
	}
	progress := "\n\n_Thinking..._\n\n"
	toolName := strings.ToLower(stringValue(props, "tool"))
	if toolName == "" {
		if part, ok := objectField(props, "part"); ok {
			toolName = strings.ToLower(stringValue(part, "tool"))
			if toolName == "" {
				toolName = strings.ToLower(stringValue(part, "toolName"))
			}
		}
	}
	if toolName == "bash" || strings.Contains(partType, "bash") || strings.Contains(partType, "tool") {
		progress = "\n\n_Running tool: bash..._\n\n"
	}
	if strings.Contains(partType, "edit") || strings.Contains(partType, "file") || strings.Contains(partType, "write") {
		progress = "\n\n_Editing files..._\n\n"
	}
	if strings.Contains(partType, "patch") {
		progress = "\n\n_Applying patch..._\n\n"
	}
	return m.dedupeProgress(progress)
}

func (m *Mapper) dedupeProgress(progress string) string {
	if m.progressSeen[progress] || m.progressCount >= m.maxProgress {
		return ""
	}
	m.progressSeen[progress] = true
	m.progressCount++
	return progress
}

func decodePayload(event opencode.Event) (map[string]any, error) {
	var payload map[string]any
	if len(event.Data) == 0 {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func sessionStatus(eventType string, props map[string]any) string {
	if eventType == "session.idle" {
		return "idle"
	}
	if status := strings.ToLower(stringValue(props, "status")); status != "" {
		return status
	}
	if status, ok := objectField(props, "status"); ok {
		return strings.ToLower(stringValue(status, "type"))
	}
	return strings.ToLower(stringValue(props, "type"))
}

func deltaText(payload map[string]any, props map[string]any) string {
	for _, source := range []map[string]any{props, payload} {
		for _, key := range []string{"delta", "text", "content"} {
			if value := stringValue(source, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func objectField(payload map[string]any, key string) (map[string]any, bool) {
	value, ok := payload[key].(map[string]any)
	return value, ok
}

func stringValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

type InFlightLocks struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

func NewInFlightLocks() *InFlightLocks {
	return &InFlightLocks{keys: make(map[string]struct{})}
}

func (l *InFlightLocks) Acquire(ctx context.Context, key string) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	l.mu.Lock()
	if _, ok := l.keys[key]; ok {
		l.mu.Unlock()
		return nil, ErrSessionBusy
	}
	l.keys[key] = struct{}{}
	l.mu.Unlock()

	var once sync.Once
	done := make(chan struct{})
	release := func() {
		once.Do(func() {
			l.mu.Lock()
			delete(l.keys, key)
			l.mu.Unlock()
			close(done)
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			release()
		case <-done:
		}
	}()
	return release, nil
}

func (l *InFlightLocks) Run(ctx context.Context, key string, fn func() error) error {
	release, err := l.Acquire(ctx, key)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}
