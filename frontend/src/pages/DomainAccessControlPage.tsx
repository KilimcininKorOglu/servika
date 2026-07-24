import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type HotlinkSettings = {
  active: boolean
  allowed: string[]
}

type IPAccessMode = 'off' | 'block' | 'allow'

type IPRule = {
  id: number
  ip_cidr: string
  created_at: string
}

type IPRulesResponse = {
  mode: IPAccessMode
  rules: IPRule[]
}

const MODE_DETAILS: Record<IPAccessMode, { label: string; description: string; badge: string }> = {
  off: {
    label: 'Off',
    description: 'No IP allow or block rules are rendered into the nginx vhost.',
    badge: 'bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300',
  },
  block: {
    label: 'Block listed IPs',
    description: 'Visitors from listed IP addresses or CIDR ranges receive 403 responses.',
    badge: 'bg-rose-100 dark:bg-rose-900/30 text-rose-700 dark:text-rose-300',
  },
  allow: {
    label: 'Allow listed IPs only',
    description: 'Only listed IP addresses or CIDR ranges are allowed when at least one rule exists.',
    badge: 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300',
  },
}

export default function DomainAccessControlPage() {
  const { id } = useParams()
  const [hotlink, setHotlink] = useState<HotlinkSettings>({ active: false, allowed: [] })
  const [allowedInput, setAllowedInput] = useState('')
  const [mode, setMode] = useState<IPAccessMode>('off')
  const [rules, setRules] = useState<IPRule[]>([])
  const [newRule, setNewRule] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  function load() {
    if (!id) return
    setLoading(true)
    setError(null)
    Promise.all([
      api.get<HotlinkSettings>(`/domains/${id}/hotlink`),
      api.get<IPRulesResponse>(`/domains/${id}/ip-rules`),
    ]).then(([hotlinkResponse, rulesResponse]) => {
      const nextHotlink = hotlinkResponse.data || { active: false, allowed: [] }
      setHotlink(nextHotlink)
      setAllowedInput((nextHotlink.allowed || []).join('\n'))
      setMode(rulesResponse.data?.mode || 'off')
      setRules(rulesResponse.data?.rules || [])
    }).catch(error => setError(apiError(error))).finally(() => setLoading(false))
  }

  useEffect(load, [id])

  function parseAllowedDomains() {
    return allowedInput
      .split(/[\n,]+/)
      .map(item => item.trim().toLowerCase())
      .filter(Boolean)
  }

  async function saveHotlink(event: React.FormEvent) {
    event.preventDefault()
    setError(null); setSuccess(null); setBusy(true)
    try {
      const allowed = parseAllowedDomains()
      await api.put(`/domains/${id}/hotlink`, { active: hotlink.active, allowed })
      setSuccess('Hotlink protection settings were saved.')
      load()
    } catch (error) { setError(apiError(error, 'Could not save hotlink settings')) }
    finally { setBusy(false) }
  }

  async function saveMode(nextMode = mode) {
    setError(null); setSuccess(null); setBusy(true)
    try {
      await api.put(`/domains/${id}/ip-rules/mode`, { mode: nextMode })
      setMode(nextMode)
      setSuccess('IP access mode was saved.')
      load()
    } catch (error) { setError(apiError(error, 'Could not save IP access mode')) }
    finally { setBusy(false) }
  }

  async function addRule(event: React.FormEvent) {
    event.preventDefault()
    setError(null); setSuccess(null); setBusy(true)
    try {
      await api.post(`/domains/${id}/ip-rules`, { ip_cidr: newRule.trim() })
      setNewRule('')
      setSuccess('IP rule was added.')
      load()
    } catch (error) { setError(apiError(error, 'Could not add IP rule')) }
    finally { setBusy(false) }
  }

  async function deleteRule(rule: IPRule) {
    if (!confirm(`Delete IP rule ${rule.ip_cidr}?`)) return
    setError(null); setSuccess(null); setBusy(true)
    try {
      await api.delete(`/domains/${id}/ip-rules/${rule.id}`)
      setSuccess('IP rule was deleted.')
      load()
    } catch (error) { setError(apiError(error, 'Could not delete IP rule')) }
    finally { setBusy(false) }
  }

  const selectedMode = MODE_DETAILS[mode]

  return (
    <div className="px-6 py-5 max-w-[1100px]">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Domains', href: '/domains' },
        { label: 'Access Control' },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🚫</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Access Control</h1>
        <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${selectedMode.badge}`}>{selectedMode.label}</span>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">
        Configure hotlink protection and domain-level IP allow or block rules. Saving re-renders the nginx vhost.
      </p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {loading ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div>
      ) : (
        <>
          <form onSubmit={saveHotlink} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-5">
            <div className="flex flex-wrap items-start justify-between gap-3 mb-3">
              <div>
                <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">Hotlink Protection</h3>
                <p className="text-xs text-slate-500 dark:text-slate-400">Blocks external sites from embedding image files served by this domain.</p>
              </div>
              <label className="flex items-center gap-2 px-3 py-2 border border-slate-200 dark:border-slate-700 rounded-lg text-sm text-slate-600 dark:text-slate-300">
                <input type="checkbox" checked={hotlink.active} onChange={event => setHotlink({ ...hotlink, active: event.target.checked })} />
                Enabled
              </label>
            </div>
            <label className="block">
              <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Additional allowed referrer domains</span>
              <textarea value={allowedInput} onChange={event => setAllowedInput(event.target.value)} rows={4} placeholder={'cdn.example.com\n*.partner.example'}
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            </label>
            <p className="text-[11px] text-slate-400 mt-2">The current domain and its www host are always allowed. Add one domain per line or comma-separated.</p>
            <button disabled={busy} className="mt-3 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
              {busy ? 'Saving…' : 'Save Hotlink Settings'}
            </button>
          </form>

          <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-5">
            <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">IP Access Mode</h3>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
              {(Object.keys(MODE_DETAILS) as IPAccessMode[]).map(item => {
                const active = mode === item
                return (
                  <button key={item} type="button" onClick={() => saveMode(item)} disabled={busy}
                    className={`text-left p-4 border rounded-xl transition disabled:opacity-60 ${active ? 'border-slate-900 dark:border-slate-100 bg-slate-50 dark:bg-slate-900/40' : 'border-slate-200 dark:border-slate-700 hover:border-slate-400'}`}>
                    <div className="flex items-center justify-between gap-2 mb-1">
                      <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{MODE_DETAILS[item].label}</span>
                      {active && <span className="text-[10px] uppercase tracking-wider font-semibold text-slate-500 dark:text-slate-400">● Selected</span>}
                    </div>
                    <p className="text-[11px] text-slate-600 dark:text-slate-400 leading-snug">{MODE_DETAILS[item].description}</p>
                  </button>
                )
              })}
            </div>
          </div>

          <form onSubmit={addRule} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-5">
            <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">New IP Rule</h3>
            <div className="flex flex-wrap items-end gap-2">
              <label className="block flex-1 min-w-[260px]">
                <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">IP address or CIDR</span>
                <input value={newRule} onChange={event => setNewRule(event.target.value)} required placeholder="203.0.113.10 or 203.0.113.0/24"
                  className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
              </label>
              <button disabled={busy || !newRule.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                {busy ? 'Adding…' : 'Add Rule'}
              </button>
            </div>
          </form>

          <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4">
            <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">Configured IP Rules</h3>
            {rules.length === 0 ? (
              <div className="text-center py-6">
                <div className="text-2xl mb-1">🧱</div>
                <p className="text-sm text-slate-500 dark:text-slate-400">No IP rules yet.</p>
              </div>
            ) : (
              <ul className="divide-y divide-slate-100 dark:divide-slate-700/60">
                {rules.map(rule => (
                  <li key={rule.id} className="flex items-center justify-between gap-3 py-2.5">
                    <div className="min-w-0">
                      <div className="font-mono text-sm text-slate-800 dark:text-slate-200">{rule.ip_cidr}</div>
                      <div className="text-[11px] text-slate-400">Created {rule.created_at}</div>
                    </div>
                    <button onClick={() => deleteRule(rule)} disabled={busy} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">Delete</button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to subscription</Link></div>
        </>
      )}
    </div>
  )
}
