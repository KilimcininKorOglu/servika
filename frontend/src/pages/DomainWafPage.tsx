import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Mode = 'inherit' | 'off' | 'block' | 'detect'
type Settings = { mode: Mode; paranoia: number }
type PlanInfo = { active: boolean; mode: string; paranoia: number; name?: string }
type Effective = { active: boolean; engine: string; paranoia: number }
type Response = {
  domain_name: string
  settings: Settings
  plan: PlanInfo
  effective: Effective
  module_loaded: boolean
}

const MODES: { key: Mode; name: string; icon: string; description: string; color: string }[] = [
  { key: 'inherit', name: 'Inherit from Plan', icon: '↩︎',
    description: 'This domain uses the WAF default from its assigned service plan.', color: 'slate' },
  { key: 'block', name: 'Block', icon: '🛡️',
    description: 'Malicious requests (SQLi, XSS, RCE…) are blocked with 403. SecRuleEngine On.', color: 'emerald' },
  { key: 'detect', name: 'Detect', icon: '👁️',
    description: 'Requests are not blocked; matching rules are written to the audit log only. DetectionOnly — ideal for rule tuning.', color: 'indigo' },
  { key: 'off', name: 'Off', icon: '⛔',
    description: 'WAF is completely disabled for this domain (even if the plan has it enabled).', color: 'rose' },
]

const PARANOIA_DESCRIPTION: Record<number, string> = {
  0: 'Plan default is used.',
  1: 'Low — basic attack signatures. Almost no false positives. (recommended)',
  2: 'Medium — more rules. Some legitimate requests may be blocked.',
  3: 'High — aggressive. Per-application exclusions may be needed.',
  4: 'Strict — most aggressive. Only for tightly audited scenarios.',
}

export default function DomainWafPage() {
  const { id } = useParams()
  const [data, setData] = useState<Response | null>(null)
  const [settings, setSettings] = useState<Settings | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  function load() {
    if (!id) return
    setLoading(true); setError(null)
    api.get<Response>(`/domains/${id}/waf`)
      .then(r => { setData(r.data); setSettings(r.data.settings) })
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [id])

  async function save() {
    if (!settings) return
    setSaving(true); setError(null); setSuccess(null)
    try {
      const r = await api.put<{ effective: Effective; module_loaded: boolean }>(`/domains/${id}/waf`, { settings })
      const ef = r.data.effective
      setSuccess(ef.active
        ? `WAF applied — ${ef.engine === 'On' ? 'Blocking' : 'Detection'} mode, paranoia ${ef.paranoia}`
        : 'Settings saved — WAF is passive for this domain')
      load()
    } catch (e) {
      setError(apiError(e, 'Save failed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="px-6 py-5 max-w-[1100px]">
      <Breadcrumb items={[
        { label: 'Home', href: '/' }, { label: 'Domains', href: '/domains' },
        { label: data?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: 'Web Application Firewall (WAF)' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Web Application Firewall</h1>
      {data && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        <Link to={`/subscriptions/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 font-medium">{data.domain_name}</Link>
        {' · '}ModSecurity v3 + OWASP Core Rule Set. Saving re-renders the nginx vhost (zero downtime).
      </p>}

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {data && !data.module_loaded && (
        <div className="mb-5 px-3 py-2.5 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-md text-xs text-amber-800 dark:text-amber-200">
          <strong>The ModSecurity module is not installed on this server.</strong> Settings are saved but WAF is not applied.
          Run <code className="font-mono">servika-waf-setup</code> on the server to enable it (existing sites are not affected).
        </div>
      )}

      {loading || !settings || !data ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div>
      ) : (
        <>
          {/* Effective status + plan info */}
          <div className="mb-4 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
            <div className="flex flex-wrap items-center gap-3">
              <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">Effective Status:</span>
              {data.effective.active ? (
                <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold ${
                  data.effective.engine === 'On'
                    ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300'
                    : 'bg-indigo-100 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-300'
                }`}>
                  {'●'} {data.effective.engine === 'On' ? 'Active · Blocking' : 'Active · Detection'} · Paranoia {data.effective.paranoia}
                </span>
              ) : (
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400">{'○'} Passive</span>
              )}
              <span className="text-xs text-slate-400 dark:text-slate-500 ml-auto">
                Plan default ({data.plan.name || '—'}):{' '}
                {data.plan.active ? `${data.plan.mode === 'detect' ? 'Detect' : 'Block'} · PL${data.plan.paranoia}` : 'Off'}
              </span>
            </div>
          </div>

          {/* Mode selector */}
          <Card title="WAF Mode">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {MODES.map(m => {
                const active = settings.mode === m.key
                const colors: Record<string, string> = {
                  slate:   active ? 'border-slate-500 bg-slate-100 dark:bg-slate-700/40 ring-2 ring-slate-400/20' : 'border-slate-200 dark:border-slate-700 hover:border-slate-400',
                  emerald: active ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/20 ring-2 ring-emerald-500/20' : 'border-slate-200 dark:border-slate-700 hover:border-emerald-300',
                  indigo:  active ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-900/20 ring-2 ring-indigo-500/20' : 'border-slate-200 dark:border-slate-700 hover:border-indigo-300',
                  rose:    active ? 'border-rose-500 bg-rose-50 dark:bg-rose-900/20 ring-2 ring-rose-500/20' : 'border-slate-200 dark:border-slate-700 hover:border-rose-300',
                }
                return (
                  <button key={m.key} type="button" onClick={() => setSettings({ ...settings, mode: m.key })}
                    className={`text-left p-4 border rounded-xl transition ${colors[m.color]}`}>
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{m.icon} {m.name}</span>
                      {active && <span className="text-[10px] uppercase tracking-wider font-semibold text-slate-500 dark:text-slate-400">{'●'} Selected</span>}
                    </div>
                    <div className="text-[11px] text-slate-600 dark:text-slate-400 leading-snug">{m.description}</div>
                  </button>
                )
              })}
            </div>
          </Card>

          {/* Paranoia */}
          <Card title="Paranoia Level (CRS)">
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-3">
              A higher level = more rules + stronger protection, but also a higher chance of false positives.
              Only effective when WAF is in <strong>Block</strong> or <strong>Detect</strong> mode.
            </p>
            <div className="flex items-center gap-3">
              <select
                value={settings.paranoia}
                onChange={e => setSettings({ ...settings, paranoia: parseInt(e.target.value) })}
                disabled={settings.mode === 'inherit' || settings.mode === 'off'}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-800 rounded text-sm font-mono disabled:opacity-50">
                <option value={0}>Inherit from plan</option>
                <option value={1}>Level 1 (Low)</option>
                <option value={2}>Level 2 (Medium)</option>
                <option value={3}>Level 3 (High)</option>
                <option value={4}>Level 4 (Strict)</option>
              </select>
              <span className="text-xs text-slate-500 dark:text-slate-400">{PARANOIA_DESCRIPTION[settings.paranoia]}</span>
            </div>
          </Card>

          <div className="flex gap-3 mt-6">
            <button onClick={save} disabled={saving}
              className="px-6 py-2.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">
              {saving ? 'Applying…' : 'Save and Apply'}
            </button>
            <button onClick={load} disabled={saving}
              className="px-4 py-2.5 border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 text-sm rounded-md">
              Reload
            </button>
          </div>
        </>
      )}
    </div>
  )
}

function Card({ title, children }: { title: string; children: any }) {
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4">
      <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-3 pb-2 border-b border-slate-100 dark:border-slate-800">{title}</h3>
      {children}
    </div>
  )
}
