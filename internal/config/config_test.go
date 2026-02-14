package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any env vars that might be set
	for _, key := range []string{
		"DREDD_PORT", "NATS_URL", "NATS_TOKEN", "DATABASE_URL", "LOG_LEVEL",
		"ANTHROPIC_API_KEY", "DREDD_MODEL", "SLACK_BOT_TOKEN",
		"SLACK_DECISIONS_CHANNEL", "CHRONICLE_URL", "DREDD_API_TOKEN",
	} {
		t.Setenv(key, "")
	}

	// Re-set to empty to clear (t.Setenv restores original after test)
	cfg := Load()

	if cfg.Port != 8750 {
		t.Errorf("expected default port 8750, got %d", cfg.Port)
	}
	if cfg.NatsURL != "nats://hermes:4222" {
		t.Errorf("expected default nats url, got %s", cfg.NatsURL)
	}
	if cfg.NatsToken != "" {
		t.Errorf("expected empty default nats token, got %s", cfg.NatsToken)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default log level info, got %s", cfg.LogLevel)
	}
	if cfg.AnthropicModel != "claude-sonnet-4-20250514" {
		t.Errorf("expected default model, got %s", cfg.AnthropicModel)
	}
	if cfg.ChronicleURL != "http://chronicle:8700" {
		t.Errorf("expected default chronicle url, got %s", cfg.ChronicleURL)
	}
	if cfg.APIToken != "" {
		t.Errorf("expected empty default api token, got %s", cfg.APIToken)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("DREDD_PORT", "9999")
	t.Setenv("NATS_URL", "nats://custom:4222")
	t.Setenv("NATS_TOKEN", "s3cr3t-token")
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/dredd")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	t.Setenv("DREDD_MODEL", "claude-opus-4-6")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_DECISIONS_CHANNEL", "C12345")
	t.Setenv("CHRONICLE_URL", "http://localhost:8700")
	t.Setenv("DREDD_API_TOKEN", "dredd-secret-token")

	cfg := Load()

	if cfg.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Port)
	}
	if cfg.NatsURL != "nats://custom:4222" {
		t.Errorf("expected custom nats url, got %s", cfg.NatsURL)
	}
	if cfg.NatsToken != "s3cr3t-token" {
		t.Errorf("expected custom nats token, got %s", cfg.NatsToken)
	}
	if cfg.DatabaseURL != "postgres://test:test@localhost/dredd" {
		t.Errorf("expected custom db url, got %s", cfg.DatabaseURL)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected debug log level, got %s", cfg.LogLevel)
	}
	if cfg.AnthropicAPIKey != "sk-test-key" {
		t.Errorf("expected custom api key, got %s", cfg.AnthropicAPIKey)
	}
	if cfg.AnthropicModel != "claude-opus-4-6" {
		t.Errorf("expected custom model, got %s", cfg.AnthropicModel)
	}
	if cfg.SlackBotToken != "xoxb-test" {
		t.Errorf("expected custom slack token, got %s", cfg.SlackBotToken)
	}
	if cfg.SlackChannel != "C12345" {
		t.Errorf("expected custom slack channel, got %s", cfg.SlackChannel)
	}
	if cfg.ChronicleURL != "http://localhost:8700" {
		t.Errorf("expected custom chronicle url, got %s", cfg.ChronicleURL)
	}
	if cfg.APIToken != "dredd-secret-token" {
		t.Errorf("expected custom api token, got %s", cfg.APIToken)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	t.Setenv("DREDD_PORT", "notanumber")

	cfg := Load()

	if cfg.Port != 8750 {
		t.Errorf("expected default port on invalid value, got %d", cfg.Port)
	}
}
