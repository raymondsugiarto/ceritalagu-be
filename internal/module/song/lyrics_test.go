package song_test

import (
	"testing"

	"github.com/yourorg/music-api/internal/module/song"
)

func TestPreviewLyrics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
	}{
		{"stops after first chorus", "[Intro]\nla\n[Verse]\nsatu\n[Pre Chorus]\nnaik\n[Chorus]\nNadia\nsayang\n[Verse]\ndua", "[Intro]\nla\n[Verse]\nsatu\n[Pre Chorus]\nnaik\n[Chorus]\nNadia\nsayang"},
		{"chorus is last", "[Verse]\nsatu\n[Chorus]\nNadia", "[Verse]\nsatu\n[Chorus]\nNadia"},
		{"lowercase tags", "[verse]\nsatu\n[chorus]\nNadia\n[outro]\nbye", "[verse]\nsatu\n[chorus]\nNadia"},
		{"no chorus", "a\nb\nc\nd\ne\nf\ng\nh\ni\nj", "a\nb\nc\nd\ne\nf\ng\nh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := song.PreviewLyrics(tt.in); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
