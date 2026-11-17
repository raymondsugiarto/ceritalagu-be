package aiprovider

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Column sizes of the song table (VARCHAR(n) counts characters): model free text is clipped
// to fit so a long value never fails the insert after the music is already paid for.
const (
	maxTitleLen    = 255
	maxLanguageLen = 50
	maxMoodLen     = 255
)

// clip shortens s to at most n characters (runes), never splitting a character.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n]))
}

var thinkBlock = regexp.MustCompile(`(?s)<think>.*?</think>`)

// lyricsStyleJSON is the JSON contract the prompt asks the model to return.
type lyricsStyleJSON struct {
	Title       string `json:"title"`
	Lyrics      string `json:"lyrics"`
	StylePrompt string `json:"style_prompt"`
	VocalGender string `json:"vocal_gender"`
	Language    string `json:"language"`
	Mood        string `json:"mood"`
}

// ParseLyricsStyle extracts and validates the lyrics/style JSON from a model reply. It tolerates
// <think> blocks, markdown code fences and text around the object. Failures are KindInvalidOutput,
// so the pipeline retries them.
func ParseLyricsStyle(provider, content string) (LyricsStyleResult, error) {
	fail := func(msg string, err error) (LyricsStyleResult, error) {
		// Raw is kept so the admin playground can show what the model actually replied.
		return LyricsStyleResult{Raw: content}, &Error{Kind: KindInvalidOutput, Provider: provider, Message: msg, Err: err}
	}

	text := thinkBlock.ReplaceAllString(content, "")
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return fail("no JSON object in reply", nil)
	}

	var out lyricsStyleJSON
	if err := json.Unmarshal([]byte(text[start:end+1]), &out); err != nil {
		return fail("reply is not valid JSON", err)
	}

	res := LyricsStyleResult{
		Title:       clip(strings.TrimSpace(out.Title), maxTitleLen),
		Lyrics:      strings.TrimSpace(out.Lyrics),
		StylePrompt: strings.TrimSpace(out.StylePrompt),
		VocalGender: strings.ToLower(strings.TrimSpace(out.VocalGender)),
		Language:    clip(strings.ToLower(strings.TrimSpace(out.Language)), maxLanguageLen),
		Mood:        clip(strings.TrimSpace(out.Mood), maxMoodLen),
		Raw:         content,
	}
	var missing []string
	if res.Title == "" {
		missing = append(missing, "title")
	}
	if res.Lyrics == "" {
		missing = append(missing, "lyrics")
	}
	if res.StylePrompt == "" {
		missing = append(missing, "style_prompt")
	}
	if len(missing) > 0 {
		return fail("missing fields: "+strings.Join(missing, ", "), nil)
	}
	if res.VocalGender != "" && res.VocalGender != "male" && res.VocalGender != "female" {
		return fail(fmt.Sprintf("vocal_gender must be male or female, got %q", res.VocalGender), errors.ErrUnsupported)
	}
	if !strings.Contains(res.Lyrics, "[") {
		return fail("lyrics have no section tags like [Verse] or [Chorus]", nil)
	}
	return res, nil
}
