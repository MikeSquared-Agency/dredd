package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MikeSquared-Agency/dredd/internal/anthropic"
	"github.com/MikeSquared-Agency/dredd/internal/api"
	"github.com/MikeSquared-Agency/dredd/internal/config"
	"github.com/MikeSquared-Agency/dredd/internal/extractor"
	"github.com/MikeSquared-Agency/dredd/internal/hermes"
	"github.com/MikeSquared-Agency/dredd/internal/processor"
	"github.com/MikeSquared-Agency/dredd/internal/slack"
	"github.com/MikeSquared-Agency/dredd/internal/store"
)

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	slog.Info("dredd starting", "port", cfg.Port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("database connected")

	// Anthropic client
	if cfg.AnthropicAPIKey == "" {
		slog.Error("ANTHROPIC_API_KEY is required")
		os.Exit(1)
	}
	llm := anthropic.NewClient(cfg.AnthropicAPIKey, cfg.AnthropicModel)
	slog.Info("anthropic client ready", "model", cfg.AnthropicModel)

	// Extractor
	ext := extractor.New(llm, slog.Default())

	// NATS/Hermes
	hermesClient, err := hermes.NewClient(ctx, cfg.NatsURL, slog.Default())
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer hermesClient.Close()
	slog.Info("NATS connected", "url", cfg.NatsURL)

	// Slack poster (optional — Dredd works without Slack, just no review loop)
	var slackPoster *slack.Poster
	if cfg.SlackBotToken != "" && cfg.SlackChannel != "" {
		slackPoster = slack.NewPoster(cfg.SlackBotToken, cfg.SlackChannel, slog.Default())
		slog.Info("slack poster ready", "channel", cfg.SlackChannel)
	} else {
		slog.Warn("slack not configured — running without review loop")
	}

	// Processor — the main pipeline
	proc := processor.New(db, ext, hermesClient, slackPoster, slog.Default())

	// Subscribe to transcript events
	if err := hermesClient.Subscribe("swarm.chronicle.transcript.stored", proc.HandleTranscriptStored); err != nil {
		slog.Error("failed to subscribe to transcript events", "error", err)
		os.Exit(1)
	}

	// Subscribe to Slack reactions for the review loop
	if err := hermesClient.Subscribe("swarm.slack.reaction", proc.HandleReaction); err != nil {
		slog.Error("failed to subscribe to slack reactions", "error", err)
		os.Exit(1)
	}

	// HTTP API
	srv := api.NewServer(cfg.Port)
	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Announce registration
	if err := hermesClient.Publish("swarm.agent.dredd.registered", map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"port":      cfg.Port,
		"mode":      "shadow",
	}); err != nil {
		slog.Warn("failed to publish registration", "error", err)
	}

	slog.Info("dredd ready — shadow mode", "port", cfg.Port)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("shutting down")
	cancel()
	slog.Info("dredd stopped")
}

func setupLogging(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
}
