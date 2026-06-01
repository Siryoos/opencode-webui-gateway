package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adina/opencode-webui-gateway/internal/auth"
	"github.com/adina/opencode-webui-gateway/internal/config"
	"github.com/adina/opencode-webui-gateway/internal/httpapi"
	"github.com/adina/opencode-webui-gateway/internal/ledger"
	"github.com/adina/opencode-webui-gateway/internal/logging"
	"github.com/adina/opencode-webui-gateway/internal/opencode"
)

func main() {
	logger := logging.New()
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err.Error())
		os.Exit(1)
	}
	if cfg.OpenCodePassword == "" {
		logger.Warn("OpenCode is unsecured; this is only allowed for local development")
	}
	led, err := ledger.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("failed to open SQLite ledger", "error", err.Error())
		os.Exit(1)
	}
	defer led.Close()
	oc := opencode.New(cfg.OpenCodeBaseURL, cfg.OpenCodeUsername, cfg.OpenCodePassword, cfg.RequestTimeout)
	server := &http.Server{Addr: ":8080", Handler: httpapi.New(auth.NewValidator(cfg.GatewayAPIKey), cfg.RequireAuthOnHealth, oc, led, logger), ReadHeaderTimeout: 5 * time.Second}

	go func() {
		logger.Info("gateway listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err.Error())
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
