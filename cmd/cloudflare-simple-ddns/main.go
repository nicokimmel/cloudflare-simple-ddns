package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloudflare-simple-ddns/internal/config"
	"cloudflare-simple-ddns/internal/ddns"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.Time(slog.TimeKey, attr.Value.Time().In(time.Local))
			}
			return attr
		},
	}))

	env, err := config.LoadEnv()
	if err != nil {
		logger.Error("startup failed", "reason", err.Error())
		os.Exit(1)
	}

	if _, err := config.LoadEntries(config.ConfigPath); err != nil {
		logger.Error("startup failed", "reason", err.Error())
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	service := ddns.NewDefaultService(env.CloudflareAPIToken, logger)

	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	service.RunSync(runCtx)
	cancel()

	ticker := time.NewTicker(env.RunInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown requested")
			logger.Info("shutdown complete")
			return
		case <-ticker.C:
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			service.RunSync(runCtx)
			cancel()
		}
	}
}
