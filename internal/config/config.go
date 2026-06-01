package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	GatewayAPIKey          string
	OpenCodeBaseURL        string
	DatabasePath           string
	AllowUnsecuredOpenCode bool
	RequireAuthOnHealth    bool
	RequestTimeout         time.Duration
	OpenCodeUsername       string
	OpenCodePassword       string
}

func Load() (Config, error) {
	var cfg Config
	var err error

	cfg.GatewayAPIKey, err = secret("GATEWAY_API_KEY", true)
	if err != nil {
		return Config{}, err
	}
	cfg.OpenCodeBaseURL = strings.TrimRight(os.Getenv("OPENCODE_BASE_URL"), "/")
	if cfg.OpenCodeBaseURL == "" {
		return Config{}, errors.New("OPENCODE_BASE_URL is required")
	}
	if _, err := url.ParseRequestURI(cfg.OpenCodeBaseURL); err != nil {
		return Config{}, fmt.Errorf("OPENCODE_BASE_URL is invalid: %w", err)
	}
	cfg.DatabasePath = os.Getenv("DATABASE_PATH")
	if cfg.DatabasePath == "" {
		return Config{}, errors.New("DATABASE_PATH is required")
	}
	cfg.AllowUnsecuredOpenCode, err = boolEnv("ALLOW_UNSECURED_OPENCODE")
	if err != nil {
		return Config{}, err
	}
	cfg.RequireAuthOnHealth, err = boolEnv("REQUIRE_AUTH_ON_HEALTH")
	if err != nil {
		return Config{}, err
	}
	seconds := os.Getenv("REQUEST_TIMEOUT_SECONDS")
	if seconds == "" {
		return Config{}, errors.New("REQUEST_TIMEOUT_SECONDS is required")
	}
	timeout, err := strconv.Atoi(seconds)
	if err != nil || timeout <= 0 {
		return Config{}, errors.New("REQUEST_TIMEOUT_SECONDS must be a positive integer")
	}
	cfg.RequestTimeout = time.Duration(timeout) * time.Second

	cfg.OpenCodeUsername, err = secret("OPENCODE_SERVER_USERNAME", false)
	if err != nil {
		return Config{}, err
	}
	if cfg.OpenCodeUsername == "" {
		cfg.OpenCodeUsername = "opencode"
	}
	cfg.OpenCodePassword, err = secret("OPENCODE_SERVER_PASSWORD", false)
	if err != nil {
		return Config{}, err
	}
	if cfg.OpenCodePassword == "" && !cfg.AllowUnsecuredOpenCode {
		return Config{}, errors.New("OPENCODE_SERVER_PASSWORD is empty and ALLOW_UNSECURED_OPENCODE is false")
	}
	return cfg, nil
}

func boolEnv(name string) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return false, fmt.Errorf("%s is required", name)
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func secret(name string, required bool) (string, error) {
	if file := os.Getenv(name + "_FILE"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read %s_FILE: %w", name, err)
		}
		value := strings.TrimSpace(string(data))
		if required && value == "" {
			return "", fmt.Errorf("%s_FILE is empty", name)
		}
		return value, nil
	}
	value := os.Getenv(name)
	if required && value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}
