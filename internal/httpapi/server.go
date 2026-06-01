package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/adina/opencode-webui-gateway/internal/auth"
	"github.com/adina/opencode-webui-gateway/internal/ledger"
	"github.com/adina/opencode-webui-gateway/internal/models"
	"github.com/adina/opencode-webui-gateway/internal/openai"
	"github.com/adina/opencode-webui-gateway/internal/opencode"
	"github.com/adina/opencode-webui-gateway/internal/streaming"
)

const defaultStreamTimeout = 120 * time.Second

type Server struct {
	auth                auth.Validator
	requireAuthOnHealth bool
	oc                  *opencode.Client
	ledger              *ledger.Ledger
	logger              *slog.Logger
	modelCreated        int64
	streamLocks         *streaming.InFlightLocks
	streamTimeout       time.Duration
}

func New(authv auth.Validator, requireHealth bool, oc *opencode.Client, led *ledger.Ledger, logger *slog.Logger) http.Handler {
	s := &Server{auth: authv, requireAuthOnHealth: requireHealth, oc: oc, ledger: led, logger: logger, modelCreated: 1780324930, streamLocks: streaming.NewInFlightLocks(), streamTimeout: streamTimeout()}
	r := chi.NewRouter()
	r.Get("/health", s.health)
	r.With(s.requireAuth).Get("/v1/models", s.models)
	r.With(s.requireAuth).Post("/v1/chat/completions", s.chat)
	return r
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failure := s.auth.Validate(r); failure != nil {
			writeError(w, http.StatusUnauthorized, failure.Message, "authentication_error", nil, failure.Code)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if s.requireAuthOnHealth {
		if failure := s.auth.Validate(r); failure != nil {
			writeError(w, http.StatusUnauthorized, failure.Message, "authentication_error", nil, failure.Code)
			return
		}
	}
	health, err := s.oc.Health(r.Context())
	resp := map[string]any{"status": "ok", "gateway": map[string]string{"status": "ok"}, "opencode": map[string]any{"status": "ok", "healthy": health.Healthy, "version": health.Version}}
	status := http.StatusOK
	if err != nil || !health.Healthy {
		status = http.StatusServiceUnavailable
		resp["status"] = "degraded"
		resp["opencode"] = map[string]any{"status": "unreachable", "healthy": nil, "version": nil}
		if err == nil {
			resp["opencode"] = map[string]any{"status": "unhealthy", "healthy": health.Healthy, "version": health.Version}
		}
	}
	writeJSON(w, status, resp)
}

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	data := make([]openai.ModelInfo, 0, len(models.Routes))
	for _, route := range models.Routes {
		data = append(data, openai.ModelInfo{ID: route.PublicID, Object: "model", Created: s.modelCreated, OwnedBy: "opencode-webui-gateway"})
	}
	writeJSON(w, http.StatusOK, openai.ModelsResponse{Object: "list", Data: data})
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var req openai.ChatRequest
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request body", "invalid_request_error", nil, "invalid_json")
		return
	}
	userID := r.Header.Get("X-OpenWebUI-User-Id")
	if userID == "" {
		param := "X-OpenWebUI-User-Id"
		writeError(w, http.StatusBadRequest, "missing X-OpenWebUI-User-Id header", "invalid_request_error", &param, "missing_required_parameter")
		return
	}
	chatID := r.Header.Get("X-OpenWebUI-Chat-Id")
	if chatID == "" {
		param := "X-OpenWebUI-Chat-Id"
		writeError(w, http.StatusBadRequest, "missing X-OpenWebUI-Chat-Id header", "invalid_request_error", &param, "missing_required_parameter")
		return
	}
	system, latest, err := openai.ExtractLatest(req)
	if err != nil {
		if v, ok := openai.IsValidation(err); ok {
			writeError(w, v.Status, v.Message, v.Type, &v.Param, v.Code)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request", "invalid_request_error", nil, "invalid_request")
		return
	}
	route, ok := models.Resolve(req.Model)
	if !ok {
		param := "model"
		writeError(w, http.StatusNotFound, "model not found", "invalid_request_error", &param, "model_not_found")
		return
	}

	entry, _, err := s.ledger.ResolveOrCreate(r.Context(), userID, chatID, req.Model, func(ctx context.Context) (string, error) {
		session, err := s.oc.CreateSession(ctx, fmt.Sprintf("Open WebUI %s %s", req.Model, chatID))
		return session.ID, err
	})
	if err != nil {
		s.writeDownstreamError(w, err)
		return
	}
	reqBody := opencode.SendMessageRequest{Agent: route.Agent, System: system, Parts: []opencode.TextInput{{Type: "text", Text: latest}}}
	id := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	if req.Stream != nil && *req.Stream {
		s.streamChat(w, r, entry, req.Model, reqBody, id, created, includeUsage(req))
		return
	}

	ocResp, err := s.oc.SendMessage(r.Context(), entry.OpenCodeSessionID, reqBody)
	if err != nil {
		s.writeDownstreamError(w, err)
		return
	}
	text, ok := s.extractText(w, ocResp)
	if !ok {
		return
	}
	if err := s.ledger.Touch(r.Context(), entry.ID); err != nil {
		s.logger.Error("failed to refresh ledger timestamp", "ledger_id", entry.ID)
		writeError(w, http.StatusBadGateway, "gateway session ledger update failed", "server_error", nil, "opencode_bad_gateway")
		return
	}
	writeJSON(w, http.StatusOK, openai.ChatCompletion{
		ID:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   req.Model,
		Choices: []openai.Choice{{Index: 0, Message: openai.AssistantResult{Role: "assistant", Content: text}, FinishReason: "stop"}},
	})
}

