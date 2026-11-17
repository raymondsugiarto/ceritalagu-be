package audio_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/yourorg/music-api/internal/audio"
)

// tone renders seconds of a sine wave as mp3 with ffmpeg.
func tone(t *testing.T, seconds string) []byte {
	t.Helper()
	out, err := exec.CommandContext(context.Background(), "ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+seconds, "-f", "mp3", "pipe:1").Output()
	if err != nil {
		t.Fatalf("render tone: %v", err)
	}
	return out
}

func TestPreviewCutsToSeconds(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	t.Parallel()
	f := audio.NewFFmpeg("")
	ctx := context.Background()

	preview, err := f.Preview(ctx, tone(t, "8"), 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Duration(ctx, preview)
	if err != nil {
		t.Fatal(err)
	}
	if got != 3 {
		t.Fatalf("preview duration = %d s, want 3", got)
	}
}

func TestPreviewShorterSource(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	t.Parallel()
	f := audio.NewFFmpeg("")
	preview, err := f.Preview(context.Background(), tone(t, "2"), 60, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Duration(context.Background(), preview); got > 2 {
		t.Fatalf("preview of a 2 s song should stay 2 s, got %d", got)
	}
}
