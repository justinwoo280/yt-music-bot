package config

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	BotToken       string // Telegram bot token from @BotFather
	TGAPIEndpoint  string // optional self-hosted Bot API server endpoint
	Proxy          string // HTTP(S) proxy URL used for both YouTube and Telegram
	MaxDurationMin int    // reject songs longer than this (minutes)
	DownloadDir    string // temp directory for downloaded/converted files
	SearchLimit    int    // max search results to show
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	c := &Config{
		BotToken:       strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		TGAPIEndpoint:  strings.TrimSpace(os.Getenv("TG_API_ENDPOINT")),
		Proxy:          firstNonEmpty(os.Getenv("PROXY"), os.Getenv("HTTP_PROXY"), os.Getenv("HTTPS_PROXY"), os.Getenv("ALL_PROXY")),
		MaxDurationMin: 12,
		DownloadDir:    os.TempDir(),
		SearchLimit:    8,
	}

	if v := strings.TrimSpace(os.Getenv("MAX_DURATION_MIN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.MaxDurationMin = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("DOWNLOAD_DIR")); v != "" {
		c.DownloadDir = v
	}
	if v := strings.TrimSpace(os.Getenv("SEARCH_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 20 {
			c.SearchLimit = n
		}
	}

	if c.BotToken == "" {
		return nil, fmt.Errorf("environment variable BOT_TOKEN is required (get one from @BotFather)")
	}
	if c.MaxDurationMin < 1 {
		c.MaxDurationMin = 12
	}
	if c.DownloadDir == "" {
		c.DownloadDir = os.TempDir()
	}
	return c, nil
}

// NewHTTPClient builds an *http.Client honouring the configured proxy.
// If no proxy is configured it falls back to the standard environment
// proxy variables (http.ProxyFromEnvironment).
func (c *Config) NewHTTPClient() *http.Client {
	transport := &http.Transport{
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}

	switch {
	case c.Proxy != "":
		if u, err := url.Parse(c.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(u)
		} else {
			transport.Proxy = http.ProxyFromEnvironment
		}
	default:
		transport.Proxy = http.ProxyFromEnvironment
	}

	return &http.Client{
		Transport: transport,
		Timeout:   0, // downloads can take a while; per-request timeouts handled elsewhere
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