func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, entry ledger.Entry, model string, req opencode.SendMessageRequest, id string, created int64, includeUsage bool) {
	ctx, cancel := context.WithTimeout(r.Context(), s.streamTimeout)
	defer cancel()
	release, err := s.streamLocks.Acquire(ctx, entry.OpenCodeSessionID)
	if err != nil {
		if errors.Is(err, streaming.ErrSessionBusy) {
			writeError(w, http.StatusConflict, "OpenCode session already has an active stream", "server_error", nil, "session_busy")
			return
		}
		s.writeDownstreamError(w, err)
		return
	}
	defer release()

	stream, err := s.oc.SubscribeEvents(ctx)
	if err != nil {
		s.writeDownstreamError(w, err)
		return
	}
	defer stream.Close()

	mapper := streaming.NewMapper(entry.OpenCodeSessionID)
	for {
		event, err := stream.Next()
		if err != nil {
			s.writeDownstreamError(w, err)
			return
		}
		action := mapper.Handle(event)
		if action.Kind == streaming.ActionConnected {
			break
		}
		if action.Kind == streaming.ActionError {
			s.writeDownstreamError(w, action.Err)
			return
		}
	}

	if err := s.oc.PromptAsync(ctx, entry.OpenCodeSessionID, req); err != nil {
		s.writeDownstreamError(w, err)
		return
	}
	mapper.MarkPromptSubmitted()

	w.Header().Set("Connection", "keep-alive")
	writer := openai.NewStreamWriter(w, id, created, model)
	w.WriteHeader(http.StatusOK)
	started := true
	if err := writer.WriteRole(); err != nil {
		s.logger.Error("stream role chunk write failed", "error", sanitizedError(err))
		return
	}
	for {
		event, err := stream.Next()
		if err != nil {
			s.finishStreamError(writer, started, err)
			return
		}
		action := mapper.Handle(event)
		switch action.Kind {
		case streaming.ActionTextDelta, streaming.ActionProgress:
			if err := writer.WriteTextDelta(action.Text); err != nil {
				s.logger.Error("stream delta chunk write failed", "error", sanitizedError(err))
				return
			}
		case streaming.ActionComplete:
			if err := writer.WriteStop(); err != nil {
				s.logger.Error("stream stop chunk write failed", "error", sanitizedError(err))
				return
			}
			if includeUsage {
				if usage, ok := s.fetchStreamUsage(ctx, entry.OpenCodeSessionID); ok {
					if err := writer.WriteUsage(usage); err != nil {
						s.logger.Error("stream usage chunk write failed", "error", sanitizedError(err))
						return
					}
				}
			}
			if err := writer.WriteDone(); err != nil {
				s.logger.Error("stream done write failed", "error", sanitizedError(err))
				return
			}
			if err := s.ledger.Touch(context.Background(), entry.ID); err != nil {
				s.logger.Error("failed to refresh ledger timestamp after stream", "ledger_id", entry.ID)
			}
			return
		case streaming.ActionError:
			s.finishStreamError(writer, started, action.Err)
			return
		}
	}
}

