package streaming

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/adina/opencode-webui-gateway/internal/opencode"
)

func TestEventMapperFiltersOutOtherSessions(t *testing.T) {
	mapper := NewMapper("ses_target")
	mapper.MarkPromptSubmitted()
	action := mapper.Handle(event("message.part.delta", `"sessionID":"ses_other","field":"text","delta":"leak"`))
	if action.Kind != ActionIgnore {
		t.Fatalf("expected ignore, got %+v", action)
	}
}

func TestEventMapperIgnoresServerConnectedAsUserContent(t *testing.T) {
	mapper := NewMapper("ses_target")
	action := mapper.Handle(opencode.Event{Type: "server.connected", Data: []byte(`{"type":"server.connected"}`)})
	if action.Kind != ActionConnected || action.Text != "" {
		t.Fatalf("expected connected without content, got %+v", action)
	}
}

func TestEventMapperIgnoresHeartbeat(t *testing.T) {
	mapper := NewMapper("ses_target")
	action := mapper.Handle(opencode.Event{Type: "server.heartbeat", Data: []byte(`{"type":"server.heartbeat"}`)})
	if action.Kind != ActionIgnore {
		t.Fatalf("expected ignore, got %+v", action)
	}
}

func TestCompletionDoesNotCompleteOnPrePromptIdle(t *testing.T) {
	mapper := NewMapper("ses_target")
	action := mapper.Handle(event("session.idle", `"sessionID":"ses_target"`))
	if action.Kind != ActionIgnore {
		t.Fatalf("expected pre-prompt idle ignored, got %+v", action)
	}
}

func TestCompletionOnIdleOnlyAfterTargetSessionActivity(t *testing.T) {
	mapper := NewMapper("ses_target")
	mapper.MarkPromptSubmitted()
	if action := mapper.Handle(event("session.idle", `"sessionID":"ses_target"`)); action.Kind != ActionIgnore {
		t.Fatalf("expected idle before activity ignored, got %+v", action)
	}
	if action := mapper.Handle(event("session.status", `"sessionID":"ses_target","status":{"type":"busy"}`)); action.Kind != ActionIgnore {
		t.Fatalf("expected busy status ignored as content, got %+v", action)
	}
	if action := mapper.Handle(event("session.idle", `"sessionID":"ses_target"`)); action.Kind != ActionComplete {
		t.Fatalf("expected complete after activity then idle, got %+v", action)
	}
}

func TestEventMapperMessagePartDeltaTextMapsToContentDelta(t *testing.T) {
	mapper := NewMapper("ses_target")
	mapper.MarkPromptSubmitted()
	action := mapper.Handle(event("message.part.delta", `"sessionID":"ses_target","messageID":"msg_1","field":"text","delta":"hello"`))
	if action.Kind != ActionTextDelta || action.Text != "hello" {
		t.Fatalf("expected text delta, got %+v", action)
	}
}

func TestEventMapperNonTextDeltaIgnored(t *testing.T) {
	mapper := NewMapper("ses_target")
	mapper.MarkPromptSubmitted()
	action := mapper.Handle(event("message.part.delta", `"sessionID":"ses_target","field":"metadata","delta":"hidden"`))
	if action.Kind != ActionIgnore {
		t.Fatalf("expected ignore, got %+v", action)
	}
}

func TestEventMapperUnknownEventIgnoredSafely(t *testing.T) {
	mapper := NewMapper("ses_target")
	action := mapper.Handle(event("unknown.event", `"sessionID":"ses_target","value":"ignored"`))
	if action.Kind != ActionIgnore {
		t.Fatalf("expected ignore, got %+v", action)
	}
}

func TestEventMapperSessionDiffProgressEmittedAtMostOnce(t *testing.T) {
	mapper := NewMapper("ses_target")
	mapper.MarkPromptSubmitted()
	first := mapper.Handle(event("session.diff", `"sessionID":"ses_target"`))
	second := mapper.Handle(event("session.diff", `"sessionID":"ses_target"`))
	if first.Kind != ActionProgress || first.Text != "\n\n_Workspace diff updated._\n\n" {
		t.Fatalf("expected diff progress, got %+v", first)
	}
	if second.Kind != ActionIgnore {
		t.Fatalf("expected duplicate diff ignored, got %+v", second)
	}
}

