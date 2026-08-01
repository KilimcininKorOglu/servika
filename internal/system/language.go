package system

// Long-running maintenance jobs triggered from the panel (update, optimize, CVE
// security update, KernelCare live patch) stream their live log into the panel.
// Their log headers, wrapper-script output, and error messages follow the panel's
// own default language (panel_settings.default_lang) instead of a fixed one.
//
// This package's route handlers are stateless (they take no DB parameter), so the
// connection is held package-level and set by main.go via Init — the same pattern
// as StartVersionCheck. English is the primary language and the fail-safe.

import "database/sql"

var db *sql.DB

// Init stores the panel DB connection; main.go calls it at startup.
func Init(d *sql.DB) { db = d }

// panelLang returns the server-default panel language. It falls back to English
// when the DB is unset (Init not called yet) or unreadable.
func panelLang() string {
	if db == nil {
		return "en"
	}
	var lang string
	_ = db.QueryRow(`SELECT default_lang FROM panel_settings WHERE id=1`).Scan(&lang)
	if lang != "tr" {
		return "en"
	}
	return "tr"
}

// t returns the English or Turkish string per the panel default language.
func t(en, tr string) string {
	if panelLang() == "tr" {
		return tr
	}
	return en
}
