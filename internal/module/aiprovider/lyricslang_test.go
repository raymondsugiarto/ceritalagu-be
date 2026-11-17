package aiprovider_test

import (
	"testing"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

func TestCheckLyricsLanguage(t *testing.T) {
	cases := []struct {
		name, lyrics, lang string
		ok                 bool
	}{
		{"clean indonesian", "[Verse]\nKecil, aku menggenggam jarimu\n[Chorus]\nSelamat Hari Ibu, Siti, mamaku tersayang", "id", true},
		{"one loanword allowed", "[Chorus]\nAku sayang mama, oh baby", "id", true},
		{"chinese characters", "[Pre Chorus]\nselalu为本ifu tetap berdiri", "id", false},
		{"english phrase", "[Chorus]\nSiti,thank you for everything\nCintamu hadir", "id", false},
		{"code token", "[Verse]\nTawanya_mock_shared?", "id", false},
		{"english song ok", "[Verse]\nThank you for everything, my love", "en", true},
		{"english song with hangul", "[Verse]\nThank you 사랑해", "en", false},
		{"accented latin ok", "[Verse]\nCafé di sudut kota", "id", true},
		{"section tags ignored", "[Intro]\n[Verse]\nHari Ibu untuk mama", "id", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := aiprovider.CheckLyricsLanguage("minimax", c.lyrics, c.lang)
			if c.ok && err != nil {
				t.Fatalf("want ok, got %v", err)
			}
			if !c.ok {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				if !aiprovider.IsRetryable(err) {
					t.Fatalf("language error must be retryable: %v", err)
				}
			}
		})
	}
}
