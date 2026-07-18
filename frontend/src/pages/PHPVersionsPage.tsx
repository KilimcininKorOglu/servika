import { useEffect, useState } from 'react'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Version = {
  version: string; code: string; resource: 'remi' | 'appstream'
  loaded: boolean
  pool_dir?: string; sock_dir?: string; service?: string; php_bin?: string
  real_version?: string; module_count?: number; description?: string
}

type Output = { title: string; output: string }
type Filter = 'all' | 'installed' | 'available'

export default function PHPVersionsPage() {
  const [versions, setVersions] = useState<Version[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [processing, setProcessing] = useState<string | null>(null)
  const [output, setOutput] = useState<Output | null>(null)
  const [filter, setFilter] = useState<Filter>('all')

  function load() {
    setLoading(true)
    api.get<{ versions: Version[] }>('/php-versions')
      .then(response => setVersions(response.data.versions || []))
      .catch(error => setError(apiError(error)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [])

  async function install(version: Version) {
    if (!confirm(`Fourteen packages will be installed for PHP ${version.version} (${version.resource}): fpm, cli, mysqlnd, and 11 extensions. Continue?`)) return
    const key = version.version + ':' + version.resource
    setProcessing(key); setError(null); setSuccess(null)
    try {
      const response = await api.post('/php-versions/install', { version: version.version, resource: version.resource })
      setSuccess(`✓ PHP ${version.version} installed`)
      setOutput({ title: `PHP ${version.version} installation`, output: response.data.output || '' })
      setTimeout(() => setSuccess(null), 4000)
      load()
    } catch (error) { setError(apiError(error, 'Installation failed')) }
    finally { setProcessing(null) }
  }

  async function remove(version: Version) {
    if (version.resource === 'appstream') {
      alert('AppStream PHP is the system default and cannot be removed.')
      return
    }
    if (!confirm(`PHP ${version.version} (Remi) and ALL its extensions will be REMOVED.\nThe operation will be rejected if a domain uses this version. Continue?`)) return
    const key = version.version + ':' + version.resource
    setProcessing(key); setError(null); setSuccess(null)
    try {
      const response = await api.post('/php-versions/remove', { version: version.version, resource: version.resource })
      setSuccess(`✓ PHP ${version.version} removed`)
      setOutput({ title: `PHP ${version.version} removal`, output: response.data.output || '' })
      setTimeout(() => setSuccess(null), 4000)
      load()
    } catch (error) { setError(apiError(error, 'Removal failed')) }
    finally { setProcessing(null) }
  }

  const filtered = versions.filter(version => {
    if (filter === 'installed') return version.loaded
    if (filter === 'available') return !version.loaded
    return true
  })
  const installedCount = versions.filter(version => version.loaded).length

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Tools and Settings' },
        { label: 'PHP Versions' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">PHP Versions</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        Add or remove PHP versions on the server. Each version runs in an independent PHP-FPM pool and can be selected per domain.
        Installation includes 14 packages (fpm, cli, mysqlnd, mbstring, bcmath, intl, gd, soap, opcache, pdo, xml, zip, pgsql, ldap).
      </p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {/* Filter */}
      <div className="flex items-center gap-2 mb-4">
        <span className="text-sm text-slate-600 dark:text-slate-400 dark:text-slate-500 mr-2">Filter:</span>
        {(['all', 'installed', 'available'] as const).map(option => (
          <button key={option} onClick={() => setFilter(option)}
            className={`px-3 py-1 text-sm rounded ${filter === option ? 'bg-brand-600 text-white' : 'border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-400 dark:text-slate-500 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800'}`}>
            {option === 'all' ? 'All' : option === 'installed' ? `Installed (${installedCount})` : `Available (${versions.length - installedCount})`}
          </button>
        ))}
      </div>

      {loading ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div> : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {filtered.map(version => {
            const key = version.version + ':' + version.resource
            const busy = processing === key
            return (
              <div key={key}
                className={`border rounded-2xl p-4 transition ${version.loaded ? 'border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/20' : 'border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800'}`}>
                <div className="flex items-start justify-between mb-2">
                  <div>
                    <div className="text-lg font-mono font-bold text-slate-900 dark:text-slate-100">PHP {version.version}</div>
                    <div className="flex items-center gap-1.5 mt-0.5">
                      <span className={`text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium ${
                        version.resource === 'appstream'
                          ? 'bg-sky-100 text-sky-700'
                          : 'bg-violet-100 dark:bg-violet-900/30 text-violet-700 dark:text-violet-300'
                      }`}>{version.resource}</span>
                      {version.loaded && <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300">INSTALLED</span>}
                      {parseInt(version.version) < 8 && <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300">EOL</span>}
                    </div>
                  </div>
                </div>

                {version.description && <div className="text-xs text-slate-500 dark:text-slate-500 mb-2">{version.description}</div>}

                {version.loaded && (
                  <div className="text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500 space-y-0.5 mb-3 font-mono bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded p-2">
                    {version.real_version && <div>Version: <span className="text-slate-900 dark:text-slate-100">{version.real_version}</span></div>}
                    {version.module_count !== undefined && <div>Extensions: <span className="text-slate-900 dark:text-slate-100">{version.module_count}</span></div>}
                    {version.service && <div className="truncate">Service: <span className="text-slate-700 dark:text-slate-300">{version.service}</span></div>}
                  </div>
                )}

                {version.loaded ? (
                  version.resource === 'appstream' ? (
                    <button disabled className="w-full px-3 py-1.5 bg-slate-100 dark:bg-slate-800 text-slate-400 dark:text-slate-500 text-sm rounded cursor-not-allowed">
                      Fixed (system default)
                    </button>
                  ) : (
                    <button onClick={() => remove(version)} disabled={busy}
                      className="w-full px-3 py-1.5 bg-red-600 hover:bg-red-700 disabled:bg-slate-300 text-white text-sm rounded">
                      {busy ? 'Removing…' : '🗑 Remove'}
                    </button>
                  )
                ) : (
                  <button onClick={() => install(version)} disabled={busy}
                    className="w-full px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded">
                    {busy ? '⏳ Installing…' : '⬇ Install'}
                  </button>
                )}
              </div>
            )
          })}
        </div>
      )}

      {output && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => setOutput(null)}>
          <div className="bg-white dark:bg-slate-800 rounded-2xl w-full shadow-xl flex flex-col max-h-[80vh]" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between px-4 py-3 border-b border-slate-200 dark:border-slate-700">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{output.title}</h3>
              <button onClick={() => setOutput(null)} className="text-slate-400 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300">×</button>
            </div>
            <pre className="flex-1 overflow-auto p-3 bg-slate-900 text-slate-100 text-xs font-mono whitespace-pre-wrap">{output.output}</pre>
            <div className="px-4 py-2 border-t border-slate-200 dark:border-slate-700 text-right">
              <button onClick={() => setOutput(null)}
                className="px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded">Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}