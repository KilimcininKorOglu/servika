import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Copy = { name: string; size_mb: number; date: string }

export default function DomainCopyPage() {
  const { id } = useParams()
  const [list, setCopies] = useState<Copy[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [ok, setOk] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  function load() {
    if (!id) return
    api.get<Copy[]>(`/domains/${id}/copy`).then(r => setCopies(r.data || [])).catch(e => setError(apiError(e))).finally(() => setLoading(false))
  }
  useEffect(load, [id])

  async function create() {
    setError(null); setOk(null); setCreating(true)
    try {
      const { data } = await api.post(`/domains/${id}/copy`, {})
      setOk(`Copy created: ${data.name} (${data.size_mb} MB)`)
      load()
    } catch (e) { setError(apiError(e, 'Could not create copy')) }
    finally { setCreating(false) }
  }

  async function remove(k: Copy) {
    if (!confirm(`Delete this copy?\n${k.name} (${k.size_mb} MB)`)) return
    setError(null); setOk(null)
    try { await api.delete(`/domains/${id}/copy/${k.name}`); load() }
    catch (e) { setError(apiError(e, 'Could not delete copy')) }
  }

  return (
    <div className="px-6 py-5">
      <div className="max-w-3xl mx-auto">
        <Breadcrumb items={[
          { label: 'Home', href: '/' },
          { label: 'Domains', href: '/domains' },
          { label: 'Copy Website' },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Copy Website</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          Creates a timestamped snapshot of your site files under <span className="font-mono">~/copies/</span>, providing a safe restore point before making changes.
        </p>

        {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
        {ok && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{ok}</div>}

        <div className="bg-amber-50 dark:bg-amber-900/10 border border-amber-200 dark:border-amber-800/40 rounded-2xl p-4 mb-4 text-xs text-amber-800 dark:text-amber-300">
          ℹ️ This tool copies only <b>files</b> and does not include the database. For a complete backup, use <b>Backup and Restore</b> .
        </div>

        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5 shadow-sm flex items-center justify-between">
          <div className="text-sm text-slate-600 dark:text-slate-300">Create a new copy of the public_html contents.</div>
          <button onClick={create} disabled={creating}
            className="px-4 py-2 text-sm font-medium bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg disabled:opacity-50">
            {creating ? 'Copying…' : 'Create Copy'}
          </button>
        </div>

        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Existing Copies</h3>
          {loading ? (
            <div className="text-sm text-slate-400">Loading…</div>
          ) : list.length === 0 ? (
            <div className="text-center py-8">
              <div className="text-3xl mb-2">📁</div>
              <p className="text-sm text-slate-500 dark:text-slate-400">No copies yet.</p>
            </div>
          ) : (
            <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
              {list.map(k => (
                <li key={k.name} className="flex items-center justify-between py-2.5">
                  <div>
                    <div className="font-mono text-sm text-slate-700 dark:text-slate-200">{k.name}</div>
                    <div className="text-xs text-slate-400">{k.date} · {k.size_mb} MB</div>
                  </div>
                  <button onClick={() => remove(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">Delete</button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to Subscription</Link></div>
      </div>
    </div>
  )
}
