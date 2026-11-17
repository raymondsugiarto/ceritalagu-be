package aiprovider_test

import (
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

func TestParseLyricsStyle(t *testing.T) {
	t.Parallel()
	valid := `{"title":"Untuk Nadia","lyrics":"[Verse]\nhai\n[Chorus]\nsayang","style_prompt":"pop ballad, warm","vocal_gender":"Female","language":"id","mood":"romantic"}`

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{"plain JSON", valid, false},
		{"code fence and think block", "<think>let me write</think>\n```json\n" + valid + "\n```", false},
		{"text around", "Here you go:\n" + valid + "\nEnjoy!", false},
		{"no JSON", "sorry, I cannot", true},
		{"broken JSON", `{"title": "x",`, true},
		{"missing lyrics", `{"title":"x","style_prompt":"pop"}`, true},
		{"bad vocal gender", `{"title":"x","lyrics":"[Verse] a","style_prompt":"pop","vocal_gender":"robot"}`, true},
		{"no section tags", `{"title":"x","lyrics":"just words","style_prompt":"pop"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := aiprovider.ParseLyricsStyle("test", tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
				if !aiprovider.IsRetryable(err) {
					t.Fatalf("invalid output must be retryable: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Title != "Untuk Nadia" || got.VocalGender != "female" || got.Language != "id" {
				t.Fatalf("unexpected result: %+v", got)
			}
		})
	}
}

func TestParseLyricsStyleClipsLongFreeText(t *testing.T) {
	long := strings.Repeat("hangat dan penuh syukur ", 20) // ~460 chars
	reply := `{"title":"T","lyrics":"[Verse]\nla","style_prompt":"pop","language":"bahasa indonesia campur inggris sedikit sekali ya kak","mood":"` + long + `"}`
	got, err := aiprovider.ParseLyricsStyle("test", reply)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := len([]rune(got.Mood)); n > 255 {
		t.Fatalf("mood not clipped: %d chars", n)
	}
	if n := len([]rune(got.Language)); n > 50 {
		t.Fatalf("language not clipped: %d chars", n)
	}
}
