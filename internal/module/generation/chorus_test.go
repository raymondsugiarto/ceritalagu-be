package generation

import (
	"testing"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

const testLyrics = `[Intro]
[Verse]
one two three four
five six seven eight
[Pre Chorus]
nine ten
[Chorus]
eleven twelve thirteen
fourteen fifteen sixteen
[Verse 2]
a b c d
e f g h
[Chorus]
x y z
`

func TestChorusWordSpan(t *testing.T) {
	through, total := chorusWordSpan(testLyrics)
	if through != 16 || total != 27 {
		t.Fatalf("through=%d total=%d, want 16/27", through, total)
	}
	if through, _ := chorusWordSpan("[Verse]\nno chorus here"); through != 0 {
		t.Fatalf("no chorus: through=%d, want 0", through)
	}
}

func words(n int, tags map[int]string) []aiprovider.AlignedWord {
	out := make([]aiprovider.AlignedWord, n)
	for i := range out {
		text := "w"
		if tag, ok := tags[i]; ok {
			text = tag + "\nw"
		}
		out[i] = aiprovider.AlignedWord{Text: text, StartSec: float64(10 + i*3), EndSec: float64(12 + i*3)}
	}
	return out
}

func TestPreviewCutByTags(t *testing.T) {
	// Chorus starts at word 10, Verse 2 at word 16 -> chorus ends at word 15: 12+45 = 57 s.
	w := words(27, map[int]string{0: "[Verse]", 8: "[Pre Chorus]", 10: "[Chorus]", 16: "[Verse 2]", 24: "[Chorus]"})
	cut, method := previewCut(testLyrics, w, 150, 60)
	if method != cutTimestamps || cut != 57+chorusTailSec {
		t.Fatalf("cut=%v method=%s", cut, method)
	}
}

func TestPreviewCutByWordShare(t *testing.T) {
	// No tags in the words: 16 of 27 words -> word index 15 -> ends at 57 s.
	cut, method := previewCut(testLyrics, words(27, nil), 150, 60)
	if method != cutTimestamps || cut != 57+chorusTailSec {
		t.Fatalf("cut=%v method=%s", cut, method)
	}
}

func TestPreviewCutEstimateAndLimits(t *testing.T) {
	cut, method := previewCut(testLyrics, nil, 135, 60)
	if method != cutEstimate || cut != 135.0*16/27+chorusTailSec {
		t.Fatalf("estimate cut=%v method=%s", cut, method)
	}
	// Chorus at the very end must not give away the whole song.
	cut, _ = previewCut("[Verse]\na b\n[Chorus]\nc d", nil, 100, 60)
	if cut != 75 {
		t.Fatalf("capped cut=%v, want 75", cut)
	}
	cut, method = previewCut("[Verse]\nno chorus", nil, 150, 60)
	if method != cutFallback || cut != 60 {
		t.Fatalf("fallback cut=%v method=%s", cut, method)
	}
}
