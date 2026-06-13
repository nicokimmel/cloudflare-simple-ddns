package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloudflare-simple-ddns/internal/cloudflare"
	"cloudflare-simple-ddns/internal/config"
	"cloudflare-simple-ddns/internal/ddns"
	"cloudflare-simple-ddns/internal/ip"
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

	cfClient := cloudflare.NewClient(env.CloudflareAPIToken, 15*time.Second)
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 15*time.Second)
	if err := cfClient.VerifyToken(verifyCtx); err != nil {
		verifyCancel()
		logger.Error("startup failed", "reason", err.Error())
		os.Exit(1)
	}
	verifyCancel()

	service := &ddns.Service{
		ConfigPath: config.ConfigPath,
		Logger:     logger,
		CF:         cfClient,
		IP:         ip.NewDetector(10 * time.Second),
	}

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
