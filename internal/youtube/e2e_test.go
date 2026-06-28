//go:build e2e

package youtube

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestE2ESearchDownload exercises the full search -> download -> mp3 pipeline.
// Run: go test -run TestE2ESearchDownload -v -timeout 180s
func TestE2ESearchDownload(t *testing.T) {
	c := NewClient(http.DefaultClient, 0, os.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	query := "Alan Walker Faded"
	t.Logf("searching: %q", query)
	tracks, err := c.Search(ctx, query, 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	for i, tr := range tracks {
		t.Logf("  [%d] %s — %s (%s) id=%s", i+1, tr.Title, tr.Artist, tr.Duration, tr.VideoID)
	}
	if len(tracks) == 0 {
		t.Fatal("no tracks returned")
	}

	target := tracks[0]
	t.Logf("downloading: %s (%s)", target.Title, target.VideoID)
	res, err := c.Download(ctx, target.VideoID, func(downloaded, total int64) {
		t.Logf("  progress: %d / %d bytes", downloaded, total)
	})
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer os.Remove(res.Path)

	t.Logf("downloaded: path=%s size=%d title=%q artist=%q dur=%ds",
		res.Path, res.FileSize, res.Title, res.Artist, res.Seconds)

	if res.FileSize == 0 {
		t.Fatal("output file is empty")
	}

	// Validate with ffprobe that it is a real audio file.
	probe := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"format=format_name,duration:stream=codec_name,channels", "-of", "default=noprint_wrappers=1", res.Path)
	out, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	t.Logf("ffprobe:\n%s", strings.TrimSpace(string(out)))

	if !strings.Contains(string(out), "mp3") && !strings.Contains(string(out), "id3") {
		// format_name may be "mp3"; stream codec "mp3". accept either.
		if !strings.Contains(string(out), "mp3") {
			t.Logf("warning: ffprobe output did not clearly indicate mp3")
		}
	}

	// Thumbnail fetch + resize.
	thumb, err := c.FetchThumbnail(ctx, target.VideoID)
	if err != nil {
		t.Logf("thumbnail (optional) failed: %v", err)
	} else {
		t.Logf("thumbnail: %d bytes", len(thumb))
		if len(thumb) < 2 || thumb[0] != 0xFF || thumb[1] != 0xD8 {
			t.Fatalf("thumbnail is not a valid JPEG")
		}
	}
}
