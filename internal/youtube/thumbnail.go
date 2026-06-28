package youtube

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
)

// FetchThumbnail downloads the video's thumbnail (hqdefault) and resizes it to
// a 320x320 square JPEG suitable for use as a Telegram audio cover. Returns
// nil bytes (no error) if the thumbnail cannot be obtained so callers can
// simply skip attaching a cover.
func (c *Client) FetchThumbnail(ctx context.Context, videoID string) ([]byte, error) {
	url := "https://i.ytimg.com/vi/" + videoID + "/hqdefault.jpg"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://www.youtube.com/")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thumbnail status %d", resp.StatusCode)
	}
	src, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, err
	}
	if len(src) == 0 {
		return nil, fmt.Errorf("empty thumbnail")
	}

	// Scale to fit 320x320 with letterboxing so it stays square (Telegram
	// audio covers look best square). Output a single JPEG frame to stdout.
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-vf", "scale=320:320:force_original_aspect_ratio=decrease,pad=320:320:(ow-iw)/2:(oh-ih)/2:color=black",
		"-frames:v", "1",
		"-f", "image2pipe",
		"-codec:v", "mjpeg",
		"-q:v", "5",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(src)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("resize thumbnail: %w", err)
	}
	if out.Len() == 0 {
		return nil, fmt.Errorf("empty resized thumbnail")
	}
	return out.Bytes(), nil
}
