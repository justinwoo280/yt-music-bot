package youtube

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	yt "github.com/kkdai/youtube/v2"
)

// DownloadResult holds the converted MP3 and its metadata.
type DownloadResult struct {
	Path     string
	Title    string
	Artist   string
	Seconds  int
	FileSize int64
}

// Download fetches the audio for the given video id and converts it to MP3.
// progress (optional) is periodically called with bytes downloaded / total.
// The caller is responsible for removing result.Path when done.
func (c *Client) Download(ctx context.Context, videoID string, progress func(downloaded, total int64)) (*DownloadResult, error) {
	if videoID == "" {
		return nil, fmt.Errorf("empty video id")
	}

	video, err := c.yt.GetVideoContext(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("fetch video info: %w", err)
	}

	if c.MaxDuration > 0 && video.Duration > 0 && video.Duration > c.MaxDuration {
		return nil, fmt.Errorf("video is %s long, exceeds limit of %s",
			formatDur(video.Duration), formatDur(c.MaxDuration))
	}

	format := pickBestAudio(video.Formats)
	if format == nil {
		return nil, fmt.Errorf("no audio stream available for %s", videoID)
	}

	stream, _, err := c.yt.GetStreamContext(ctx, video, format)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	inFile, err := os.CreateTemp(c.WorkDir, "ytm-in-*.bin")
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	inPath := inFile.Name()
	defer os.Remove(inPath)

	pr := &progressReader{r: stream, total: format.ContentLength, fn: progress}
	if _, err := io.Copy(inFile, pr); err != nil {
		inFile.Close()
		return nil, fmt.Errorf("download stream: %w", err)
	}
	pr.finish()
	if err := inFile.Close(); err != nil {
		return nil, fmt.Errorf("close temp input: %w", err)
	}

	title := strings.TrimSpace(video.Title)
	if title == "" {
		title = videoID
	}
	artist := strings.TrimSpace(video.Author)

	outF, err := os.CreateTemp(c.WorkDir, "ytm-*.mp3")
	if err != nil {
		return nil, fmt.Errorf("create temp output: %w", err)
	}
	outPath := outF.Name()
	outF.Close()
	os.Remove(outPath) // remove the empty placeholder; ffmpeg will create it

	res := &DownloadResult{
		Path:    outPath,
		Title:   title,
		Artist:  artist,
		Seconds: int(video.Duration.Seconds()),
	}

	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", inPath,
		"-vn",
		"-codec:a", "libmp3lame",
		"-qscale:a", "2",
		"-id3v2_version", "3",
		"-write_id3v1", "1",
		"-metadata", "title=" + title,
		"-metadata", "artist=" + artist,
		outPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if stderr, err := cmd.CombinedOutput(); err != nil {
		if len(stderr) > 0 {
			return nil, fmt.Errorf("ffmpeg failed: %w: %s", err, strings.TrimSpace(string(stderr)))
		}
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		return nil, fmt.Errorf("stat output: %w", err)
	}
	res.FileSize = info.Size()
	return res, nil
}

// pickBestAudio prefers audio-only formats (smaller) with the highest bitrate.
func pickBestAudio(formats yt.FormatList) *yt.Format {
	var best *yt.Format
	for i := range formats {
		f := &formats[i]
		if !strings.HasPrefix(strings.ToLower(f.MimeType), "audio/") {
			continue
		}
		if best == nil || f.Bitrate > best.Bitrate {
			best = f
		}
	}
	if best != nil {
		return best
	}
	// fall back to any format with audio
	withAudio := formats.WithAudioChannels()
	sort.Slice(withAudio, func(i, j int) bool {
		return withAudio[i].Bitrate > withAudio[j].Bitrate
	})
	if len(withAudio) > 0 {
		return &withAudio[0]
	}
	return nil
}

func formatDur(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

// progressReader wraps a reader and reports download progress at most every
// 500ms, plus a final report when the stream ends.
type progressReader struct {
	r     io.Reader
	n     int64
	total int64
	fn    func(downloaded, total int64)
	last  time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.n += int64(n)
		if p.fn != nil && time.Since(p.last) >= 500*time.Millisecond {
			p.last = time.Now()
			p.fn(p.n, p.total)
		}
	}
	return n, err
}

func (p *progressReader) finish() {
	if p.fn != nil {
		p.fn(p.n, p.total)
	}
}
