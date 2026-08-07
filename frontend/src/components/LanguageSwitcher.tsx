import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import { useReportError } from '@/lib/errors'
import { getLang, setLang, LANGS, LANG_NAMES, type Lang } from '@/lib/i18n'
import { useAuth } from '@/store/auth'

// Language dropdown. Mirrors the TopBar theme button: local state stays in sync
// via the servika:lang-change event so every mounted switcher agrees. On change it
// applies the language (cookie + i18next via setLang) and persists it to the
// user's pref_lang through PUT /me/language (open to all roles). The DB write is
// best-effort — the cookie already makes the choice durable in the browser.
//
// It also mounts on the two sign-in screens, where there is no account to write
// a preference to. The write is skipped there rather than sent and allowed to
// fail: /me/language answers 401 without a session, and firing a request whose
// only possible outcome is a rejection is not a fallback, it is noise.
export default function LanguageSwitcher() {
  const report = useReportError()
  const signedIn = useAuth((s) => s.username !== null)
  const [lang, setLangState] = useState<Lang>(getLang())
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onChange(e: Event) {
      const detail = (e as CustomEvent).detail as Lang
      if ((LANGS as readonly string[]).includes(detail)) setLangState(detail)
    }
    window.addEventListener('servika:lang-change', onChange)
    return () => window.removeEventListener('servika:lang-change', onChange)
  }, [])

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [])

  function choose(next: Lang) {
    setOpen(false)
    if (next === lang) return
    setLang(next)
    setLangState(next)
    if (signedIn) api.put('/me/language', { pref_lang: next }).catch(report('languagePreference'))
  }

  // Short code shown on the button: the base subtag, uppercased (EN, PT, ZH).
  const short = lang.split('-')[0].toUpperCase()

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="px-2 py-1.5 text-xs font-semibold uppercase tracking-wide text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-md transition"
        title={LANG_NAMES[lang]}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        {short}
      </button>
      {open && (
        <ul
          role="listbox"
          className="absolute right-0 z-50 mt-1 max-h-80 w-44 overflow-auto rounded-lg border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-700 dark:bg-slate-800"
        >
          {LANGS.map((code) => (
            <li key={code}>
              <button
                type="button"
                role="option"
                aria-selected={code === lang}
                onClick={() => choose(code)}
                className={`flex w-full items-center justify-between px-3 py-1.5 text-left text-xs transition hover:bg-slate-100 dark:hover:bg-slate-700 ${
                  code === lang
                    ? 'font-semibold text-brand-600 dark:text-brand-400'
                    : 'text-slate-600 dark:text-slate-300'
                }`}
              >
                <span>{LANG_NAMES[code]}</span>
                <span className="ml-2 text-[10px] uppercase tracking-wide text-slate-400 dark:text-slate-500">{code}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
