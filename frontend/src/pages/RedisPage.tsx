import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Status = {
  enabled: boolean
  host: string
  port: number
  username: string
  password?: string
  prefix: string
  wp_snippet?: string
  wp_connected?: number
}

export default function RedisPage() {
  const { id } = useParams()
  const [status, setStatus] = useState<Status | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [copied, setCopied] = useState<string | null>(null)

  function load() {
    setLoading(true)
    api.get<Status>(`/domains/${id}/redis`)
      .then(response => setStatus(response.data))
      .catch(error => setError(apiError(error)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [id])

  async function enable() {
    setError(null); setSuccess(null); setBusy(true)
    try {
      const { data } = await api.post<Status>(`/domains/${id}/redis`, {})
      setStatus(data)
      setSuccess(data.wp_connected && data.wp_connected > 0
        ? `Redis cache was enabled, and ${data.wp_connected} WordPress installation${data.wp_connected === 1 ? ' was' : 's were'} connected automatically. No additional setup is required.`
        : 'Redis cache was enabled. Configure non-WordPress applications with the connection details below.')
    } catch (error) { setError(apiError(error, 'Could not enable Redis cache')) }
    finally { setBusy(false) }
  }
  async function disable() {
    if (!confirm('Disable Redis cache? The ACL user for this domain will be deleted.')) return
    setError(null); setSuccess(null); setBusy(true)
    try {
      await api.delete(`/domains/${id}/redis`)
      load()
      setSuccess('Redis cache was disabled.')
    } catch (error) { setError(apiError(error, 'Could not disable Redis cache')) }
    finally { setBusy(false) }
  }

  function copy(text: string, label: string) {
    navigator.clipboard?.writeText(text)
    setCopied(label)
    setTimeout(() => setCopied(null), 1500)
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'Domains', href: '/domains' }, { label: 'Redis Cache' }]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">⚡</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Redis Cache</h1>
        {status && (
          <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${status.enabled
            ? 'bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300'
            : 'bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400'}`}>
            {status.enabled ? '● Active' : 'Disabled'}
          </span>
        )}
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
        Allocates an <strong>isolated, dedicated Redis object cache</strong> to this domain. WordPress and dynamic applications can reduce database load and respond faster. Other sites cannot access this cache.
      </p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {loading ? (
        <div className="py-12 text-center text-sm text-slate-400">Loading…</div>
      ) : !status?.enabled ? (
        <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-6 text-center">
          <div className="text-3xl mb-2">⚡</div>
          <p className="text-sm text-slate-600 dark:text-slate-300 mb-1">Redis cache is disabled for this domain.</p>
          <p className="text-xs text-slate-400 mb-4">Enabling it creates an isolated ACL user and connection details.</p>
          <button onClick={enable} disabled={busy}
            className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
            {busy ? 'Enabling…' : 'Enable Redis Cache'}
          </button>
        </div>
      ) : (
        <>
          {/* Connection details */}
          <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden mb-4">
            <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60 flex items-center justify-between">
              <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">Connection Details</h3>
              <button onClick={disable} disabled={busy}
                className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">
                Disable
              </button>
            </div>
            <div className="divide-y divide-slate-100 dark:divide-slate-700/60">
              <CopyRow label="Host" value={`${status.host}:${status.port}`} onCopy={copy} copied={copied} />
              <CopyRow label="Username" value={status.username} onCopy={copy} copied={copied} />
              <CopyRow label="Password" value={status.password || ''} secret onCopy={copy} copied={copied} />
              <CopyRow label="Key prefix" value={status.prefix} onCopy={copy} copied={copied} />
            </div>
          </div>

          {/* WordPress snippet */}
          {status.wp_snippet && (
            <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden">
              <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60 flex items-center justify-between">
                <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">WordPress Setup</h3>
                <button onClick={() => copy(status.wp_snippet!, 'wp')}
                  className="text-xs px-2.5 py-1 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-md">
                  {copied === 'wp' ? 'Copied ✓' : 'Copy'}
                </button>
              </div>
              <div className="p-4">
                <p className="text-xs text-slate-500 dark:text-slate-400 mb-2">
                  1) Add the following lines to your <code className="font-mono bg-slate-100 dark:bg-slate-900 px-1 rounded">wp-config.php</code> file.
                  2) Install the <strong>Redis Object Cache</strong> plugin from the WordPress dashboard, then select "Enable Object Cache."
                </p>
                <pre className="text-[11px] font-mono bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-3 overflow-x-auto text-slate-700 dark:text-slate-200 whitespace-pre">{status.wp_snippet}</pre>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function CopyRow({ label, value, secret, onCopy, copied }: {
  label: string; value: string; secret?: boolean
  onCopy: (text: string, label: string) => void; copied: string | null
}) {
  const [visible, setVisible] = useState(false)
  const displayedValue = secret && !visible ? '•'.repeat(Math.min(value.length, 20)) : value
  return (
    <div className="flex items-center gap-3 px-4 py-2.5">
      <span className="text-xs text-slate-500 dark:text-slate-400 w-28 shrink-0">{label}</span>
      <span className="flex-1 font-mono text-xs text-slate-800 dark:text-slate-200 truncate">{displayedValue}</span>
      {secret && (
        <button onClick={() => setVisible(current => !current)} className="text-xs text-slate-400 hover:text-slate-600 dark:hover:text-slate-200">
          {visible ? 'hide' : 'show'}
        </button>
      )}
      <button onClick={() => onCopy(value, label)}
        className="text-xs px-2 py-0.5 border border-slate-200 dark:border-slate-700 rounded text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">
        {copied === label ? '✓' : 'copy'}
      </button>
    </div>
  )
}
