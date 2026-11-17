package aiprovider

import (
	"strings"
	"testing"
)

func TestMaskKey(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"short-key":                     "…ey",
		"eyJhbGciOiJSUzI1NiJ9.abc.x9Qk": "eyJhbG…x9Qk",
	}
	for in, want := range cases {
		if got := MaskKey(in); got != want {
			t.Errorf("MaskKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogBodyOmitsLongStrings(t *testing.T) {
	body := []byte(`{"data":{"audio":"` + strings.Repeat("ab", 5000) + `","status":2},"base_resp":{"status_code":0}}`)
	got := LogBody(body, 8000, 100000)
	if !strings.Contains(got, `"audio":"<10000 chars omitted>"`) && !strings.Contains(got, `<10000 chars omitted>`) {
		t.Fatalf("audio not omitted: %s", got)
	}
	if !strings.Contains(got, `"status_code":0`) {
		t.Fatalf("status lost: %s", got)
	}
	if got := LogBody([]byte("not json "+strings.Repeat("x", 50)), 10, 20); !strings.HasSuffix(got, "<59 bytes total>") {
		t.Fatalf("not cut: %s", got)
	}
}
