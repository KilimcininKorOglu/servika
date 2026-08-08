import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiError } from '@/lib/api'
import { useDialog } from '@/lib/dialog'
import Breadcrumb from '@/components/Breadcrumb'

type Kind = 'node' | 'python'
type Runtime = { kind: Kind; version: string; path: string; system: boolean }
type RuntimeList = { node: Runtime[]; python: Runtime[] }

// The operation runs in a detached transient systemd unit under PID 1, so it
// survives a closed tab. Status is polled rather than held in this component.
type ActiveOp = { kind: Kind; version: string; action: 'install' | 'remove' }
type OpStatus = { running: boolean; kind?: Kind; version?: string; action?: 'install' | 'remove' }
type LogResponse = { log: string; running: boolean }

const KINDS: Array<{ kind: Kind; label: string }> = [
  { kind: 'node', label: 'Node.js' },
  { kind: 'python', label: 'Python' },
]

export default function RuntimeVersionsPage() {
  const { t } = useTranslation('RuntimeVersionsPage')
  const { confirm, notify } = useDialog()
  const [runtimes, setRuntimes] = useState<RuntimeList>({ node: [], python: [] })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [activeOp, setActiveOp] = useState<ActiveOp | null>(null)
  const [opLog, setOpLog] = useState('')
  const [wanted, setWanted] = useState<Record<Kind, string>>({ node: '', python: '' })
  const logRef = useRef<HTMLPreElement>(null)

  // Split so the mount effect never writes state synchronously: fetchRuntimes
  // settles only through promise callbacks, and load() adds the spinner for the
  // refresh that follows a completed install or removal.
  const fetchRuntimes = useCallback(() => {
    api.get<RuntimeList>('/app-runtimes')
      .then(r => setRuntimes({ node: r.data.node || [], python: r.data.python || [] }))
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }, [])

  const load = useCallback(() => {
    setLoading(true)
    fetchRuntimes()
  }, [fetchRuntimes])

  // Initial load, plus any operation already running so one started in another
  // tab is picked up rather than lost.
  useEffect(() => {
    fetchRuntimes()
    api.get<OpStatus>('/app-runtimes/status')
      .then(r => {
        if (r.data.running && r.data.kind && r.data.version) {
          setActiveOp({ kind: r.data.kind, version: r.data.version, action: r.data.action || 'install' })
        }
      })
      .catch(() => { // Ignore transient failures while resuming an operation.
      })
  }, [fetchRuntimes])

  useEffect(() => {
    if (!activeOp) return
    let done = false
    const tick = async () => {
      try {
        const r = await api.get<LogResponse>('/app-runtimes/log')
        if (done) return
        setOpLog(r.data.log || '')
        if (!r.data.running) {
          setSuccess(activeOp.action === 'remove'
            ? t('success.removed', { version: activeOp.version })
            : t('success.installed', { version: activeOp.version }))
          setTimeout(() => setSuccess(null), 6000)
          setActiveOp(null)
          load()
        }
      } catch {
        // Keep polling through transient network failures.
      }
    }
    const id = window.setInterval(tick, 2000)
    tick()
    return () => { done = true; window.clearInterval(id) }
  }, [activeOp, load, t])

  useEffect(() => { logRef.current?.scrollTo({ top: logRef.current.scrollHeight }) }, [opLog])

  async function install(kind: Kind) {
    const version = wanted[kind].trim()
    if (!version) return
    if (activeOp) { await notify({ message: t('alerts.opInProgress') }); return }
    if (!(await confirm({ message: t('confirm.install', { version }) }))) return
    setError(null); setSuccess(null); setOpLog('')
    try {
      await api.post('/app-runtimes/install', { kind, version })
      setOpLog(t('log.installStarted', { version }))
      setActiveOp({ kind, version, action: 'install' })
      setWanted({ ...wanted, [kind]: '' })
    } catch (e) { setError(apiError(e, t('errors.startInstallFailed'))) }
  }

  async function remove(runtime: Runtime) {
    if (activeOp) { await notify({ message: t('alerts.opInProgress') }); return }
    if (!(await confirm({ message: t('confirm.remove', { version: runtime.version }), dangerous: true }))) return
    setError(null); setSuccess(null); setOpLog('')
    try {
      await api.post('/app-runtimes/remove', { kind: runtime.kind, version: runtime.version })
      setOpLog(t('log.removeStarted', { version: runtime.version }))
      setActiveOp({ kind: runtime.kind, version: runtime.version, action: 'remove' })
    } catch (e) { setError(apiError(e, t('errors.startRemoveFailed'))) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: t('breadcrumb.home'), href: '/' },
        { label: t('breadcrumb.tools'), href: '/tools-settings' },
        { label: t('breadcrumb.current') },
      ]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">{t('title')}</h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{t('subtitle')}</p>
      </div>

      {error && <div className="mb-3 rounded-xl border border-red-200 bg-red-50 px-3 py-2.5 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-900/15 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2.5 text-xs text-emerald-700 dark:border-emerald-800/50 dark:bg-emerald-900/15 dark:text-emerald-300">{success}</div>}

      {activeOp && (
        <div className="mb-4 rounded-2xl border border-brand-200 bg-brand-50 p-4 dark:border-brand-900/50 dark:bg-brand-900/15">
          <div className="mb-2 flex items-center gap-2">
            <span className="h-3 w-3 animate-spin rounded-full border-2 border-brand-400 border-t-transparent" />
            <span className="text-sm font-semibold text-brand-700 dark:text-brand-300">
              {activeOp.action === 'remove'
                ? t('log.removeInProgress', { version: activeOp.version })
                : t('log.installInProgress', { version: activeOp.version })}
            </span>
          </div>
          <pre ref={logRef} className="max-h-48 overflow-auto rounded-xl bg-slate-900 p-3 font-mono text-xs text-slate-100">{opLog || t('log.waiting')}</pre>
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-12 text-sm text-slate-400">
          <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-slate-300 border-t-transparent dark:border-slate-600 dark:border-t-transparent" />
          {t('loading')}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {KINDS.map(({ kind, label }) => (
            <section key={kind} className="rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900/60">
              <h2 className="mb-3 text-lg font-semibold text-slate-900 dark:text-slate-100">{label}</h2>

              <div className="mb-3 space-y-2">
                {runtimes[kind].length === 0 && (
                  <p className="text-sm text-slate-500 dark:text-slate-400">{t('empty')}</p>
                )}
                {runtimes[kind].map(runtime => (
                  <div key={runtime.version} className="flex items-center gap-2 rounded-xl border border-slate-200 px-3 py-2 dark:border-slate-700">
                    <span className="font-mono text-sm font-semibold text-slate-900 dark:text-slate-100">
                      {runtime.system ? t('systemVersion') : runtime.version}
                    </span>
                    <span className="truncate font-mono text-xs text-slate-500 dark:text-slate-400">{runtime.path}</span>
                    {runtime.system ? (
                      <span className="ml-auto rounded bg-sky-100 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-sky-700 dark:bg-sky-900/30 dark:text-sky-300">
                        {t('badges.system')}
                      </span>
                    ) : (
                      <button onClick={() => remove(runtime)} disabled={!!activeOp}
                        className="ml-auto shrink-0 rounded-lg px-2.5 py-1 text-xs text-red-600 transition hover:bg-red-50 disabled:opacity-40 dark:text-red-400 dark:hover:bg-red-900/30">
                        {t('actions.remove')}
                      </button>
                    )}
                  </div>
                ))}
              </div>

              <div className="flex gap-2">
                <input
                  type="text"
                  value={wanted[kind]}
                  onChange={e => setWanted({ ...wanted, [kind]: e.target.value })}
                  placeholder={kind === 'node' ? t('placeholders.node') : t('placeholders.python')}
                  className="flex-1 rounded-md border border-slate-300 px-3 py-2 font-mono text-sm outline-none focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 dark:border-slate-600 dark:bg-slate-800"
                />
                <button onClick={() => install(kind)} disabled={!!activeOp || !wanted[kind].trim()}
                  className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100">
                  {t('actions.install')}
                </button>
              </div>
              <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
                {kind === 'node' ? t('hints.node') : t('hints.python')}
              </p>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
