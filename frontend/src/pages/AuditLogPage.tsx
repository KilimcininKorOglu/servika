// Security log — the readable face of audit_log. The table has been filling up
// since the first release but was not surfaced anywhere in the panel; seeing
// failed login attempts meant SSHing into the server.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'

type Entry = {
  id: number
  time: string
  username: string
  ip: string
  action: string
  target: string
  ok: boolean
}

export default function AuditLogPage() {
  const { t } = useTranslation('AuditLogPage')
  const [list, setList] = useState<Entry[]>([])
  const [actions, setActions] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [action, setAction] = useState('')
  const [onlyFailed, setOnlyFailed] = useState(false)
  const [limit, setLimit] = useState(200)
  const [query, setQuery] = useState('')

  const fetchLog = useCallback(async () => {
    setLoading(true)
    try {
      const p = new URLSearchParams()
      p.set('limit', String(limit))
      if (action) p.set('action', action)
      if (onlyFailed) p.set('only_failed', '1')
      const r = await api.get<Entry[]>(`/audit?${p.toString()}`)
      setList(Array.isArray(r.data) ? r.data : [])
      setError(null)
    } catch (e) {
      setError(apiError(e, t('errors.loadFailed')))
    } finally {
      setLoading(false)
    }
  }, [action, onlyFailed, limit, t])

  useEffect(() => { fetchLog() }, [fetchLog])

  useEffect(() => {
    api.get<string[]>('/audit/actions')
      .then((r) => setActions(Array.isArray(r.data) ? r.data : []))
      .catch(() => {})
  }, [])

  // Search is client-side: the server filters by action/result, free text (IP,
  // target, username) is matched over the already-fetched page.
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return list
    return list.filter((k) =>
      `${k.username} ${k.ip} ${k.action} ${k.target}`.toLowerCase().includes(q))
  }, [list, query])

  const failed = list.filter((k) => !k.ok).length

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[{ label: t('breadcrumbHome'), href: '/' }, { label: t('title') }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
          {t('subtitle')}
        </p>
      </div>

      {list.length > 0 && (
        <div className="flex flex-wrap gap-2 mb-4">
          <div className="px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 text-sm text-slate-700 dark:text-slate-300">
            <span className="font-semibold">{list.length}</span> <span className="opacity-75">{t('stats.entries')}</span>
          </div>
          {failed > 0 && (
            <div className="px-3 py-2 rounded-lg border border-red-200 bg-red-50 text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300 text-sm">
              <span className="font-semibold">{failed}</span> <span className="opacity-75">{t('stats.failed')}</span>
            </div>
          )}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2 mb-4">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('searchPlaceholder')}
          className="w-full sm:w-64 px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-1 focus:ring-brand-500"
        />
        <select
          value={action}
          onChange={(e) => setAction(e.target.value)}
          className="px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
        >
          <option value="">{t('filters.allActions')}</option>
          {actions.map((a) => <option key={a} value={a}>{a}</option>)}
        </select>
        <select
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
          className="px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
        >
          <option value={100}>{t('filters.last', { n: 100 })}</option>
          <option value={200}>{t('filters.last', { n: 200 })}</option>
          <option value={500}>{t('filters.last', { n: 500 })}</option>
          <option value={1000}>{t('filters.last', { n: 1000 })}</option>
        </select>
        <label className="inline-flex items-center gap-1.5 px-3 py-2 text-sm text-slate-600 dark:text-slate-400 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={onlyFailed}
            onChange={(e) => setOnlyFailed(e.target.checked)}
            className="rounded border-slate-300 dark:border-slate-700"
          />
          {t('filters.failedOnly')}
        </label>
      </div>

      {error && (
        <div className="mb-4 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">{error}</div>
      )}

      {loading ? (
        <div className="py-16 text-center text-sm text-slate-400">{t('loading')}</div>
      ) : list.length === 0 && !error ? (
        <EmptyState title={t('empty.title')} description={t('empty.description')} />
      ) : filtered.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-400">{t('noMatch')}</div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900/60">
              <tr>
                {['time', 'result', 'action', 'user', 'ip', 'target'].map((b) => (
                  <th key={b} className="px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 whitespace-nowrap">
                    {t(`columns.${b}`)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800 bg-white dark:bg-slate-950">
              {filtered.map((k) => (
                <tr key={k.id} className="hover:bg-slate-50 dark:hover:bg-slate-900/60 transition">
                  <td className="px-3 py-2 whitespace-nowrap font-mono text-xs text-slate-500">{k.time}</td>
                  <td className="px-3 py-2 whitespace-nowrap">
                    {k.ok
                      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{t('status.success')}</span>
                      : <span className="px-2 py-0.5 rounded text-xs bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300">{t('status.failed')}</span>}
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap font-mono text-xs">{k.action}</td>
                  <td className="px-3 py-2 whitespace-nowrap">{k.username || <span className="text-slate-400">—</span>}</td>
                  <td className="px-3 py-2 whitespace-nowrap font-mono text-xs text-slate-500">{k.ip || '—'}</td>
                  <td className="px-3 py-2 text-slate-600 dark:text-slate-400">{k.target || <span className="text-slate-400">—</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