func (s *Server) fetchStreamUsage(ctx context.Context, sessionID string) (openai.Usage, bool) {
	messages, err := s.oc.ListMessages(ctx, sessionID)
	if err != nil {
		s.logger.Error("OpenCode final message fetch for usage failed", "error", sanitizedError(err))
		return openai.Usage{}, false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if usage, ok := openai.UsageFromOpenCodeTokens(messages[i].Info.Tokens); ok {
			return usage, true
		}
	}
	return openai.Usage{}, false
}

func (s *Server) finishStreamError(writer *openai.StreamWriter, started bool, err error) {
	if !started {
		return
	}
	s.logger.Error("OpenCode stream failed", "error", sanitizedError(err))
	_ = writer.WriteTextDelta("\n\n_OpenCode streaming stopped before completion._\n\n")
	_ = writer.WriteStop()
	_ = writer.WriteDone()
}

func (s *Server) extractText(w http.ResponseWriter, ocResp opencode.MessageResponse) (string, bool) {
	if ocResp.Info.Error != nil {
		writeError(w, http.StatusBadGateway, "OpenCode execution failed", "server_error", nil, "opencode_execution_error")
		return "", false
	}
	text := opencode.FlattenText(ocResp)
	if text == "" {
		writeError(w, http.StatusBadGateway, "OpenCode returned no usable text content", "server_error", nil, "opencode_no_text_content")
		return "", false
	}
	return text, true
}

func (s *Server) writeDownstreamError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, "OpenCode request timed out", "server_error", nil, "opencode_timeout")
		return
	}
	if errors.Is(err, opencode.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "OpenCode is unavailable", "server_error", nil, "opencode_unavailable")
		return
	}
	if errors.Is(err, opencode.ErrAuthFailed) {
		writeError(w, http.StatusServiceUnavailable, "OpenCode authentication failed", "server_error", nil, "opencode_auth_failed")
		return
	}
	writeError(w, http.StatusBadGateway, "OpenCode returned an unusable response", "server_error", nil, "opencode_bad_gateway")
}

func streamTimeout() time.Duration {
	value := os.Getenv("STREAM_TIMEOUT_SECONDS")
	if value == "" {
		return defaultStreamTimeout
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return defaultStreamTimeout
	}
	return time.Duration(seconds) * time.Second
}

func includeUsage(req openai.ChatRequest) bool {
	value, ok := req.StreamOptions["include_usage"]
	if !ok {
		return false
	}
	include, _ := value.(bool)
	return include
}

func sanitizedError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "context canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "context deadline exceeded"
	case errors.Is(err, opencode.ErrUnavailable):
		return "opencode unavailable"
	case errors.Is(err, opencode.ErrAuthFailed):
		return "opencode auth failed"
	case errors.Is(err, opencode.ErrBadGateway):
		return "opencode bad gateway"
	case errors.Is(err, opencode.ErrSSEMalformedJSON):
		return "opencode malformed event json"
	case errors.Is(err, io.EOF):
		return "opencode event stream closed"
	default:
		return "internal stream error"
	}
}

func writeError(w http.ResponseWriter, status int, message, typ string, param *string, code string) {
	writeJSON(w, status, openai.ErrorEnvelope{Error: openai.ErrorBody{Message: message, Type: typ, Param: param, Code: code}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
