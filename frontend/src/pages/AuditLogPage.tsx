// Security log — the readable face of audit_log. The table has been filling up
// since the first release but was not surfaced anywhere in the panel; seeing
// failed login attempts meant SSHing into the server.
import { useEffect, useMemo, useState } from 'react'
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

// Action styling, keyed on the shape of Servika's own English action codes rather
// than an enumerated list, so a new code renders sensibly the day it is written.
// Colour is never the only signal: every badge also carries an icon and a
// translated label, and the raw code stays visible next to it.
type ActionStyle = { kind: string; icon: string; className: string }

const ACTION_STYLES: { match: (a: string) => boolean; style: ActionStyle }[] = [
  {
    match: (a) => a.endsWith('.delete'),
    style: {
      kind: 'destructive',
      icon: 'M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16',
      className: 'bg-red-50 text-red-700 ring-red-200 dark:bg-red-900/20 dark:text-red-300 dark:ring-red-900/40',
    },
  },
  {
    match: (a) => a.endsWith('.create'),
    style: {
      kind: 'create',
      icon: 'M12 4v16m8-8H4',
      className: 'bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-900/20 dark:text-emerald-300 dark:ring-emerald-900/40',
    },
  },
  {
    match: (a) => a.startsWith('auth.') || a.startsWith('customer.'),
    style: {
      kind: 'auth',
      icon: 'M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z',
      className: 'bg-indigo-50 text-indigo-700 ring-indigo-200 dark:bg-indigo-900/20 dark:text-indigo-300 dark:ring-indigo-900/40',
    },
  },
  {
    match: (a) => a.startsWith('migration.'),
    style: {
      kind: 'migration',
      icon: 'M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4',
      className: 'bg-sky-50 text-sky-700 ring-sky-200 dark:bg-sky-900/20 dark:text-sky-300 dark:ring-sky-900/40',
    },
  },
  {
    match: (a) => a.endsWith('.status') || a.endsWith('.limits') || a.endsWith('.update') || a.endsWith('.password'),
    style: {
      kind: 'change',
      icon: 'M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z',
      className: 'bg-blue-50 text-blue-700 ring-blue-200 dark:bg-blue-900/20 dark:text-blue-300 dark:ring-blue-900/40',
    },
  },
]

const OTHER_ACTION: ActionStyle = {
  kind: 'other',
  icon: 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
  className: 'bg-slate-100 text-slate-600 ring-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:ring-slate-700',
}

function actionStyle(action: string): ActionStyle {
  return ACTION_STYLES.find((r) => r.match(action))?.style ?? OTHER_ACTION
}

export default function AuditLogPage() {
  const { t } = useTranslation('AuditLogPage')
  const [list, setList] = useState<Entry[]>([])
  const [actions, setActions] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)

  const [action, setAction] = useState('')
  const [onlyFailed, setOnlyFailed] = useState(false)
  const [limit, setLimit] = useState(200)
  const [query, setQuery] = useState('')

  // Derived instead of stored: loading means the request for the CURRENT filter
  // combination has not settled, so changing a filter shows the spinner on the
  // same render rather than one frame of the previous result set.
  const filterKey = `${action}|${onlyFailed}|${limit}`
  const [loadedFor, setLoadedFor] = useState<string | null>(null)
  const loading = loadedFor !== filterKey

  useEffect(() => {
    let cancelled = false
    const p = new URLSearchParams()
    p.set('limit', String(limit))
    if (action) p.set('action', action)
    if (onlyFailed) p.set('only_failed', '1')
    api.get<Entry[]>(`/audit?${p.toString()}`)
      .then((r) => { if (!cancelled) { setList(Array.isArray(r.data) ? r.data : []); setError(null) } })
      .catch((e) => { if (!cancelled) setError(apiError(e, t('errors.loadFailed'))) })
      .finally(() => { if (!cancelled) setLoadedFor(filterKey) })
    return () => { cancelled = true }
  }, [action, onlyFailed, limit, filterKey, t])

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
                  <td className="px-3 py-2 whitespace-nowrap">
                    <ActionBadge action={k.action} label={t(`actionKind.${actionStyle(k.action).kind}`)} />
                  </td>
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

// ActionBadge shows the action category as an icon plus a translated label, with
// the raw code kept alongside so filtering and searching still work on the value
// the server actually stores.
function ActionBadge({ action, label }: { action: string; label: string }) {
  const style = actionStyle(action)
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        title={label}
        className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-medium ring-1 ring-inset ${style.className}`}
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-3 w-3 shrink-0" aria-hidden="true">
          <path strokeLinecap="round" strokeLinejoin="round" d={style.icon} />
        </svg>
        {label}
      </span>
      <code className="font-mono text-xs text-slate-500 dark:text-slate-400">{action}</code>
    </span>
  )
}
