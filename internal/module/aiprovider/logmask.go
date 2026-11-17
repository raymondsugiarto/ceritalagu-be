package aiprovider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// MaskKey shows only the start and end of an API key, for logs: "eyJhbG…x9Qk".
// Keys shorter than 16 characters show only the last 2, so most of a short key never leaks.
func MaskKey(key string) string {
	n := utf8.RuneCountInString(key)
	switch {
	case n == 0:
		return ""
	case n < 16:
		r := []rune(key)
		return "…" + string(r[max(0, n-2):])
	default:
		r := []rune(key)
		return string(r[:6]) + "…" + string(r[n-4:])
	}
}

// LogBody makes a provider request/response body fit for a log line: JSON strings longer than
// maxString (e.g. hex or base64 audio) become "<N chars omitted>", and the whole text is cut at maxTotal.
func LogBody(body []byte, maxString, maxTotal int) string {
	var v any
	if err := json.Unmarshal(body, &v); err == nil {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false) // keep "<N chars omitted>" and lyrics readable
		if err := enc.Encode(shorten(v, maxString)); err == nil {
			body = bytes.TrimRight(buf.Bytes(), "\n")
		}
	}
	s := string(body)
	if len(s) > maxTotal {
		return s[:maxTotal] + fmt.Sprintf("… <%d bytes total>", len(s))
	}
	return s
}

func shorten(v any, maxString int) any {
	switch t := v.(type) {
	case string:
		if len(t) > maxString {
			return fmt.Sprintf("<%d chars omitted>", len(t))
		}
		return t
	case map[string]any:
		for k, x := range t {
			t[k] = shorten(x, maxString)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = shorten(x, maxString)
		}
		return t
	default:
		return v
	}
}
