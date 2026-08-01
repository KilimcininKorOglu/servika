// Panel i18n — one JSON namespace per component under locales/<lang>/<Namespace>.json.
// Adding a namespace only needs one JSON per language; the import.meta.glob below
// collects every locale directory automatically, no manual registration.
//
// English is the default and fallback. The active language is mirrored in the
// `servika.lang` cookie (never localStorage), exactly like the theme in
// lib/theme.ts, so boot applies it before first paint.

import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { getCookie, setCookie } from '@/lib/cookies'

// Supported UI languages. English is the source/fallback; the rest are ordered
// as they appear in the switcher. pt-BR (Brazilian) is distinct from pt (Iberian).
export const LANGS = ['en', 'tr', 'de', 'fr', 'it', 'pt', 'pt-BR', 'es', 'cs', 'ro', 'ja', 'zh'] as const
export type Lang = (typeof LANGS)[number]

// Native display names for the language switcher.
export const LANG_NAMES: Record<Lang, string> = {
  en: 'English',
  tr: 'Türkçe',
  de: 'Deutsch',
  fr: 'Français',
  it: 'Italiano',
  pt: 'Português',
  'pt-BR': 'Português (Brasil)',
  es: 'Español',
  cs: 'Čeština',
  ro: 'Română',
  ja: '日本語',
  zh: '中文',
}

const KEY = 'servika.lang'
const ONE_YEAR_SEC = 365 * 24 * 60 * 60 // Language is a durable preference, not session-scoped.

function isLang(v: unknown): v is Lang {
  return typeof v === 'string' && (LANGS as readonly string[]).includes(v)
}

export function getLang(): Lang {
  if (typeof window === 'undefined') return 'en'
  // Default: English — the panel's primary language and the i18next fallback.
  const c = getCookie(KEY)
  return isLang(c) ? c : 'en'
}

// Collect every locale JSON across all language directories in one glob.
const allModules = import.meta.glob('./locales/*/*.json', { eager: true }) as Record<string, { default: Record<string, unknown> }>

// Build { lang: { namespace: {...} } } from paths like ./locales/pt-BR/TopBar.json.
function buildResources(modules: Record<string, { default: Record<string, unknown> }>) {
  const out: Record<string, Record<string, Record<string, unknown>>> = {}
  for (const path in modules) {
    const match = path.match(/\.\/locales\/([^/]+)\/([^/]+)\.json$/)
    if (!match) continue
    const [, lang, ns] = match
    ;(out[lang] ??= {})[ns] = modules[path].default
  }
  return out
}

const resources = buildResources(allModules)
const namespaces = Array.from(new Set(Object.values(resources).flatMap((r) => Object.keys(r))))

i18n.use(initReactI18next).init({
  resources,
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
      const lang = isLang(data?.lang) ? data.lang : 'en'
      if (lang !== i18n.language) {
        void i18n.changeLanguage(lang)
        if (typeof document !== 'undefined') document.documentElement.lang = lang
      }
    })
    .catch(() => {}) // Login screen must never break over the default language.
}

export default i18n
