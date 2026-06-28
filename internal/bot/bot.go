package bot

import (
	"context"
	"fmt"
	"log"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"yt-music-bot/internal/config"
	"yt-music-bot/internal/youtube"
)

// Bot ties together the Telegram API and the YouTube downloader.
type Bot struct {
	api      *tgbotapi.BotAPI
	yt       *youtube.Client
	cfg      *config.Config
	log      *log.Logger
	sessions *sessionStore
}

// New creates the bot, configuring the Telegram API client (with optional
// proxy and optional self-hosted Bot API endpoint).
func New(cfg *config.Config, httpClient *http.Client, ytClient *youtube.Client) (*Bot, error) {
	endpoint := cfg.TGAPIEndpoint
	if endpoint == "" {
		endpoint = tgbotapi.APIEndpoint
	}
	api, err := tgbotapi.NewBotAPIWithClient(cfg.BotToken, endpoint, httpClient)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	api.Debug = false

	return &Bot{
		api:      api,
		yt:       ytClient,
		cfg:      cfg,
		log:      log.New(log.Writer(), "[bot] ", log.LstdFlags|log.Lmsgprefix),
		sessions: newSessionStore(),
	}, nil
}

// Run starts long-polling for updates until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	b.log.Printf("authorized as @%s", b.api.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return ctx.Err()
		case upd, ok := <-updates:
			if !ok {
				return nil
			}
			b.dispatch(ctx, upd)
		}
	}
}

// dispatch fans each update out to its handler in its own goroutine so a slow
// download never blocks the polling loop.
func (b *Bot) dispatch(ctx context.Context, upd tgbotapi.Update) {
	switch {
	case upd.Message != nil:
		go b.handleMessage(ctx, upd.Message)
	case upd.CallbackQuery != nil:
		go b.handleCallback(ctx, upd.CallbackQuery)
	}
}
