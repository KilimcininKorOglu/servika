import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type SummaryRow = { domain_id: number; domain_name: string; count: number; total_bytes: number; last_backup: string }
type Summary = { domains: SummaryRow[]; total_size_bytes: number; total_backups: number; destination_count: number; schedule: string }

export default function BackupManagementPage() {
  const [o, setSummary] = useState<Summary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [backingUp, setBackingUp] = useState(false)

  function load() {
    setLoading(true)
    api.get<Summary>('/admin/backups/summary')
      .then(r => setSummary(r.data))
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [])

  async function backupNow() {
    setError(null); setSuccess(null); setBackingUp(true)
    try {
      await api.post('/admin/backups/tick')
      setSuccess('Scheduled backups were triggered. Refresh in a few seconds to see the results.')
    } catch (e) { setError(apiError(e, 'Could not trigger backups')) }
    finally { setBackingUp(false) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Tools and Settings', href: '/tools-settings' },
        { label: 'Backup Manager' },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">💾</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Backup Manager</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">View backups for all domains in one place. Daily automatic backups run, and S3/SFTP destinations are configured per domain.</p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {/* KPI */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
        <Kpi label="Total Backup Size" value={o ? formatBytes(o.total_size_bytes) : '—'} color="sky" icon="💽" />
        <Kpi label="Total Backups" value={o ? String(o.total_backups) : '—'} color="violet" icon="📦" />
        <Kpi label="Domain Count" value={o ? String(o.domains.length) : '—'} color="teal" icon="🌐" />
        <Kpi label="Active Remote Destinations" value={o ? String(o.destination_count) : '—'} color="emerald" icon="☁️" subtitle="S3 / SFTP" />
      </div>

      {/* Schedule and action */}
      <div className="mb-5 flex flex-wrap items-center gap-3 px-4 py-3 rounded-2xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/60">
        <span className="text-sm text-slate-600 dark:text-slate-300">
          🕒 Automatic backups: <strong>{o?.schedule || 'Daily at 03:00'}</strong> · 7-day retention
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button onClick={backupNow} disabled={backingUp}
            className="px-3.5 py-2 text-sm font-medium bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg disabled:opacity-50">
            {backingUp ? 'Triggering…' : '⏱ Back Up All Domains Now'}
          </button>
          <button onClick={load} disabled={loading} className="px-3 py-2 text-sm border border-slate-200 dark:border-slate-700 rounded-lg text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">↻ Refresh</button>
        </div>
      </div>

      {/* Table */}
      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden">
        <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">Domain Backups</h3>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900/50 text-[11px] uppercase tracking-wider text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-700/60">
              <tr>
                <th className="text-left font-medium px-4 py-2.5">Domain</th>
                <th className="text-right font-medium px-4 py-2.5">Backup Count</th>
                <th className="text-right font-medium px-4 py-2.5">Total Size</th>
                <th className="text-left font-medium px-4 py-2.5">Latest Backup</th>
                <th className="text-right font-medium px-4 py-2.5">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-700/60">
              {loading ? (
                <tr><td colSpan={5} className="px-4 py-10 text-center text-sm text-slate-400">Loading…</td></tr>
              ) : !o || o.domains.length === 0 ? (
                <tr><td colSpan={5} className="px-4 py-10 text-center text-sm text-slate-500 dark:text-slate-400">No domains.</td></tr>
              ) : (
                o.domains.map(d => (
                  <tr key={d.domain_id} className="hover:bg-slate-50 dark:hover:bg-slate-800/40">
                    <td className="px-4 py-2.5 font-medium text-slate-800 dark:text-slate-100">{d.domain_name}</td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs text-slate-600 dark:text-slate-300">{d.count}</td>
                    <td className="px-4 py-2.5 text-right font-mono text-xs text-slate-600 dark:text-slate-300">{d.count ? formatBytes(d.total_bytes) : '—'}</td>
                    <td className="px-4 py-2.5 text-xs font-mono text-slate-500 dark:text-slate-400">{d.last_backup || <span className="text-slate-400">never</span>}</td>
                    <td className="px-4 py-2.5 text-right">
                      <Link to={`/subscriptions/${d.domain_id}/backups`} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-brand-600 dark:text-brand-400 hover:bg-slate-50 dark:hover:bg-slate-700">Manage →</Link>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
      <p className="text-xs text-slate-400 dark:text-slate-500 mt-3">
        ℹ️ Backups <span className="font-mono">/var/backups/servika/&lt;domain&gt;/</span> are stored there. Backups remain available if a domain is deleted, allowing recovery from accidental deletion. Open “Manage” to download or restore individual backups and configure destinations.
      </p>
    </div>
  )
}

function Kpi({ label, value, color, icon, subtitle }: { label: string; value: string; color: string; icon: string; subtitle?: string }) {
  const c: Record<string, string> = {
    sky: 'text-sky-600 dark:text-sky-400', violet: 'text-violet-600 dark:text-violet-400',
    teal: 'text-teal-600 dark:text-teal-400', emerald: 'text-emerald-600 dark:text-emerald-400',
  }
  return (
    <div className="rounded-2xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/60 p-4">
      <div className="flex items-center gap-2 text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{icon} {label}</div>
      <div className={`text-2xl font-semibold mt-1 ${c[color] || 'text-slate-700 dark:text-slate-200'}`}>{value}</div>
      {subtitle && <div className="text-[11px] text-slate-400 mt-0.5">{subtitle}</div>}
    </div>
  )
}

function formatBytes(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`
}
