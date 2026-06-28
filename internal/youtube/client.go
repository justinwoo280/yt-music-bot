package youtube

import (
	"net/http"
	"time"

	yt "github.com/kkdai/youtube/v2"
)

// Client wraps a YouTube (kkdai) client and a plain HTTP client for innertube.
type Client struct {
	http        *http.Client
	yt          *yt.Client
	MaxDuration time.Duration
	WorkDir     string
}

// NewClient builds a YouTube client. maxDurationMin <= 0 disables the limit.
func NewClient(httpClient *http.Client, maxDurationMin int, workDir string) *Client {
	c := &Client{
		http:        httpClient,
		MaxDuration: time.Duration(maxDurationMin) * time.Minute,
		WorkDir:     workDir,
		yt: &yt.Client{
			HTTPClient: httpClient,
		},
	}
	if maxDurationMin > 0 {
		c.MaxDuration = time.Duration(maxDurationMin) * time.Minute
	}
	return c
}
