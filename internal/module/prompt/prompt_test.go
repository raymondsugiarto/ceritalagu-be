package prompt_test

import (
	"strings"
	"testing"

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
