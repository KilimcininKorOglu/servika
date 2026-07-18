import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Status = {
  domain_name: string
  username: string
  active: boolean
  shell: string
  ssh_host: string
  ssh_port: number
  has_key: boolean
  is_demo: boolean
}

export default function DomainSSHPage() {
  const { id } = useParams()
  const [status, setStatus] = useState<Status | null>(null)
  const [loading, setLoading] = useState(true)
  const [isProcessing, setIsProcessing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [key, setKey] = useState('')

  function load() {
    if (!id) return
    setLoading(true); setError(null)
    api.get<Status>(`/domains/${id}/ssh`)
      .then(r => setStatus(r.data))
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [id])

  async function toggle(active: boolean) {
    setIsProcessing(true); setError(null); setSuccess(null)
    try {
      await api.put(`/domains/${id}/ssh`, { active: active })
      setSuccess(active ? 'SSH access active.' : 'SSH access disabled.')
      setTimeout(() => setSuccess(null), 4000)
      load()
    } catch (e) {
      setError(apiError(e, 'Operation failed'))
    } finally { setIsProcessing(false) }
  }

  async function saveKey() {
    setIsProcessing(true); setError(null); setSuccess(null)
    try {
      const { data } = await api.put(`/domains/${id}/ssh/key`, { key })
      setSuccess(data.has_key ? 'SSH key saved.' : 'SSH keys cleared.')
      setTimeout(() => setSuccess(null), 4000)
      setKey('')
      load()
    } catch (e) {
      setError(apiError(e, 'Could not save the key'))
    } finally { setIsProcessing(false) }
  }

  if (loading) return <div className="px-6 py-5 text-slate-400">Loading…</div>
  if (!status) return <div className="px-6 py-5"><div className="text-sm text-red-600">{error || 'Not found'}</div></div>

  const sshCommand = `ssh ${status.username}@${status.ssh_host} -p ${status.ssh_port}`

  return (
    <div className="px-6 py-5">
      <div className="max-w-3xl mx-auto">
        <Breadcrumb items={[
          { label: 'Home', href: '/' },
          { label: 'Domains', href: '/domains' },
          { label: status.domain_name, href: `/subscriptions/${id}` },
          { label: 'SSH Access' },
        ]} />

        <div className="flex items-start justify-between gap-4 mb-1">
          <div>
            <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">SSH Access</h1>
            <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
              <span className="font-mono">{status.domain_name}</span> account shell access.
            </p>
          </div>
          <span className={`shrink-0 inline-flex items-center gap-1.5 text-xs font-semibold px-2.5 py-1 rounded-full ${
            status.active ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-300'
          }`}>
            <span className={`w-2 h-2 rounded-full ${status.active ? 'bg-emerald-500' : 'bg-slate-400'}`} />
            {status.active ? 'SSH ON' : 'SSH OFF'}
          </span>
        </div>

        {error && <div className="my-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{error}</div>}
        {success && <div className="my-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

        {/* Status and toggle */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
          <div className="flex items-center justify-between gap-4">
            <div>
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Shell Access</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                Enabled: <code className="font-mono">/bin/bash</code> · Disabled: <code className="font-mono">/usr/sbin/nologin</code>.
                Current: <code className="font-mono">{status.shell || '—'}</code>
              </p>
            </div>
            {status.active ? (
              <button onClick={() => toggle(false)} disabled={isProcessing || status.is_demo}
                className="shrink-0 px-4 py-2 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 text-sm font-medium rounded-lg">
                Disable SSH
              </button>
            ) : (
              <button onClick={() => toggle(true)} disabled={isProcessing || status.is_demo}
                className="shrink-0 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-lg">
                Enable SSH
              </button>
            )}
          </div>
          {status.is_demo && <p className="mt-3 text-xs text-amber-600 dark:text-amber-400">SSH access cannot be changed for the demo domain.</p>}
        </div>

        {/* Connection details */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Connection Details</h3>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-sm">
            <Info label="User" value={status.username} />
            <Info label="Host" value={status.ssh_host} />
            <Info label="Port" value={String(status.ssh_port)} />
          </div>
          <div className="mt-3">
            <label className="text-xs font-medium text-slate-600 dark:text-slate-400">Connection command</label>
            <div className="mt-1 flex items-center gap-2">
              <code className="flex-1 px-3 py-2 bg-slate-900 text-slate-100 rounded-lg text-xs font-mono overflow-x-auto">{sshCommand}</code>
              <button onClick={() => navigator.clipboard?.writeText(sshCommand)} className="shrink-0 text-xs px-2.5 py-2 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700">Copy</button>
            </div>
          </div>
          <p className="mt-3 text-xs text-slate-500 dark:text-slate-400">Password: <strong>the same as your FTP account</strong>. It is synchronized automatically while SSH is active. Alternatively, add an SSH public key below.</p>
        </div>

        {/* SSH public key */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">SSH Public Key (authorized_keys)</h3>
          <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
            Add your public key to sign in without a password. {status.has_key
              ? <span className="text-emerald-600 dark:text-emerald-400">A key is currently configured.</span>
              : <span className="text-slate-500">No key is configured yet.</span>}
          </p>
          <textarea
            value={key}
            onChange={e => setKey(e.target.value)}
            rows={4}
            spellCheck={false}
            placeholder="ssh-ed25519 AAAA... user@host"
            className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-xs font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none"
          />
          <div className="mt-3 flex items-center justify-between">
            <p className="text-xs text-slate-400">Save an empty value to remove all keys.</p>
            <button onClick={saveKey} disabled={isProcessing || status.is_demo}
              className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-lg">
              Save Key
            </button>
          </div>
        </div>

        <div className="mt-4">
          <Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to subscription</Link>
        </div>
      </div>
    </div>
  )
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="px-3 py-2 bg-slate-50 dark:bg-slate-900/40 rounded-lg border border-slate-200 dark:border-slate-700">
      <div className="text-[10px] uppercase tracking-wider text-slate-400">{label}</div>
      <div className="font-mono text-slate-800 dark:text-slate-200 truncate">{value}</div>
    </div>
  )
}
