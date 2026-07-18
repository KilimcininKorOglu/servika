import { useEffect, useState } from 'react'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

// The actual nested /system/usage response shape matches MonitoringPage.
type Usage = {
  system?: { hostname?: string; ip?: string; os_name?: string; kernel?: string; arch?: string; cpu_model?: string; cpu_cores?: number; panel_version?: string }
  cpu?: { percent?: number; cores?: number; load_1m?: number; load_5m?: number; load_15m?: number }
  memory?: { total_kb?: number; used_kb?: number; free_kb?: number; percent?: number }
  swap?: { total_kb?: number; used_kb?: number; percent?: number }
  disk?: { total_byte?: number; used_byte?: number; free_byte?: number; percent?: number; mount?: string }
}

type Counts = { domains: number; activeDomains: number }

function formatBytes(bytes: number) {
  if (!bytes || bytes < 0) return '0 B'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 ** 2) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 ** 3) return (bytes / 1024 / 1024).toFixed(1) + ' MB'
  return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

// Convert undefined, null, and NaN values to zero.
function numberOrZero(value: number | undefined | null): number {
  return typeof value === 'number' && isFinite(value) ? value : 0
}

export default function StatisticsPage() {
  const [usage, setUsage] = useState<Usage | null>(null)
  const [counts, setCounts] = useState<Counts | null>(null)
  const [error, setError] = useState<string | null>(null)

  function load() {
    api.get<Usage>('/system/usage').then(response => setUsage(response.data)).catch(caughtError => setError(apiError(caughtError)))
    api.get('/domains').then(response => {
      const domains = (response.data as any[]) || []
      setCounts({
        domains: domains.length,
        activeDomains: domains.filter((domain: any) => domain.status === 'active').length,
      })
    }).catch(() => {})
  }
  useEffect(() => { load(); const timer = setInterval(load, 10000); return () => clearInterval(timer) }, [])

  const cpu = numberOrZero(usage?.cpu?.percent)
  const memory = numberOrZero(usage?.memory?.percent)
  const disk = numberOrZero(usage?.disk?.percent)
  const cores = numberOrZero(usage?.cpu?.cores) || numberOrZero(usage?.system?.cpu_cores) || 1
  const oneMinuteLoad = numberOrZero(usage?.cpu?.load_1m)

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Statistics' },
      ]} />
      <div className="flex items-center justify-between mb-1">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Statistics</h1>
        <span className="text-xs text-emerald-600 dark:text-emerald-400 font-medium">● Live (10 sec)</span>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">Server resource usage, domain counts, and system summary.</p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}

      {/* Four system metric cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
        <Metric title="CPU" value={usage ? cpu.toFixed(1) + '%' : '–'}
          subtitle={usage ? `${cores} cores` : ''} color="indigo" ratio={cpu} />
        <Metric title="Memory" value={usage ? memory.toFixed(1) + '%' : '–'}
          subtitle={usage ? `${formatBytes(numberOrZero(usage?.memory?.used_kb) * 1024)} / ${formatBytes(numberOrZero(usage?.memory?.total_kb) * 1024)}` : ''}
          color="emerald" ratio={memory} />
        <Metric title="Disk" value={usage ? disk.toFixed(1) + '%' : '–'}
          subtitle={usage ? `${formatBytes(numberOrZero(usage?.disk?.used_byte))} / ${formatBytes(numberOrZero(usage?.disk?.total_byte))}` : ''}
          color="violet" ratio={disk} />
        <Metric title="Load (1 min)" value={usage ? oneMinuteLoad.toFixed(2) : '–'}
          subtitle={usage ? `5 min: ${numberOrZero(usage?.cpu?.load_5m).toFixed(2)} · 15 min: ${numberOrZero(usage?.cpu?.load_15m).toFixed(2)}` : ''}
          color="amber" ratio={Math.min(100, (oneMinuteLoad / cores) * 100)} />
      </div>

      {/* System summary and counters */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3 mb-5">
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-4">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">System</h3>
          <div className="space-y-1.5 text-sm">
            <Row label="Hostname" value={usage?.system?.hostname || '–'} />
            <Row label="Operating system" value={usage?.system?.os_name || '–'} />
            <Row label="Kernel" value={usage?.system?.kernel || '–'} />
            <Row label="Processor" value={usage?.system?.cpu_model ? `${usage.system.cpu_model} · ${cores} cores` : '–'} />
            <Row label="Swap" value={usage?.swap ? `${numberOrZero(usage.swap.percent).toFixed(1)}% · ${formatBytes(numberOrZero(usage.swap.used_kb) * 1024)} / ${formatBytes(numberOrZero(usage.swap.total_kb) * 1024)}` : '–'} />
            <Row label="Panel version" value={usage?.system?.panel_version || '–'} />
          </div>
        </div>
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-4">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Domains</h3>
          <div className="space-y-1.5 text-sm">
            <Row label="Total domains" value={counts ? String(counts.domains) : '–'} />
            <Row label="Active domains" value={
              <span className="text-emerald-700 dark:text-emerald-300 font-semibold">{counts ? counts.activeDomains : 0}</span>
            } />
            <Row label="Inactive domains" value={String(counts ? counts.domains - counts.activeDomains : 0)} />
          </div>
        </div>
      </div>

      <div className="text-xs text-slate-400 dark:text-slate-500 text-center mt-6">
        Visit the <a href="/monitoring" className="text-brand-600 dark:text-brand-400 hover:underline">Monitoring</a> page for more detailed monitoring.
      </div>
    </div>
  )
}

function Metric({ title, value, subtitle, color, ratio }: { title: string; value: string; subtitle: string; color: string; ratio: number }) {
  const colorMap: Record<string, string> = {
    indigo: 'bg-indigo-500', emerald: 'bg-emerald-500',
    violet: 'bg-violet-500', amber: 'bg-amber-500',
  }
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-4">
      <div className="text-xs text-slate-500 dark:text-slate-500 uppercase tracking-wider">{title}</div>
      <div className="text-2xl font-bold text-slate-900 dark:text-slate-100 mt-1">{value}</div>
      <div className="text-[11px] text-slate-500 dark:text-slate-500 mt-0.5 truncate">{subtitle}</div>
      <div className="mt-2 h-1.5 bg-slate-100 dark:bg-slate-700 rounded overflow-hidden">
        <div className={`h-full ${colorMap[color]} transition-all`} style={{ width: Math.min(100, Math.max(0, ratio)) + '%' }} />
      </div>
    </div>
  )
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 py-1 border-b border-slate-50 dark:border-slate-700/40 last:border-0">
      <span className="text-xs text-slate-500 dark:text-slate-500 shrink-0">{label}</span>
      <span className="text-xs font-mono text-slate-800 dark:text-slate-200 text-right truncate">{value}</span>
    </div>
  )
}
