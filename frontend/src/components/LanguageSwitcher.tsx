import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import { getLang, setLang, type Lang } from '@/lib/i18n'

// EN/TR toggle. Mirrors the TopBar theme button: local state stays in sync via
// the servika:lang-change event so every mounted switcher agrees. On change it
// applies the language (cookie + i18next via setLang) and persists it to the
// user's pref_lang through PUT /me/language (open to all roles). The DB write is
// best-effort — the cookie already makes the choice durable in the browser.
export default function LanguageSwitcher() {
  const [lang, setLangState] = useState<Lang>(getLang())

  useEffect(() => {
    function onChange(e: Event) {
      const detail = (e as CustomEvent).detail as Lang
      if (detail === 'en' || detail === 'tr') setLangState(detail)
    }
    window.addEventListener('servika:lang-change', onChange)
    return () => window.removeEventListener('servika:lang-change', onChange)
  }, [])

  function toggle() {
    const next: Lang = lang === 'en' ? 'tr' : 'en'
    setLang(next)
    setLangState(next)
    api.put('/me/language', { pref_lang: next }).catch(() => {})
  }

  return (
    <button
      type="button"
      onClick={toggle}
      className="px-2 py-1.5 text-xs font-semibold uppercase tracking-wide text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-md transition"
      title={lang === 'en' ? 'Language: English, click for Türkçe' : 'Dil: Türkçe, İngilizce için tıkla'}
    >
      {lang === 'en' ? 'EN' : 'TR'}
    </button>
  )
}
