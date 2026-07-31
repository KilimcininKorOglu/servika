// Panel i18n — one JSON namespace per component under locales/{en,tr}/<Namespace>.json.
// Adding a namespace only needs the two JSON files; the import.meta.glob below
// collects them automatically, no manual registration.
//
// English is the default and fallback; Turkish is the second language. The
// active language is mirrored in the `servika.lang` cookie (never localStorage),
// exactly like the theme in lib/theme.ts, so boot applies it before first paint.

import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { getCookie, setCookie } from '@/lib/cookies'

export type Lang = 'en' | 'tr'

const KEY = 'servika.lang'
const ONE_YEAR_SEC = 365 * 24 * 60 * 60 // Language is a durable preference, not session-scoped.

export function getLang(): Lang {
  if (typeof window === 'undefined') return 'en'
  // Default: English — the panel's primary language and the i18next fallback.
  return getCookie(KEY) === 'tr' ? 'tr' : 'en'
}

const enModules = import.meta.glob('./locales/en/*.json', { eager: true }) as Record<string, { default: Record<string, unknown> }>
const trModules = import.meta.glob('./locales/tr/*.json', { eager: true }) as Record<string, { default: Record<string, unknown> }>

function buildResources(modules: Record<string, { default: Record<string, unknown> }>) {
  const out: Record<string, Record<string, unknown>> = {}
  for (const path in modules) {
    const match = path.match(/([^/]+)\.json$/)
    if (match) out[match[1]] = modules[path].default
  }
  return out
}

const enResources = buildResources(enModules)
const trResources = buildResources(trModules)
const namespaces = Array.from(new Set([...Object.keys(enResources), ...Object.keys(trResources)]))

i18n.use(initReactI18next).init({
  resources: { en: enResources, tr: trResources },
  lng: getLang(),
  fallbackLng: 'en',
  defaultNS: 'common',
  ns: namespaces.length ? namespaces : ['common'],
  interpolation: { escapeValue: false }, // React already escapes.
  returnEmptyString: false,
})

// setLang persists the choice (cookie), switches i18next live, updates <html lang>,
// and notifies listeners (LanguageSwitcher instances) via a CustomEvent — the same
// shape as the theme's servika:theme-change event.
export function setLang(lang: Lang) {
  setCookie(KEY, lang, ONE_YEAR_SEC)
  void i18n.changeLanguage(lang)
  if (typeof document !== 'undefined') document.documentElement.lang = lang
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent('servika:lang-change', { detail: lang }))
  }
}

// Call during boot from main.tsx before the initial render (sets <html lang>).
export function bootLang() {
  if (typeof document !== 'undefined') document.documentElement.lang = getLang()
}

// Server-default language for the pre-login screen. Only used when the visitor has
// NO explicit choice yet (no servika.lang cookie): the login screen then opens in
// whatever the admin set at install time (GET /api/v1/public/language). We switch
// live but deliberately do NOT write the cookie — the cookie means "the user chose
// this", and a signed-in user's own pref_lang always takes over afterwards. Runs
// after first paint, so it never blocks rendering; failure silently keeps English.
export function applyServerDefaultLang() {
  if (typeof window === 'undefined') return
  if (getCookie(KEY)) return // Explicit user choice already wins.
  fetch('/api/v1/public/language', { headers: { Accept: 'application/json' } })
    .then((res) => (res.ok ? res.json() : null))
    .then((data) => {
      const lang = data?.lang === 'tr' ? 'tr' : 'en'
      if (lang !== i18n.language) {
        void i18n.changeLanguage(lang)
        if (typeof document !== 'undefined') document.documentElement.lang = lang
      }
    })
    .catch(() => {}) // Login screen must never break over the default language.
}

export default i18n
