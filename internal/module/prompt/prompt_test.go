package prompt_test

import (
	"strings"
	"testing"

	"github.com/yourorg/music-api/internal/i18n"
	"github.com/yourorg/music-api/internal/module/prompt"
	"github.com/yourorg/music-api/internal/module/questionnaire"
)

func TestRenderDefault(t *testing.T) {
	t.Parallel()
	a := questionnaire.Answers{
		RecipientType: "partner", RecipientName: "Nadia", Occasion: "birthday", Genre: "pop",
		Voice: questionnaire.VoiceFemale, Special: "senyumnya", Memory: "pantai", Message: "aku sayang kamu",
		Language: questionnaire.LanguageID,
	}
	sys, usr := prompt.Render(prompt.Default, a, prompt.Hints{Genre: "warm acoustic"})

	if strings.Contains(sys+usr, "{{") {
		t.Fatalf("unrendered placeholder left:\n%s\n%s", sys, usr)
	}
	for _, want := range []string{"Bahasa Indonesia", "female"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
	for _, want := range []string{"Nadia", "nickname: Nadia", "warm acoustic", "pantai", "aku sayang kamu"} {
		if !strings.Contains(usr, want) {
			t.Errorf("user prompt missing %q", want)
		}
	}
}

func TestRenderSurpriseVoiceAndEnglish(t *testing.T) {
	t.Parallel()
	sys, _ := prompt.Render(prompt.Default, questionnaire.Answers{Voice: questionnaire.VoiceSurprise, Language: questionnaire.LanguageEN}, prompt.Hints{})
	if !strings.Contains(sys, "English") || !strings.Contains(sys, "whichever suits") {
		t.Fatalf("unexpected system prompt:\n%s", sys)
	}
}

func TestRenderUsesOptionNamesAndOtherText(t *testing.T) {
	tpl := prompt.Template{UserPrompt: "{{recipient_type}}|{{occasion}}|{{genre}}"}
	a := questionnaire.Answers{RecipientType: "other", RecipientOther: "Guru favoritku", Occasion: "birthday", Genre: "pop"}
	_, got := prompt.Render(tpl, a, prompt.Hints{OccasionName: "Ulang Tahun"})
	if got != "Guru favoritku|Ulang Tahun|pop" {
		t.Fatalf("got %q", got)
	}
}

func TestSongLocaleFollowsQuestionnaireLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		lang questionnaire.Language
		want i18n.Locale
	}{
		{questionnaire.LanguageID, i18n.ID},
		{questionnaire.LanguageEN, i18n.EN},
		{"", i18n.ID},
		{"fr", i18n.ID},
	}
	for _, tt := range tests {
		if got := prompt.SongLocale(tt.lang); got != tt.want {
			t.Errorf("SongLocale(%q) = %q; want %q", tt.lang, got, tt.want)
		}
	}
}

func TestOptionNamePrefersSongLanguageTranslation(t *testing.T) {
	t.Parallel()
	tests := []struct{ base, translated, want string }{
		{"Ibu", "Mom", "Mom"},
		{"Ibu", "", "Ibu"},
		{"Ibu", "   ", "Ibu"},
		{"", "", ""},
	}
	for _, tt := range tests {
		if got := prompt.OptionName(tt.base, tt.translated); got != tt.want {
			t.Errorf("OptionName(%q, %q) = %q; want %q", tt.base, tt.translated, got, tt.want)
		}
	}
}

// The names in the prompt come from Hints (resolved for the song language); the UI locale in the
// request context plays no part in Render.
func TestRenderMoodFallbackInSongLanguage(t *testing.T) {
	t.Parallel()
	tpl := prompt.Template{UserPrompt: "{{mood}}|{{mood_hint}}|{{genre}}"}
	tests := []struct {
		lang questionnaire.Language
		h    prompt.Hints
		want string
	}{
		{questionnaire.LanguageID, prompt.Hints{GenreName: "Lagu Nina Bobo"}, "sesuai momen|match the occasion|Lagu Nina Bobo"},
		{questionnaire.LanguageEN, prompt.Hints{GenreName: "Lullaby"}, "to suit the occasion|match the occasion|Lullaby"},
	}
	for _, tt := range tests {
		_, got := prompt.Render(tpl, questionnaire.Answers{Genre: "lullaby", Language: tt.lang}, tt.h)
		if got != tt.want {
			t.Errorf("%s: got %q; want %q", tt.lang, got, tt.want)
		}
	}
}
