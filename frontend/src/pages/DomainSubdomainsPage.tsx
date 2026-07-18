import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Sub = { id: number; subdomain: string; fqdn: string; php_version: string; docroot: string; created_at: string }

export default function DomainSubdomainsPage() {
  const { id } = useParams()
  const [subdomains, setSubdomains] = useState<Sub[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [subdomainName, setSubdomainName] = useState('')
  const [saving, setSaving] = useState(false)
  const [sslBusy, setSSLBusy] = useState<number | null>(null)

  function load() {
    if (!id) return
    setLoading(true)
    api.get<Sub[]>(`/domains/${id}/subdomain`).then(response => setSubdomains(response.data || [])).catch(error => setError(apiError(error))).finally(() => setLoading(false))
  }
  useEffect(load, [id])

  async function create(event: React.FormEvent) {
    event.preventDefault()
    setError(null); setSuccess(null); setSaving(true)
    try {
      const { data } = await api.post(`/domains/${id}/subdomain`, { subdomain: subdomainName.trim() })
      setSuccess(`${data.fqdn} was created. A DNS A record was added.`)
      setSubdomainName('')
      load()
    } catch (error) { setError(apiError(error, 'Could not create subdomain')) }
    finally { setSaving(false) }
  }

  async function remove(subdomain: Sub) {
    if (!confirm(`Delete ${subdomain.fqdn}?\nThe vhost, files (document root), and DNS record will be removed. This cannot be undone.`)) return
    setError(null); setSuccess(null)
    try { await api.delete(`/domains/${id}/subdomain/${subdomain.id}`); load() }
    catch (error) { setError(apiError(error, 'Could not delete subdomain')) }
  }

  async function issueSSL(subdomain: Sub, type: 'letsencrypt' | 'self-signed') {
    setError(null); setSuccess(null); setSSLBusy(subdomain.id)
    try {
      await api.post(`/domains/${id}/subdomain/${subdomain.id}/ssl`, { type })
      setSuccess(`${subdomain.fqdn} SSL was installed using ${type === 'letsencrypt' ? "Let's Encrypt" : 'a self-signed certificate'}.`)
    } catch (error) { setError(apiError(error, 'Could not install SSL')) }
    finally { setSSLBusy(null) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Domains', href: '/domains' },
        { label: 'Subdomains' },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🌐</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Subdomains</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">Create subdomains (such as <span className="font-mono">blog.domain.com</span>) under this domain. Each one has a separate web directory.</p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      <form onSubmit={create} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-5">
        <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">New Subdomain</h3>
        <div className="flex flex-wrap items-end gap-2">
          <label className="block">
            <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Subdomain</span>
            <input value={subdomainName} onChange={event => setSubdomainName(event.target.value.toLowerCase())} required placeholder="blog"
              className="mt-1 w-48 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          </label>
          <button disabled={saving || !subdomainName.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
            {saving ? 'Creating…' : 'Add Subdomain'}
          </button>
        </div>
        <p className="text-[11px] text-slate-400 mt-2">Lowercase letters, numbers, and hyphens. Examples: <span className="font-mono">blog</span>, <span className="font-mono">shop</span>, <span className="font-mono">api</span>.</p>
      </form>

      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4">
        <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">Existing Subdomains</h3>
        {loading ? <div className="text-sm text-slate-400 py-2">Loading…</div>
          : subdomains.length === 0 ? (
            <div className="text-center py-6">
              <div className="text-2xl mb-1">🌐</div>
              <p className="text-sm text-slate-500 dark:text-slate-400">No subdomains yet.</p>
            </div>
          ) : (
            <ul className="divide-y divide-slate-100 dark:divide-slate-700/60">
              {subdomains.map(subdomain => (
                <li key={subdomain.id} className="flex items-center justify-between gap-3 py-2.5">
                  <div className="min-w-0">
                    <a href={`http://${subdomain.fqdn}`} target="_blank" rel="noreferrer" className="font-mono text-sm text-brand-600 dark:text-brand-400 hover:underline">{subdomain.fqdn}</a>
                    <div className="text-[11px] text-slate-400 font-mono truncate">{subdomain.docroot} · PHP {subdomain.php_version}</div>
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    <button onClick={() => issueSSL(subdomain, 'letsencrypt')} disabled={sslBusy === subdomain.id} title="Install Let's Encrypt SSL"
                      className="text-xs px-2.5 py-1 border border-emerald-300 dark:border-emerald-800 text-emerald-700 dark:text-emerald-400 rounded-md hover:bg-emerald-50 dark:hover:bg-emerald-900/20 disabled:opacity-50">
                      {sslBusy === subdomain.id ? '…' : "🔒 Let's Encrypt"}
                    </button>
                    <button onClick={() => issueSSL(subdomain, 'self-signed')} disabled={sslBusy === subdomain.id} title="Install self-signed SSL"
                      className="text-xs px-2 py-1 border border-slate-300 dark:border-slate-700 text-slate-500 rounded-md hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-50">
                      Self-signed
                    </button>
                    <button onClick={() => remove(subdomain)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20">Delete</button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        <p className="text-[11px] text-slate-400 mt-3 pt-3 border-t border-slate-100 dark:border-slate-700/60">
          ℹ️ The subdomain is configured on the web server immediately. Your domain's DNS A record must point to this server before it can be accessed.
        </p>
      </div>

      <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to subscription</Link></div>
    </div>
  )
}
