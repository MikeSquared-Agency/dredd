package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port            int
	NatsURL         string
	NatsToken       string
	DatabaseURL     string
	LogLevel        string
	AnthropicAPIKey string
	AnthropicModel  string
	SlackBotToken   string
	SlackChannel    string
	ChronicleURL    string
	APIToken        string
}

func Load() Config {
	return Config{
		Port:            envInt("DREDD_PORT", 8750),
		NatsURL:         envStr("NATS_URL", "nats://hermes:4222"),
		NatsToken:       envStr("NATS_TOKEN", ""),
		DatabaseURL:     envStr("DATABASE_URL", ""),
		LogLevel:        envStr("LOG_LEVEL", "info"),
		AnthropicAPIKey: envStr("ANTHROPIC_API_KEY", ""),
		AnthropicModel:  envStr("DREDD_MODEL", "claude-sonnet-4-20250514"),
		SlackBotToken:   envStr("SLACK_BOT_TOKEN", ""),
		SlackChannel:    envStr("SLACK_DECISIONS_CHANNEL", ""),
		ChronicleURL:    envStr("CHRONICLE_URL", "http://chronicle:8700"),
		APIToken:        envStr("DREDD_API_TOKEN", ""),
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
