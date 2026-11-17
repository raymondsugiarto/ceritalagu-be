package song

import "strings"

const fallbackPreviewLines = 8

// PreviewLyrics returns the lyrics up to the end of the first [Chorus]: roughly what the 60 s
// preview plays. Without a chorus tag it returns the first lines.
func PreviewLyrics(lyrics string) string {
	lines := strings.Split(strings.TrimSpace(lyrics), "\n")
	inChorus := false
	for i, line := range lines {
		tag := strings.ToLower(strings.TrimSpace(line))
		isTag := strings.HasPrefix(tag, "[") && strings.HasSuffix(tag, "]")
		switch {
		case isTag && inChorus:
			return strings.TrimSpace(strings.Join(lines[:i], "\n"))
		case isTag && strings.Contains(tag, "chorus") && !strings.Contains(tag, "pre"):
			inChorus = true
		}
	}
	if inChorus {
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return strings.TrimSpace(strings.Join(lines[:min(len(lines), fallbackPreviewLines)], "\n"))
}
