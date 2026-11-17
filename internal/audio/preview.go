// Package audio post-processes generated songs with ffmpeg (preview trim + fade-out).
package audio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

// FFmpeg cuts previews by piping audio through the ffmpeg binary.
type FFmpeg struct {
	bin string
}

// NewFFmpeg uses the given binary path; empty means "ffmpeg" from PATH.
func NewFFmpeg(bin string) *FFmpeg {
	if bin == "" {
		bin = "ffmpeg"
	}
	return &FFmpeg{bin: bin}
}

// Preview returns the first seconds of full (the caller picks the cut, e.g. the end of the first
// chorus) as MP3 with a fadeSec fade-out at the end. The result is a separate file, so the client
// never receives the full song.
func (f *FFmpeg) Preview(ctx context.Context, full []byte, seconds float64, fadeSec int) ([]byte, error) {
	if seconds <= 0 {
		return nil, errors.New("preview seconds must be positive")
	}
	fade := math.Min(math.Max(float64(fadeSec), 0), seconds)
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-t", strconv.FormatFloat(seconds, 'f', 2, 64),
		"-af", fmt.Sprintf("afade=t=out:st=%.2f:d=%.2f", seconds-fade, fade),
		"-codec:a", "libmp3lame", "-b:a", "192k",
		"-f", "mp3", "pipe:1",
	}
	cmd := exec.CommandContext(ctx, f.bin, args...) //nolint:gosec // fixed binary, args built from numbers only
	cmd.Stdin = bytes.NewReader(full)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg preview: %w: %s", err, stderr.String())
	}
	if out.Len() == 0 {
		return nil, errors.New("ffmpeg preview: empty output")
	}
	return out.Bytes(), nil
}

// Duration returns the length of audio in whole seconds by decoding it once with ffmpeg
// (ffprobe cannot read mp3 duration from a pipe).
func (f *FFmpeg) Duration(ctx context.Context, data []byte) (int, error) {
	cmd := exec.CommandContext(ctx, f.bin, "-hide_banner", "-loglevel", "error", "-nostats", //nolint:gosec // fixed binary and args
		"-i", "pipe:0", "-progress", "pipe:1", "-f", "null", "-")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffmpeg duration: %w", err)
	}
	var micros int64 = -1
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "out_time_us="); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				micros = n
			}
		}
	}
	if micros < 0 {
		return 0, errors.New("ffmpeg duration: no out_time_us in progress output")
	}
	return int((micros + 500_000) / 1_000_000), nil
}
