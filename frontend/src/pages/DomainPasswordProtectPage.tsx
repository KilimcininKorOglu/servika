import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Protection = { id: number; path: string; username: string; created_at: string }

export default function DomainPasswordProtectPage() {
  const { id } = useParams()
  const [protections, setProtections] = useState<Protection[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [path, setPath] = useState('/private')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [isSaving, setIsSaving] = useState(false)

  function load() {
    if (!id) return
    setLoading(true)
    api.get<Protection[]>(`/domains/${id}/protection`)
      .then(r => setProtections(r.data || [])).catch(e => setError(apiError(e))).finally(() => setLoading(false))
  }
  useEffect(load, [id])

  async function add(e: React.FormEvent) {
    e.preventDefault()
    setError(null); setSuccess(null); setIsSaving(true)
    try {
      await api.post(`/domains/${id}/protection`, { path, username, password })
      setSuccess(`${path} is now protected with the user "${username}".`)
      setPassword('')
      load()
    } catch (err) {
      setError(apiError(err, 'Could not add protection'))
    } finally { setIsSaving(false) }
  }

  async function remove(k: Protection) {
    if (!confirm(`Remove user "${k.username}" from the protection on ${k.path}?`)) return
    setError(null); setSuccess(null)
    try {
      await api.delete(`/domains/${id}/protection/${k.id}`)
      load()
    } catch (err) { setError(apiError(err, 'Could not remove protection')) }
  }

  // Group users by their protected path.
  const groups = protections.reduce<Record<string, Protection[]>>((a, k) => { (a[k.path] ||= []).push(k); return a }, {})

  return (
    <div className="px-6 py-5">
      <div className="max-w-3xl mx-auto">
        <Breadcrumb items={[
          { label: 'Home', href: '/' },
          { label: 'Domains', href: '/domains' },
          { label: 'Password-Protected Directories' },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Password-Protected Directories</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          Protect a directory with HTTP authentication (<span className="font-mono">.htpasswd</span>). Visitors cannot access it without a username and password.
        </p>

        {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
        {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

        {/* Add-protection form */}
        <form onSubmit={add} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Add Protection or User</h3>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <label className="block">
              <span className="text-xs text-slate-500 dark:text-slate-400">Directory path</span>
              <input value={path} onChange={e => setPath(e.target.value)} required placeholder="/private"
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            </label>
            <label className="block">
              <span className="text-xs text-slate-500 dark:text-slate-400">Username</span>
              <input value={username} onChange={e => setUsername(e.target.value)} required placeholder="username"
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            </label>
            <label className="block">
              <span className="text-xs text-slate-500 dark:text-slate-400">Password</span>
              <input value={password} onChange={e => setPassword(e.target.value)} required type="password" placeholder="••••••••"
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            </label>
          </div>
          <p className="text-[11px] text-slate-400 mt-2">The path must start with <span className="font-mono">/</span> (for example, <span className="font-mono">/private</span> or <span className="font-mono">/admin</span>). You can add multiple users to the same path.</p>
          <button disabled={isSaving} className="mt-3 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
            {isSaving ? 'Adding…' : 'Add Protection'}
          </button>
        </form>

        {/* Existing protections */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Protected Directories</h3>
          {loading ? (
            <div className="text-sm text-slate-400">Loading…</div>
          ) : protections.length === 0 ? (
            <div className="text-center py-8">
              <div className="text-3xl mb-2">🔒</div>
              <p className="text-sm text-slate-500 dark:text-slate-400">No protected directories yet.</p>
            </div>
          ) : (
            <div className="space-y-4">
              {Object.entries(groups).map(([g, ks]) => (
                <div key={g} className="border border-slate-100 dark:border-slate-700 rounded-lg overflow-hidden">
                  <div className="flex items-center gap-2 px-3 py-2 bg-slate-50 dark:bg-slate-900/40">
                    <span className="text-sm">🔒</span>
                    <span className="font-mono text-sm text-slate-700 dark:text-slate-200">{g}</span>
                    <span className="text-xs text-slate-400">· {ks.length} users</span>
                  </div>
                  <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                    {ks.map(k => (
                      <li key={k.id} className="flex items-center justify-between px-3 py-2">
                        <span className="text-sm text-slate-600 dark:text-slate-300">{k.username}</span>
                        <button onClick={() => remove(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">Remove</button>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to subscription</Link></div>
      </div>
    </div>
  )
}
