package config

import "strings"

// Supported UI languages. Must stay in sync with frontend/src/lib/i18n.ts LANGS.
// English is the source/fallback; the panel persists any of these in users.pref_lang
// and panel_settings.default_lang. pt-BR (Brazilian) is distinct from pt (Iberian).
var supportedLangs = map[string]struct{}{
	"en": {}, "tr": {}, "de": {}, "fr": {}, "it": {}, "pt": {},
	"pt-BR": {}, "es": {}, "cs": {}, "ro": {}, "ja": {}, "zh": {},
}

// canonLang trims the input and canonicalizes the region form: base subtag lower,
// region subtag upper (pt-br → pt-BR). It does NOT validate membership.
func canonLang(v string) string {
	s := strings.TrimSpace(v)
	if base, region, ok := strings.Cut(s, "-"); ok {
		return strings.ToLower(base) + "-" + strings.ToUpper(region)
	}
	return strings.ToLower(s)
}

// NormalizeLang returns the canonical supported code, or "en" for anything not in
// the supported set. Callers use it as the single tr/en-style whitelist so a bad
// value can never reach the database.
func NormalizeLang(v string) string {
	s := canonLang(v)
	if _, ok := supportedLangs[s]; ok {
		return s
	}
	return "en"
}

// IsSupportedLang reports whether v (after canonicalization) is a supported code.
// Use it when an unknown value should be rejected rather than coerced to "en".
func IsSupportedLang(v string) bool {
	_, ok := supportedLangs[canonLang(v)]
	return ok
}
