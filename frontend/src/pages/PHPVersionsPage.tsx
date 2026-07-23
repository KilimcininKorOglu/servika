import { useCallback, useEffect, useRef, useState } from 'react'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Version = {
  version: string; code: string; resource: 'remi' | 'appstream'
  loaded: boolean; installable?: boolean
  pool_dir?: string; sock_dir?: string; service?: string; php_bin?: string
  real_version?: string; module_count?: number; description?: string
}

// Detached job runs in a background systemd transient unit under PID 1.
// It survives tab close. Status and log polling show live progress.
type ActiveOp = { version: string; resource: string; action: 'install' | 'remove' }
type OpStatus = { running: boolean; version?: string; resource?: string; action?: 'install' | 'remove'; status?: string }
type LogResponse = { log: string; running: boolean; version?: string; resource?: string; action?: 'install' | 'remove' }

type Filter = 'all' | 'installed' | 'available'

export default function PHPVersionsPage() {
  const [versions, setVersions] = useState<Version[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [activeOp, setActiveOp] = useState<ActiveOp | null>(null)
  const [opLog, setOpLog] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const logRef = useRef<HTMLPreElement>(null)

  const load = useCallback(() => {
    setLoading(true)
    api.get<{ versions: Version[] }>('/php-versions')
      .then(r => setVersions(r.data.versions || []))
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }, [])

  // Initial load: version list + catch any running job (resume-on-reopen).
  useEffect(() => {
    load()
    api.get<OpStatus>('/php-versions/status')
      .then(r => {
        if (r.data.running && r.data.version) {
          setActiveOp({
            version: r.data.version,
            resource: r.data.resource || 'remi',
            action: r.data.action || 'install',
          })
        }
      })
      .catch(() => { // Ignore transient failures while resuming jobs.
      })
  }, [load])

  // Poll active job every two seconds. Refresh the version list when the job completes.
  useEffect(() => {
    if (!activeOp) return
    let done = false
    const tick = async () => {
      try {
        const r = await api.get<LogResponse>('/php-versions/log')
        if (done) return
        setOpLog(r.data.log || '')
        if (!r.data.running) {
          setSuccess(`PHP ${activeOp.version} ${activeOp.action === 'remove' ? 'removed' : 'installed'}`)
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
  }, [activeOp, load])

  useEffect(() => { logRef.current?.scrollTo({ top: logRef.current.scrollHeight }) }, [opLog])

  async function install(v: Version) {
    if (activeOp) { alert('A PHP operation is already in progress. Wait for it to finish.'); return }
    if (!confirm(`Fourteen packages will be installed for PHP ${v.version} (${v.resource}). Continue?`)) return
    setError(null); setSuccess(null); setOpLog('')
    try {
      await api.post('/php-versions/install', { version: v.version, resource: v.resource })
      setOpLog(`PHP ${v.version} installation started...\n`)
      setActiveOp({ version: v.version, resource: v.resource, action: 'install' })
    } catch (e) { setError(apiError(e, 'Could not start installation')) }
  }

  async function remove(v: Version) {
    if (v.resource === 'appstream') {
      alert('AppStream PHP is the system default and cannot be removed.')
      return
    }
    if (activeOp) { alert('A PHP operation is already in progress. Wait for it to finish.'); return }
    if (!confirm(`PHP ${v.version} (Remi) and ALL its extensions will be REMOVED.\nThe operation will be rejected if a domain uses this version. Continue?`)) return
    setError(null); setSuccess(null); setOpLog('')
    try {
      await api.post('/php-versions/remove', { version: v.version, resource: v.resource })
      setOpLog(`PHP ${v.version} removal started...\n`)

      setActiveOp({ version: v.version, resource: v.resource, action: 'remove' })
    } catch (e) { setError(apiError(e, 'Could not start removal')) }
  }

  const filtered = versions.filter(v => {
    if (filter === 'installed') return v.loaded
    if (filter === 'available') return !v.loaded
    return true
  })
  const installedCount = versions.filter(v => v.loaded).length

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Tools and Settings', href: '/tools-settings' },
        { label: 'PHP Versions' },
      ]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">PHP Versions</h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
          Add or remove PHP versions. Each runs in an independent PHP-FPM pool and can be selected per domain.
          Install includes 14 packages (fpm, cli, mysqlnd, and 11 extensions).
        </p>
      </div>

      {error && <div className="mb-3 flex items-start gap-2 rounded-xl border border-red-200 bg-red-50 px-3 py-2.5 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-900/15 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 flex items-start gap-2 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2.5 text-xs text-emerald-700 dark:border-emerald-800/50 dark:bg-emerald-900/15 dark:text-emerald-300">{success}</div>}

      {/* Active operation, inline live log */}
      {activeOp && (
        <div className="mb-4 rounded-2xl border border-brand-200 bg-brand-50 p-4 dark:border-brand-900/50 dark:bg-brand-900/15">
          <div className="mb-2 flex items-center gap-2">
            <span className="h-3 w-3 animate-spin rounded-full border-2 border-brand-400 border-t-transparent" />
            <span className="text-sm font-semibold text-brand-700 dark:text-brand-300">
              PHP {activeOp.version} {activeOp.action === 'remove' ? 'removal' : 'installation'} in progress...
            </span>
          </div>
          <pre ref={logRef} className="max-h-48 overflow-auto rounded-xl bg-slate-900 p-3 font-mono text-xs text-slate-100">{opLog || 'Waiting for output...'}</pre>
        </div>
      )}

      {/* Filter tabs */}
      <div className="mb-4 flex items-center gap-0.5 rounded-xl border border-slate-200 bg-slate-100 p-0.5 dark:border-slate-800 dark:bg-slate-800/60">
        {(['all', 'installed', 'available'] as const).map(opt => (
          <button key={opt} onClick={() => setFilter(opt)}
            className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${filter === opt ? 'bg-white text-slate-900 shadow-sm dark:bg-slate-700 dark:text-slate-100' : 'text-slate-500 hover:text-slate-800 dark:text-slate-400 dark:hover:text-slate-200'}`}>
            {opt === 'all' ? `All (${versions.length})` : opt === 'installed' ? `Installed (${installedCount})` : `Available (${versions.length - installedCount})`}
          </button>
        ))}
      </div>

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-12 text-sm text-slate-400">
          <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-slate-300 border-t-transparent dark:border-slate-600 dark:border-t-transparent" />
          Loading...
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
          {filtered.map(v => {
            const key = v.version + ':' + v.resource
            return (
              <div key={key}
                className={`rounded-2xl border p-4 transition ${
                  v.loaded ? 'border-emerald-200 bg-emerald-50 dark:border-emerald-800 dark:bg-emerald-900/20'
                    : 'border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900/60'}`}>
                <div className="mb-2 flex items-start justify-between">
                  <div>
                    <div className="font-mono text-lg font-bold text-slate-900 dark:text-slate-100">PHP {v.version}</div>
                    <div className="mt-0.5 flex items-center gap-1.5">
                      <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${
                        v.resource === 'appstream' ? 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
                          : 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300'
                      }`}>{v.resource}</span>
                      {v.loaded && <span className="rounded bg-emerald-100 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">INSTALLED</span>}
                      {parseInt(v.version) < 8 && <span className="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">EOL</span>}
                    </div>
                  </div>
                </div>

                {v.description && <div className="mb-2 text-xs text-slate-500 dark:text-slate-400">{v.description}</div>}

                {v.loaded && (
                  <div className="mb-3 space-y-0.5 rounded-lg border border-slate-200 bg-white p-2 font-mono text-xs text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400">
                    {v.real_version && <div>Version: <span className="text-slate-900 dark:text-slate-100">{v.real_version}</span></div>}
                    {v.module_count !== undefined && <div>Extensions: <span className="text-slate-900 dark:text-slate-100">{v.module_count}</span></div>}
                    {v.service && <div className="truncate">Service: <span className="text-slate-700 dark:text-slate-300">{v.service}</span></div>}
                  </div>
                )}

                {v.loaded ? (
                  v.resource === 'appstream' ? (
                    <button disabled className="w-full cursor-not-allowed rounded-xl bg-slate-100 px-3 py-2 text-sm text-slate-400 dark:bg-slate-800 dark:text-slate-500">
                      Fixed (system default)
                    </button>
                  ) : (
                    <button onClick={() => remove(v)} disabled={!!activeOp}
                      className="w-full rounded-xl bg-red-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-40">
                      {activeOp?.version === v.version ? 'In progress...' : 'Remove'}
                    </button>
                  )
                ) : (
                  <button onClick={() => install(v)} disabled={!!activeOp}
                    className="w-full rounded-xl bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100">
                    {activeOp?.version === v.version ? 'In progress...' : 'Install'}
                  </button>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
