import { useEffect, useState } from 'react'
import { api } from '@/lib/api'

type Limit = { usage: number; limit: number }
export type Summary = {
  domain_name: string; sk: string; plan_name: string; php_version: string
  ipv4: string; ssl_enabled: boolean; ssl_expiry?: string
  disk_mb: Limit; traffic_mb: Limit
  db_count: Limit; ftp_count: Limit; email_count: Limit; domain_count: Limit
  dns_record: number; cron_job: number
  backup_count: number; backup_mb: number
}

export default function DomainResourceCard({ domainId }: { domainId: number | string }) {
  const [summary, setSummary] = useState<Summary | null>(null)
  const [loading, setLoading] = useState(true)

  function load() {
    setLoading(true)
    api.get<Summary>(`/domains/${domainId}/resources`)
      .then(r => setSummary(r.data))
      .catch(() => setSummary(null))
      .finally(() => setLoading(false))
  }
  useEffect(load, [domainId])

  if (loading) {
    return (
      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <div className="h-5 bg-slate-100 dark:bg-slate-800 rounded w-32 mb-3 animate-pulse" />
        <div className="space-y-3">
          {[1, 2, 3, 4].map(i => (
            <div key={i} className="h-3 bg-slate-100 dark:bg-slate-800 rounded animate-pulse" />
          ))}
        </div>
      </div>
    )
  }
  if (!summary) return null

  return (
    <div className="space-y-3">
      {/* Plan + Summary */}
      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Plan and Resources</h3>
          <button onClick={load} className="text-slate-400 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300" title="Refresh">
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
        </div>

        <div className="mb-3 pb-3 border-b border-slate-100 dark:border-slate-800">
          <div className="text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-500 mb-0.5">Service Plan</div>
          <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{summary.plan_name}</div>
        </div>

        <Bar label="Disk" usage={summary.disk_mb.usage} limit={summary.disk_mb.limit} unit="MB" color="indigo" />
        <Bar label="Traffic (monthly)" usage={summary.traffic_mb.usage} limit={summary.traffic_mb.limit} unit="MB" color="sky" />
        <Bar label="Database" usage={summary.db_count.usage} limit={summary.db_count.limit} unit="DB" color="emerald" />
        <Bar label="FTP Account" usage={summary.ftp_count.usage} limit={summary.ftp_count.limit} unit="account" color="amber" />
        <Bar label="Email Mailbox" usage={summary.email_count.usage} limit={summary.email_count.limit} unit="mailbox" color="rose" />
        <Bar label="Subdomain" usage={summary.domain_count.usage} limit={summary.domain_count.limit} unit="domain" color="violet" />
      </div>

      {/* Configuration Summary */}
      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-4">
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Configuration</h3>
        <Row label="IP Address" value={summary.ipv4 || '—'} mono />
        <Row label="System User" value={summary.sk} mono />
        <Row label="PHP Version"
          value={<span><span className="font-mono font-medium text-slate-800 dark:text-slate-200">PHP {summary.php_version}</span></span>}
        />
        <Row label="SSL/TLS"
          value={
            summary.ssl_enabled
              ? <span className="flex items-center gap-1.5">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" />
                  <span className="text-emerald-700 dark:text-emerald-300 text-xs font-medium">Active</span>
                  {summary.ssl_expiry && <span className="text-slate-400 dark:text-slate-500 text-[10px]">→ {summary.ssl_expiry}</span>}
                </span>
              : <span className="flex items-center gap-1.5">
                  <span className="w-1.5 h-1.5 rounded-full bg-slate-300" />
                  <span className="text-slate-500 dark:text-slate-500 text-xs">None</span>
                </span>
          }
        />
      </div>

      {/* Additional Counters */}
      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-4">
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Counters</h3>
        <div className="grid grid-cols-2 gap-y-2 gap-x-3">
          <Mini label="DNS record" value={summary.dns_record} />
          <Mini label="Cron job" value={summary.cron_job} />
          <Mini label="Backup" value={summary.backup_count} />
          <Mini label="Backup size" value={`${summary.backup_mb} MB`} />
        </div>
      </div>
    </div>
  )
}

// ----- helpers -----
// Live gradient bars. For unlimited resources (limit=0), instead of a faint 8% stub,
// render a full-width gradient that fades to the right → conveys "unlimited/flowing" feel.
function Bar({ label, usage, limit, unit, color }: { label: string; usage: number; limit: number; unit: string; color: string }) {
  const unlimited = limit === 0
  const percentage = unlimited ? 0 : Math.min(100, Math.round((usage / limit) * 100))
  const grad: Record<string, string> = {
    indigo:  'from-indigo-400 to-indigo-600',
    sky:     'from-sky-400 to-sky-600',
    emerald: 'from-emerald-400 to-emerald-600',
    amber:   'from-amber-400 to-amber-600',
    rose:    'from-rose-400 to-rose-600',
    violet:  'from-violet-400 to-violet-600',
  }
  const fill = percentage >= 90 ? 'from-red-400 to-red-600' : (percentage >= 75 ? 'from-amber-400 to-amber-600' : (grad[color] || 'from-slate-400 to-slate-600'))
  const fade = 'linear-gradient(to right, black 0%, black 35%, transparent 96%)'
  return (
    <div className="mb-3 last:mb-0">
      <div className="flex items-baseline justify-between mb-1">
        <span className="text-xs font-medium text-slate-600 dark:text-slate-300">{label}</span>
        <span className="text-[11px] font-mono text-slate-500 dark:text-slate-400">
          {unlimited
            ? <><span className="text-slate-700 dark:text-slate-200 font-semibold">{formatCount(usage)}</span> {unit} · <span className="text-emerald-500 font-bold">∞</span></>
            : <><span className="text-slate-700 dark:text-slate-200 font-semibold">{formatCount(usage)}</span> / {formatCount(limit)} {unit}</>
          }
        </span>
      </div>
      <div className="h-2 rounded-full bg-slate-100 dark:bg-slate-700/50 overflow-hidden">
        {unlimited ? (
          <div
            className={`h-full rounded-full bg-gradient-to-r ${grad[color] || 'from-slate-400 to-slate-600'}`}
            style={{ width: '100%', maskImage: fade, WebkitMaskImage: fade }}
          />
        ) : (
          <div className={`h-full rounded-full bg-gradient-to-r ${fill}`} style={{ width: Math.max(percentage, 3) + '%' }} />
        )}
      </div>
    </div>
  )
}
function formatCount(value: number) {
  if (value >= 1024) return (value / 1024).toFixed(1) + 'k'
  return String(value)
}
function Row({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between py-1.5 border-b border-slate-50 dark:border-slate-800 last:border-0">
      <span className="text-xs text-slate-500 dark:text-slate-500">{label}</span>
      <span className={`text-xs text-slate-700 dark:text-slate-300 text-right ${mono ? 'font-mono' : ''} max-w-[60%] truncate`} title={typeof value === 'string' ? value : undefined}>{value}</span>
    </div>
  )
}
function Mini({ label, value }: { label: string; value: number | string }) {
  return (
    <div>
      <div className="text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-500">{label}</div>
      <div className="text-sm font-mono font-medium text-slate-800 dark:text-slate-200">{value}</div>
    </div>
  )
}
