package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Track represents a single YouTube search result.
type Track struct {
	VideoID   string
	Title     string
	Artist    string
	Duration  string // human readable, e.g. "3:45"
	Seconds   int
	Thumbnail string
}

const (
	innertubeKey  = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	innertubeURL  = "https://www.youtube.com/youtubei/v1/search?prettyPrint=false&key=" + innertubeKey
	clientVersion = "2.20240601.00.00"
	searchTimeout = 25 * time.Second
)

var userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Search queries YouTube and returns up to limit tracks.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Track, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("empty query")
	}
	if limit <= 0 {
		limit = 8
	}

	body := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": clientVersion,
				"hl":            "en",
				"gl":            "US",
			},
		},
		"query": query,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, innertubeURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", "https://www.youtube.com")
	req.Header.Set("Referer", "https://www.youtube.com/")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search request returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	tracks := collectVideoRenderers(tree, limit)
	if len(tracks) == 0 {
		return nil, fmt.Errorf("no results found for %q", query)
	}
	return tracks, nil
}

// collectVideoRenderers walks the JSON tree collecting every videoRenderer it
// finds. This is intentionally generic so layout changes don't break parsing.
func collectVideoRenderers(tree map[string]any, limit int) []Track {
	var out []Track
	seen := map[string]bool{}

	var walk func(n any)
	walk = func(n any) {
		if len(out) >= limit {
			return
		}
		switch v := n.(type) {
		case map[string]any:
			if vr, ok := v["videoRenderer"].(map[string]any); ok {
				if t := parseVideoRenderer(vr); t != nil && t.VideoID != "" && !seen[t.VideoID] {
					seen[t.VideoID] = true
					out = append(out, *t)
				}
			}
			// still descend even if it was a videoRenderer (children unlikely, but safe)
			for _, child := range v {
				if len(out) >= limit {
					return
				}
				walk(child)
			}
		case []any:
			for _, item := range v {
				if len(out) >= limit {
					return
				}
				walk(item)
			}
		}
	}
	walk(tree)
	return out
}

func parseVideoRenderer(vr map[string]any) *Track {
	t := &Track{}
	t.VideoID = asString(vr["videoId"])
	if t.VideoID == "" {
		return nil
	}
	t.Title = firstRunText(vr["title"])
	t.Artist = firstRunText(vr["longBylineText"])
	if t.Artist == "" {
		t.Artist = firstRunText(vr["ownerChannelName"])
	}
	if lt, ok := vr["lengthText"].(map[string]any); ok {
		t.Duration = asString(lt["simpleText"])
		if t.Duration == "" {
			t.Duration = firstRunText(lt)
		}
	}
	t.Seconds = parseDurationSeconds(t.Duration)

	if th, ok := vr["thumbnail"].(map[string]any); ok {
		if thumbs, ok := th["thumbnails"].([]any); ok && len(thumbs) > 0 {
			if first, ok := thumbs[len(thumbs)-1].(map[string]any); ok {
				t.Thumbnail = asString(first["url"])
			}
		}
	}
	return t
}

func firstRunText(field any) string {
	m, ok := field.(map[string]any)
	if !ok {
		return ""
	}
	if s := asString(m["simpleText"]); s != "" {
		return s
	}
	if runs, ok := m["runs"].([]any); ok {
		var b strings.Builder
		for _, r := range runs {
			if rm, ok := r.(map[string]any); ok {
				b.WriteString(asString(rm["text"]))
			}
		}
		return b.String()
	}
	return ""
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func parseDurationSeconds(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	// handle H:MM:SS
	if len(parts) == 3 {
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		sec, _ := strconv.Atoi(parts[2])
		return h*3600 + m*60 + sec
	}
	if len(parts) == 2 {
		m, _ := strconv.Atoi(parts[0])
		sec, _ := strconv.Atoi(parts[1])
		return m*60 + sec
	}
	n, _ := strconv.Atoi(parts[0])
	return n
}

// ExtractVideoID parses a video id from any common YouTube / YouTube Music URL,
// or returns the input as-is if it already looks like an 11-char id.
func ExtractVideoID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if reVideoID.MatchString(s) && len(s) == 11 {
		return s
	}
	m := reVideoURL.FindStringSubmatch(s)
	if len(m) >= 2 && m[1] != "" {
		return m[1]
	}
	return ""
}

var (
	reVideoID  = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	reVideoURL = regexp.MustCompile(`(?:youtu\.be/|watch\?v=|embed/|shorts/|music\.youtube\.com/watch\?v=|youtube\.com/watch\?v=)([A-Za-z0-9_-]{11})`)
)
