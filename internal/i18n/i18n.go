// Package i18n resolves the viewer's locale (plan docs/plans/i18n-multilanguage.md §5).
//
// Supported locales are "id" (default; the base columns of master data are Indonesian) and "en".
// The locale of a request comes from, in order: the ?locale= query parameter, the NEXT_LOCALE
// cookie (shared with the frontend), the Accept-Language header, then the default.
//
// The locale only picks customer-facing labels. AI-facing hints stay English, and the song's
// language is the questionnaire's `language`, never the UI locale (§4.10).
package i18n

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Locale is a supported UI locale code.
type Locale string

// Supported locales.
const (
	ID Locale = "id"
	EN Locale = "en"
)

// Default is the fallback locale; master-data base columns hold its text.
const Default = ID

// Supported lists every locale the app can render.
var Supported = []Locale{ID, EN}

// CookieName is the cookie the frontend writes when the viewer picks a language.
const CookieName = "NEXT_LOCALE"

// QueryParam is the explicit override, e.g. GET /questionnaire-options?locale=en.
const QueryParam = "locale"

// Parse returns the supported locale for a tag such as "en", "EN" or "en-US" (primary subtag).
func Parse(tag string) (Locale, bool) {
	primary, _, _ := strings.Cut(strings.TrimSpace(tag), "-")
	primary, _, _ = strings.Cut(primary, "_")
	l := Locale(strings.ToLower(primary))
	if slices.Contains(Supported, l) {
		return l, true
	}
	return "", false
}

// MatchAcceptLanguage picks the best supported locale from an Accept-Language header, honouring
// q-values (q=0 means "not acceptable") and header order for ties.
func MatchAcceptLanguage(header string) (Locale, bool) {
	type entry struct {
		tag string
		q   float64
		i   int
	}
	var entries []entry
	for i, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		tag := strings.TrimSpace(fields[0])
		if tag == "" {
			continue
		}
		q := 1.0
		for _, f := range fields[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(f), "=")
			if ok && strings.TrimSpace(k) == "q" {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
				if err != nil {
					parsed = 0
				}
				q = parsed
			}
		}
		if q > 0 {
			entries = append(entries, entry{tag: tag, q: q, i: i})
		}
	}
	sort.SliceStable(entries, func(a, b int) bool { return entries[a].q > entries[b].q })
	for _, e := range entries {
		if l, ok := Parse(e.tag); ok {
			return l, true
		}
	}
	return "", false
}

// Resolve applies the order ?locale → cookie → Accept-Language → Default. Unsupported values are skipped.
func Resolve(query, cookie, acceptLanguage string) Locale {
	if l, ok := Parse(query); ok {
		return l
	}
	if l, ok := Parse(cookie); ok {
		return l
	}
	if l, ok := MatchAcceptLanguage(acceptLanguage); ok {
		return l
	}
	return Default
}

type ctxKey struct{}

// WithLocale stores l in ctx.
func WithLocale(ctx context.Context, l Locale) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the request locale, or Default when none was stored (CLI, worker, tests).
func FromContext(ctx context.Context) Locale {
	if l, ok := ctx.Value(ctxKey{}).(Locale); ok && l != "" {
		return l
	}
	return Default
}