func TestProgressMessagesDeduplicatedAndRateLimited(t *testing.T) {
	mapper := NewMapper("ses_target")
	mapper.MarkPromptSubmitted()
	events := []opencode.Event{
		event("message.part.updated", `"sessionID":"ses_target","partType":"thinking"`),
		event("message.part.updated", `"sessionID":"ses_target","partType":"thinking"`),
		event("message.part.updated", `"sessionID":"ses_target","tool":"bash","partType":"tool"`),
		event("message.part.updated", `"sessionID":"ses_target","partType":"file-edit"`),
		event("message.part.updated", `"sessionID":"ses_target","partType":"patch"`),
		event("session.diff", `"sessionID":"ses_target"`),
	}
	var got []string
	for _, ev := range events {
		if action := mapper.Handle(ev); action.Kind == ActionProgress {
			got = append(got, action.Text)
		}
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 progress messages after dedupe/rate limit, got %d: %q", len(got), got)
	}
	for _, text := range got {
		if text == "hidden" || text == "raw output" || text == "" {
			t.Fatalf("unsafe progress text emitted: %q", text)
		}
	}
}

func TestBusyReturnedForConcurrentStreamLock(t *testing.T) {
	locks := NewInFlightLocks()
	release, err := locks.Acquire(context.Background(), "ses_target")
	if err != nil {
		t.Fatalf("first acquire returned error: %v", err)
	}
	defer release()
	_, err = locks.Acquire(context.Background(), "ses_target")
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("expected ErrSessionBusy, got %v", err)
	}
}

func TestLockReleasedAfterSuccess(t *testing.T) {
	locks := NewInFlightLocks()
	err := locks.Run(context.Background(), "ses_target", func() error { return nil })
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	assertCanAcquire(t, locks, "ses_target")
}

func TestLockReleasedAfterError(t *testing.T) {
	locks := NewInFlightLocks()
	want := errors.New("stream failed")
	err := locks.Run(context.Background(), "ses_target", func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("expected stream error, got %v", err)
	}
	assertCanAcquire(t, locks, "ses_target")
}

func TestLockReleasedAfterContextCancellation(t *testing.T) {
	locks := NewInFlightLocks()
	ctx, cancel := context.WithCancel(context.Background())
	_, err := locks.Acquire(ctx, "ses_target")
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if release, err := locks.Acquire(context.Background(), "ses_target"); err == nil {
			release()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("lock was not released after context cancellation")
}

func TestEventMapperFinalMessageHookCompletesAfterActivity(t *testing.T) {
	mapper := NewMapper("ses_target")
	mapper.MarkPromptSubmitted()
	_ = mapper.Handle(event("message.part.delta", `"sessionID":"ses_target","field":"text","delta":"hello"`))
	if action := mapper.ConfirmFinalMessage(true, nil); action.Kind != ActionComplete {
		t.Fatalf("expected final message hook completion, got %+v", action)
	}
	if action := mapper.ConfirmFinalMessage(false, nil); action.Kind != ActionIgnore {
		t.Fatalf("expected negative final message hook ignored, got %+v", action)
	}
	want := errors.New("fetch failed")
	if action := mapper.ConfirmFinalMessage(false, want); action.Kind != ActionError || !errors.Is(action.Err, want) {
		t.Fatalf("expected controlled hook error, got %+v", action)
	}
}

func event(eventType string, properties string) opencode.Event {
	data := fmt.Sprintf(`{"type":%q,"properties":{%s}}`, eventType, properties)
	return opencode.Event{Type: eventType, Data: []byte(data)}
}

func assertCanAcquire(t *testing.T, locks *InFlightLocks, key string) {
	t.Helper()
	release, err := locks.Acquire(context.Background(), key)
	if err != nil {
		t.Fatalf("expected lock to be released, got %v", err)
	}
	release()
}
