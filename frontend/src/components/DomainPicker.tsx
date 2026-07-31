import { useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'

type DomainSummary = { id: number; domain_name: string }

/**
 * Domain-mode sidebar picker.
 *
 * Selecting a domain only swaps the id in the path; the sub-page is kept — on
 * /subscriptions/12/dns, switching opens /subscriptions/7/dns. Comparing the
 * same screen across domains is a single click.
 */
export default function DomainPicker({ activeID }: { activeID: string }) {
  const { t } = useTranslation('DomainPicker')
  const [domains, setDomains] = useState<DomainSummary[]>([])
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const boxRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    api.get<DomainSummary[]>('/domains')
      .then((r) => setDomains(Array.isArray(r.data) ? r.data : []))
      .catch(() => {})
  }, [])

  // Close on outside click and Escape.
  useEffect(() => {
    if (!open) return
    function onClick(e: MouseEvent) {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setOpen(false)
    }
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') setOpen(false) }
    document.addEventListener('mousedown', onClick)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onClick)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  useEffect(() => { if (open) searchRef.current?.focus() }, [open])

  const active = domains.find((d) => String(d.id) === activeID)
  const filtered = useMemo(() => {
    const t = query.trim().toLowerCase()
    if (!t) return domains
    return domains.filter((d) => d.domain_name.toLowerCase().includes(t))
  }, [domains, query])

  function pick(id: number) {
    // Swap the id in the path, carry the sub-page across as-is.
    const subPath = location.pathname.replace(/^\/subscriptions\/\d+/, '')
    setOpen(false)
    setQuery('')
    navigate(`/subscriptions/${id}${subPath}`)
  }

  return (
    <div ref={boxRef} className="relative px-2 pt-2">
      <button
        type="button"
        onClick={() => setOpen((s) => !s)}
        aria-haspopup="listbox"
        aria-expanded={open}
        className="w-full flex items-center gap-2 px-2.5 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800 transition text-left"
      >
        <svg className="w-4 h-4 flex-shrink-0 text-brand-600 dark:text-brand-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.7}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <span className="min-w-0 flex-1 truncate text-sm font-medium text-slate-900 dark:text-slate-100">
          {active?.domain_name || t('selectDomain')}
        </span>
        <svg className={`w-3.5 h-3.5 flex-shrink-0 text-slate-400 transition-transform ${open ? 'rotate-180' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {open && (
        <div className="absolute left-2 right-2 top-full mt-1 z-30 rounded-lg border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950 shadow-lg overflow-hidden">
          {domains.length > 8 && (
            <div className="p-1.5 border-b border-slate-100 dark:border-slate-800">
              <input
                ref={searchRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t('search')}
                className="w-full px-2 py-1.5 text-sm rounded-md bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
          )}
          <ul role="listbox" className="max-h-64 overflow-y-auto py-1">
            {filtered.length === 0 && (
              <li className="px-3 py-2 text-sm text-slate-400">{t('noResults')}</li>
            )}
            {filtered.map((d) => {
              const selected = String(d.id) === activeID
              return (
                <li key={d.id}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={selected}
                    onClick={() => pick(d.id)}
                    className={`w-full text-left px-3 py-1.5 text-sm truncate transition ${
                      selected
                        ? 'bg-slate-100 dark:bg-slate-800 text-slate-900 dark:text-slate-100 font-medium'
                        : 'text-slate-600 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/60'
                    }`}
                  >
                    {d.domain_name}
                  </button>
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </div>
  )
}
