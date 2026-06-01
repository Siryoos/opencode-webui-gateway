package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/adina/opencode-webui-gateway/internal/auth"
	"github.com/adina/opencode-webui-gateway/internal/ledger"
	"github.com/adina/opencode-webui-gateway/internal/models"
	"github.com/adina/opencode-webui-gateway/internal/openai"
	"github.com/adina/opencode-webui-gateway/internal/opencode"
)

type Server struct {
	auth                auth.Validator
	requireAuthOnHealth bool
	oc                  *opencode.Client
	ledger              *ledger.Ledger
	logger              *slog.Logger
	modelCreated        int64
}

func New(authv auth.Validator, requireHealth bool, oc *opencode.Client, led *ledger.Ledger, logger *slog.Logger) http.Handler {
	s := &Server{auth: authv, requireAuthOnHealth: requireHealth, oc: oc, ledger: led, logger: logger, modelCreated: 1780324930}
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

	ocResp, err := s.oc.SendMessage(r.Context(), entry.OpenCodeSessionID, opencode.SendMessageRequest{Agent: route.Agent, System: system, Parts: []opencode.TextInput{{Type: "text", Text: latest}}})
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
	id := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	if req.Stream != nil && *req.Stream {
		s.writeSSE(w, id, created, req.Model, text)
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

func (s *Server) writeSSE(w http.ResponseWriter, id string, created int64, model string, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	stop := "stop"
	chunks := []openai.ChatCompletionChunk{
		{ID: id, Object: "chat.completion.chunk", Created: created, Model: model, Choices: []openai.ChunkChoice{{Index: 0, Delta: map[string]any{"role": "assistant"}, FinishReason: nil}}},
		{ID: id, Object: "chat.completion.chunk", Created: created, Model: model, Choices: []openai.ChunkChoice{{Index: 0, Delta: map[string]any{"content": content}, FinishReason: nil}}},
		{ID: id, Object: "chat.completion.chunk", Created: created, Model: model, Choices: []openai.ChunkChoice{{Index: 0, Delta: map[string]any{}, FinishReason: &stop}}},
	}
	flusher, _ := w.(http.Flusher)
	for _, chunk := range chunks {
		data, _ := json.Marshal(chunk)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
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

func writeError(w http.ResponseWriter, status int, message, typ string, param *string, code string) {
	writeJSON(w, status, openai.ErrorEnvelope{Error: openai.ErrorBody{Message: message, Type: typ, Param: param, Code: code}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
