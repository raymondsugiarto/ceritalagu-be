package generation

import (
	"math"
	"strings"

	"github.com/yourorg/music-api/internal/module/aiprovider"
)

// Preview cut tunables. The preview ends at the end of the first chorus, never at a fixed time;
// these only keep a bad timestamp or estimate from producing a silly clip.
const (
	chorusTailSec      = 1.5  // let the last chorus word ring out before the fade ends
	minPreviewSec      = 20.0 // shorter than this means the cut point is wrong
	maxPreviewFraction = 0.75 // never give away more than this share of the full song
)

// Cut methods, logged so the admin can see how a preview was cut.
const (
	cutTimestamps = "chorus_timestamps" // provider word timestamps
	cutEstimate   = "chorus_estimate"   // share of sung words through the first chorus × duration
	cutFallback   = "fallback_seconds"  // lyrics have no [Chorus]: generation.previewSeconds
)

// previewCut returns where the preview ends, in seconds, and how it was found.
func previewCut(lyrics string, words []aiprovider.AlignedWord, durationSec, fallbackSec int) (float64, string) {
	duration := float64(durationSec)
	through, total := chorusWordSpan(lyrics)
	if through == 0 || total == 0 || duration <= 0 {
		return math.Min(float64(fallbackSec), duration), cutFallback
	}

	cut, method := 0.0, cutEstimate
	if end, ok := chorusEndFromWords(words, through, total); ok {
		cut, method = end+chorusTailSec, cutTimestamps
	} else {
		// No timestamps: assume words are spread evenly over the song. Rough, but still ends the
		// preview around the end of the chorus rather than at an arbitrary second.
		cut = duration*float64(through)/float64(total) + chorusTailSec
	}
	limit := duration * maxPreviewFraction
	return math.Min(math.Max(cut, math.Min(minPreviewSec, limit)), limit), method
}

// chorusWordSpan counts sung words through the end of the first [Chorus] (not [Pre Chorus]) and in
// the whole song. through is 0 when there is no chorus.
func chorusWordSpan(lyrics string) (through, total int) {
	inChorus, done := false, false
	for _, line := range strings.Split(lyrics, "\n") {
		if tag, ok := sectionTag(line); ok {
			if inChorus {
				done = true
			}
			inChorus = !done && isChorus(tag)
			continue
		}
		n := len(strings.Fields(line))
		total += n
		if !done {
			through += n
		}
	}
	if !done && !inChorus { // never saw a chorus
		return 0, total
	}
	return through, total
}

// chorusEndFromWords finds the end of the first chorus in provider timestamps: by the section tag
// when the provider keeps tags in the words, else by the same share of words as in the lyrics
// (robust to the provider splitting words a little differently).
func chorusEndFromWords(words []aiprovider.AlignedWord, through, total int) (float64, bool) {
	if len(words) == 0 {
		return 0, false
	}
	start := -1
	for i, w := range words {
		tags := tagsIn(w.Text)
		if start < 0 {
			for _, t := range tags {
				if isChorus(t) {
					start = i
					break
				}
			}
			continue
		}
		if len(tags) > 0 && i > 0 {
			return words[i-1].EndSec, true
		}
	}
	idx := int(math.Round(float64(through)/float64(total)*float64(len(words)))) - 1
	idx = min(max(idx, 0), len(words)-1)
	return words[idx].EndSec, words[idx].EndSec > 0
}

func sectionTag(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
		return strings.ToLower(t), true
	}
	return "", false
}

func tagsIn(text string) []string {
	var tags []string
	for _, line := range strings.Split(text, "\n") {
		if tag, ok := sectionTag(line); ok {
			tags = append(tags, tag)
		}
	}
	return tags
}

func isChorus(tag string) bool {
	return strings.Contains(tag, "chorus") && !strings.Contains(tag, "pre")
}
