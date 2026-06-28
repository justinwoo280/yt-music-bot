package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"yt-music-bot/internal/bot"
	"yt-music-bot/internal/config"
	"yt-music-bot/internal/youtube"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	fmt.Printf("yt-music-bot %s\n", version)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}

	httpClient := cfg.NewHTTPClient()
	ytClient := youtube.NewClient(httpClient, cfg.MaxDurationMin, cfg.DownloadDir)

	b, err := bot.New(cfg, httpClient, ytClient)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bot:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := b.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
}
