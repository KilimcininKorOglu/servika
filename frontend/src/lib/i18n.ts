// Panel i18n — one JSON namespace per component under locales/<lang>/<Namespace>.json.
// Adding a namespace only needs one JSON per language; the import.meta.glob below
// collects every locale directory automatically, no manual registration.
//
// The glob is LAZY. Loading it eagerly put all twelve languages in the entry
// chunk, where the translations were the large majority of its bytes and eleven
// twelfths of them could never be read by the visitor who downloaded them. Each
// JSON is its own chunk now and the backend below fetches one when a component
// first asks for it.
//
// English is the default and fallback. The active language is mirrored in the
// `servika.lang` cookie (never localStorage), exactly like the theme in
// lib/theme.ts, so boot applies it before first paint.

import i18n, { type BackendModule, type TFunction } from 'i18next'
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

// Every locale JSON across all language directories, one dynamic import each.
// The keys look like ./locales/pt-BR/TopBar.json.
const localeModules = import.meta.glob<{ default: Record<string, unknown> }>('./locales/*/*.json')

// Namespaces that must be in memory before the first render, because they are
// used where nothing can catch a suspended component:
//
//   - common               the default namespace, plus DialogProvider and
//                          lib/useCopyOrOffer, which are mounted app-wide
//   - LoginPage,           statically imported routes in App.tsx that never
//     CustomerLoginPage    reach DashboardLayout's boundary
//   - DashboardLayout,     rendered by the layout ABOVE its own Suspense
//     TopBar, MobileNavBar, boundary, which only wraps the Outlet
//     DomainPicker
//   - ErrorSurface         mounted through lib/errors.ts anywhere
//
// A new always-mounted component that calls useTranslation belongs here too.
// Everything else is a page below the layout's boundary and loads on demand.
const BOOT_NAMESPACES = [
  'common',
  'DashboardLayout',
  'TopBar',
  'MobileNavBar',
  'ErrorSurface',
  'DomainPicker',
  'LoginPage',
  'CustomerLoginPage',
] as const

// The backend resolves the EXACT language directory and never reduces a code to
// its base: pt and pt-BR are separate languages here with different files, so
// trimming the region (or setting load: 'languageOnly') would quietly serve
// Iberian Portuguese to a Brazilian visitor.
const localeBackend: BackendModule = {
  type: 'backend',
  init() { /* the glob is the whole configuration */ },
  read(language, namespace, callback) {
    const load = localeModules[`./locales/${language}/${namespace}.json`]
    if (!load) {
      // Report it rather than leaving the caller pending: i18next then falls
      // back to English instead of suspending forever on a namespace that will
      // never arrive.
      callback(new Error(`missing locale bundle: ${language}/${namespace}`), false)
      return
    }
    load()
      .then((module) => callback(null, module.default))
      .catch((err) => callback(err instanceof Error ? err : new Error(String(err)), false))
  },
}

// initI18n resolves once BOOT_NAMESPACES are loaded for lang, so main.tsx can
// render knowing the always-mounted components have their text. Everything
// after that suspends at the nearest boundary while its namespace arrives.
export function initI18n(lang: Lang): Promise<TFunction> {
  return i18n.use(localeBackend).use(initReactI18next).init({
    lng: lang,
    fallbackLng: 'en',
    supportedLngs: LANGS as unknown as string[],
    defaultNS: 'common',
    ns: [...BOOT_NAMESPACES],
    interpolation: { escapeValue: false }, // React already escapes.
    returnEmptyString: false,
    react: { useSuspense: true },
  })
}

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
export function bootLang(lang: Lang) {
  if (typeof document !== 'undefined') document.documentElement.lang = lang
}

// How long boot waits for the server-default language before giving up on it.
// The screen behind this is the sign-in form, so a backend that stops answering
// must cost a moment and then English, never a page that never appears.
const BOOT_LANG_TIMEOUT_MS = 1500

// resolveBootLang decides which language to initialise with, BEFORE the first
// render. It used to switch the language after first paint, which was invisible
// only while every language was already in memory; loading translations on
// demand would have turned it into the sign-in screen painting in English and
// then swapping.
//
// The cookie means "the visitor chose this" and wins outright without a request.
// Without one the admin's install-time default applies (GET
// /api/v1/public/language), and that choice is deliberately NOT written to the
// cookie: a signed-in user's own pref_lang has to be able to take over.
export async function resolveBootLang(): Promise<Lang> {
  if (typeof window === 'undefined') return 'en'
  const chosen = getCookie(KEY)
  if (isLang(chosen)) return chosen
  try {
    const res = await fetch('/api/v1/public/language', {
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(BOOT_LANG_TIMEOUT_MS),
    })
    if (!res.ok) return 'en'
    const data: unknown = await res.json()
    const lang = (data as { lang?: unknown })?.lang
    return isLang(lang) ? lang : 'en'
  } catch {
    return 'en' // Sign-in must never break over the default language.
  }
}
