import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type AddonDomain = {
  id: number
  domain_name: string
  parked: boolean
  docroot: string
  php_version: string
  ssl: boolean
  created_at: string
}

type RedirectStatus = {
  active: boolean
  target_url?: string
  status_code?: number
}

export default function DomainAddonDomainsPage() {
  const { id } = useParams()
  const [addons, setAddons] = useState<AddonDomain[]>([])
  const [redirect, setRedirect] = useState<RedirectStatus>({ active: false })
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [redirectSaving, setRedirectSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [domainName, setDomainName] = useState('')
  const [parked, setParked] = useState(false)
  const [targetURL, setTargetURL] = useState('')
  const [statusCode, setStatusCode] = useState(301)

  function load() {
    if (!id) return
    setLoading(true)
    Promise.all([
      api.get<AddonDomain[]>(`/domains/${id}/addon-domains`),
      api.get<RedirectStatus>(`/domains/${id}/redirect`),
    ]).then(([addonsResponse, redirectResponse]) => {
      setAddons(addonsResponse.data || [])
      setRedirect(redirectResponse.data || { active: false })
      setTargetURL(redirectResponse.data?.target_url || '')
      setStatusCode(redirectResponse.data?.status_code || 301)
    }).catch(error => setError(apiError(error))).finally(() => setLoading(false))
  }

  useEffect(load, [id])

  async function create(event: React.FormEvent) {
    event.preventDefault()
    setError(null); setSuccess(null); setSaving(true)
    try {
      const { data } = await api.post<AddonDomain>(`/domains/${id}/addon-domains`, { domain_name: domainName.trim(), parked })
      setSuccess(`${data.domain_name} was created.`)
      setDomainName('')
      setParked(false)
      load()
    } catch (error) { setError(apiError(error, 'Could not create addon domain')) }
    finally { setSaving(false) }
  }

  async function remove(addon: AddonDomain) {
    const mode = addon.parked ? 'parked domain' : 'addon domain'
    if (!confirm(`Delete ${addon.domain_name}?
The ${mode}, vhost, DNS zone, and panel records will be removed. This cannot be undone.`)) return
    setError(null); setSuccess(null)
    try {
      await api.delete(`/domains/${id}/addon-domains/${addon.id}`)
      setSuccess(`${addon.domain_name} was deleted.`)
      load()
    } catch (error) { setError(apiError(error, 'Could not delete addon domain')) }
  }

  async function saveRedirect(event: React.FormEvent) {
    event.preventDefault()
    setError(null); setSuccess(null); setRedirectSaving(true)
    try {
      await api.put(`/domains/${id}/redirect`, { target_url: targetURL.trim(), status_code: statusCode })
      setSuccess('Redirect settings were saved.')
      load()
    } catch (error) { setError(apiError(error, 'Could not save redirect settings')) }
    finally { setRedirectSaving(false) }
  }

  async function deleteRedirect() {
    if (!confirm('Remove the domain redirect?')) return
    setError(null); setSuccess(null); setRedirectSaving(true)
    try {
      await api.delete(`/domains/${id}/redirect`)
      setSuccess('Redirect settings were removed.')
      setTargetURL('')
      setStatusCode(301)
      load()
    } catch (error) { setError(apiError(error, 'Could not remove redirect settings')) }
    finally { setRedirectSaving(false) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Domains', href: '/domains' },
        { label: 'Addon Domains' },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🌐</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Addon Domains</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">Create addon or parked domains and configure whole-domain redirects.</p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      <form onSubmit={create} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-5">
        <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">New Addon Domain</h3>
        <div className="flex flex-wrap items-end gap-2">
          <label className="block">
            <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Domain</span>
            <input value={domainName} onChange={event => setDomainName(event.target.value.toLowerCase())} required placeholder="example.com"
              className="mt-1 w-64 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          </label>
          <label className="flex items-center gap-2 px-3 py-2 border border-slate-200 dark:border-slate-700 rounded-lg text-sm text-slate-600 dark:text-slate-300">
            <input type="checkbox" checked={parked} onChange={event => setParked(event.target.checked)} />
            Parked domain
          </label>
          <button disabled={saving || !domainName.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
            {saving ? 'Creating…' : 'Add Domain'}
          </button>
        </div>
        <p className="text-[11px] text-slate-400 mt-2">Addon domains use their own document root. Parked domains share the parent domain document root.</p>
      </form>

      <form onSubmit={saveRedirect} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-5">
        <div className="flex items-center justify-between gap-3 mb-3">
          <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Whole-Domain Redirect</h3>
          {redirect.active && <span className="text-xs px-2 py-1 rounded-full bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300">Active</span>}
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <label className="block flex-1 min-w-[260px]">
            <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Target URL</span>
            <input value={targetURL} onChange={event => setTargetURL(event.target.value)} required placeholder="https://example.com"
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          </label>
          <label className="block">
            <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Status</span>
            <select value={statusCode} onChange={event => setStatusCode(Number(event.target.value))}
              className="mt-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none">
              <option value={301}>301 Permanent</option>
              <option value={302}>302 Temporary</option>
            </select>
          </label>
          <button disabled={redirectSaving || !targetURL.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
            {redirectSaving ? 'Saving…' : 'Save Redirect'}
          </button>
          {redirect.active && <button type="button" onClick={deleteRedirect} disabled={redirectSaving} className="px-4 py-2 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 text-sm font-medium rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">Remove</button>}
        </div>
      </form>

      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4">
        <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">Existing Domains</h3>
        {loading ? <div className="text-sm text-slate-400 py-2">Loading…</div>
          : addons.length === 0 ? (
            <div className="text-center py-6">
              <div className="text-2xl mb-1">🌐</div>
              <p className="text-sm text-slate-500 dark:text-slate-400">No addon domains yet.</p>
            </div>
          ) : (
            <ul className="divide-y divide-slate-100 dark:divide-slate-700/60">
              {addons.map(addon => (
                <li key={addon.id} className="flex items-center justify-between gap-3 py-2.5">
                  <div className="min-w-0">
                    <a href={`http://${addon.domain_name}`} target="_blank" rel="noreferrer" className="font-mono text-sm text-brand-600 dark:text-brand-400 hover:underline">{addon.domain_name}</a>
                    <div className="text-[11px] text-slate-400 font-mono truncate">{addon.parked ? 'Parked' : addon.docroot} · PHP {addon.php_version}</div>
                  </div>
                  <button onClick={() => remove(addon)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20">Delete</button>
                </li>
              ))}
            </ul>
          )}
      </div>

      <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to subscription</Link></div>
    </div>
  )
}
