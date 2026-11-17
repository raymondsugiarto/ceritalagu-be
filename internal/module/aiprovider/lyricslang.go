package aiprovider

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	sectionTag = regexp.MustCompile(`\[[^\]\n]*\]`)
	// codeToken catches model glitches such as "Tawanya_mock_shared?" or "foo.bar": letters joined by
	// an underscore or a dot, which never appear in sung lyrics.
	codeToken = regexp.MustCompile(`\p{L}[_]\p{L}|\p{L}\.\p{L}`)
	wordRe    = regexp.MustCompile(`[\p{L}']+`)
)

// englishMarkers are common English words that are never Indonesian words, so finding them in
// Indonesian lyrics means the model switched language mid-song ("thank you for everything").
var englishMarkers = map[string]bool{
	"the": true, "you": true, "your": true, "you're": true, "my": true, "me": true, "and": true,
	"for": true, "with": true, "thank": true, "thanks": true, "everything": true, "forever": true,
	"always": true, "never": true, "heart": true, "i'm": true, "don't": true, "is": true, "are": true,
	"will": true, "of": true, "to": true, "love": true, "baby": true, "miss": true, "this": true,
	"that": true, "we": true, "our": true, "be": true, "it": true, "all": true,
}

// maxEnglishMarkers is how many distinct English marker words Indonesian lyrics may contain.
const maxEnglishMarkers = 1

// CheckLyricsLanguage verifies the lyrics are written only in the requested language ("id" or "en"):
// Latin script only, no code-like glitch tokens, and for Indonesian no English phrases. Failures are
// KindInvalidOutput so the pipeline retries the call.
func CheckLyricsLanguage(provider, lyrics, lang string) error {
	fail := func(msg string) error {
		return &Error{Kind: KindInvalidOutput, Provider: provider, Message: msg}
	}
	body := sectionTag.ReplaceAllString(lyrics, " ")

	var foreign []rune
	seen := map[rune]bool{}
	for _, r := range body {
		if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) && !seen[r] {
			seen[r] = true
			foreign = append(foreign, r)
		}
	}
	if len(foreign) > 0 {
		return fail(fmt.Sprintf("lyrics contain non-Latin characters %q (expected %s only)", string(foreign[:min(len(foreign), 10)]), lang))
	}
	if m := codeToken.FindString(body); m != "" {
		return fail(fmt.Sprintf("lyrics contain a code-like token near %q", m))
	}
	if lang != "id" {
		return nil
	}

	found := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(body), -1) {
		if englishMarkers[w] {
			found[w] = true
		}
	}
	if len(found) > maxEnglishMarkers {
		words := make([]string, 0, len(found))
		for w := range found {
			words = append(words, w)
		}
		sort.Strings(words)
		return fail("Indonesian lyrics contain English words: " + strings.Join(words, ", "))
	}
	return nil
}
