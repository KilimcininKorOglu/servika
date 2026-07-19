import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '@/lib/api'
import { useAuth } from '@/store/auth'
import LoadHistoryChart from '@/components/LoadHistoryChart'

type SystemInfo = {
  hostname: string; ip: string; os_name: string; kernel: string
  arch: string; cpu_model: string; cpu_cores: number; panel_version: string
}
type CPU = { percent: number; cores: number; load_1m: number; load_5m: number; load_15m: number }
type Memory = { total_kb: number; used_kb: number; free_kb: number; percent: number }
type Swap = { total_kb: number; used_kb: number; percent: number }
type Disk = { total_byte: number; used_byte: number; free_byte: number; percent: number; mount: string; fs?: string }
type Network = { interface: string; rx_bytes_sec: number; tx_bytes_sec: number; rx_total_byte: number; tx_total_byte: number }
type Service = { name: string; label: string; enabled: boolean }
type SystemUsage = {
  system: SystemInfo; cpu: CPU; memory: Memory; swap: Swap
  disk: Disk; disks: Disk[]; network: Network; services: Service[]; uptime_sec: number
  quota_reboot_required?: boolean
}
type Domain = { id: number; domain_name: string; ssl: boolean; status: string }

export default function HomePage() {
  const username = useAuth((s) => s.username)
  const [s, setS] = useState<SystemUsage | null>(null)
  const [domains, setDomains] = useState<Domain[]>([])

  useEffect(() => {
    const fetchUsage = () => api.get<SystemUsage>('/system/usage').then((r) => setS(r.data)).catch(() => {})
    fetchUsage()
    api.get<Domain[]>('/domains').then((r) => setDomains(r.data)).catch(() => {})
    const id = setInterval(fetchUsage, 5000)
    return () => clearInterval(id)
  }, [])

  const enabled = domains.filter((d) => d.status === 'active').length
  const sslEnabled = domains.filter((d) => d.ssl).length
  const diskList = s ? (s.disks?.length ? s.disks : [s.disk]) : []
  const primaryDisk = s ? (diskList[0] || s.disk) : null

  return (
    <div className="px-6 py-5 max-w-[1400px] mx-auto">
      <div className="flex items-center justify-between mb-4">
        <div>
          <h1 className="text-lg font-semibold text-slate-900 dark:text-slate-100 leading-tight">System Dashboard</h1>
          <p className="text-xs text-slate-500 dark:text-slate-400">
            Welcome, <span className="text-slate-700 dark:text-slate-300 font-medium">{username?.full_name || username?.name}</span>
          </p>
        </div>
        <div className="flex items-center gap-1.5 text-[11px] text-slate-400">
          <span className={`w-1.5 h-1.5 rounded-full ${s ? 'bg-emerald-500 animate-pulse' : 'bg-slate-300'}`} />
          Live
        </div>
      </div>

      {/* Disk quota inactive — single reboot required (only when flag is true) */}
      {s?.quota_reboot_required && (
        <div className="mb-3 flex items-start gap-3 rounded-2xl border border-amber-300 dark:border-amber-800/60 bg-amber-50 dark:bg-amber-900/15 px-4 py-3">
          <svg className="w-5 h-5 shrink-0 text-amber-500 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m0 3.75h.008M10.363 3.591 2.257 17.657a1.5 1.5 0 0 0 1.302 2.25h16.882a1.5 1.5 0 0 0 1.302-2.25L13.638 3.591a1.5 1.5 0 0 0-2.598 0Z" />
          </svg>
          <div className="min-w-0">
            <div className="text-sm font-semibold text-amber-800 dark:text-amber-200">Disk quota inactive</div>
            <div className="text-xs text-amber-700 dark:text-amber-300 mt-0.5">
              Disk quota is inactive — a single server reboot is required to enable it.
            </div>
          </div>
        </div>
      )}

      {/* Compact KPI ring gauges */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-3">
        <KpiRing label="CPU" percent={s?.cpu.percent ?? 0} alt={s ? `${s.cpu.cores} cores` : '…'} color="brand" ready={!!s} />
        <KpiRing label="Memory" percent={s?.memory.percent ?? 0} alt={s ? `${fmtGB(s.memory.used_kb)} / ${fmtGB(s.memory.total_kb)}` : '…'} color="emerald" ready={!!s} />
        <KpiRing label="Disk" percent={primaryDisk?.percent ?? 0} alt={primaryDisk ? `${fmtByteGB(primaryDisk.used_byte)} / ${fmtByteGB(primaryDisk.total_byte)}` : '…'} color="violet" ready={!!s} />
        <LoadCard cpu={s?.cpu} />
      </div>

      {/* System information strip with inline chips */}
      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl px-4 py-2.5 mb-3">
        <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5">
          <Info label="Server" val={s?.system.hostname} mono />
          <Info label="IP" val={s?.system.ip} mono />
          <Info label="OS" val={s?.system.os_name} />
          <Info label="Kernel" val={s?.system.kernel} mono short />
          <Info label="Uptime" val={s ? formatUptime(s.uptime_sec) : undefined} />
          {s && s.swap.total_kb > 0 && <Info label="Swap" val={`%${s.swap.percent.toFixed(0)}`} />}
          <Info label="Version" val={s?.system.panel_version} mono />
        </div>
      </div>

      {/* Load history chart */}
      <div className="mb-3">
        <LoadHistoryChart />
      </div>

      {/* Three-column grid: disk, network, and domains */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 mb-3">
        <Card title="Disk Usage">
          {!s ? <Loading /> : (
            <div className="space-y-2.5">
              {diskList.map((d, i) => <DiskRow key={i} d={d} />)}
            </div>
          )}
        </Card>

        <Card title="Network Traffic">
          {!s ? <Loading /> : !s.network.interface ? (
            <div className="text-xs text-slate-400 dark:text-slate-500 py-2 italic">No interface found</div>
          ) : (
            <>
              <div className="text-[11px] text-slate-400 dark:text-slate-500 font-mono mb-2">{s.network.interface}</div>
              <div className="grid grid-cols-2 gap-2">
                <div className="rounded-lg bg-emerald-50 dark:bg-emerald-900/15 border border-emerald-100 dark:border-emerald-800/40 p-2.5">
                  <div className="text-[10px] uppercase tracking-wide text-emerald-600 dark:text-emerald-400 font-semibold">↓ Download</div>
                  <div className="text-base font-bold font-mono text-emerald-700 dark:text-emerald-300 mt-0.5">{fmtRate(s.network.rx_bytes_sec)}</div>
                  <div className="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5">Σ {fmtByteGB(s.network.rx_total_byte)}</div>
                </div>
                <div className="rounded-lg bg-sky-50 dark:bg-sky-900/15 border border-sky-100 dark:border-sky-800/40 p-2.5">
                  <div className="text-[10px] uppercase tracking-wide text-sky-600 dark:text-sky-400 font-semibold">↑ Upload</div>
                  <div className="text-base font-bold font-mono text-sky-700 dark:text-sky-300 mt-0.5">{fmtRate(s.network.tx_bytes_sec)}</div>
                  <div className="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5">Σ {fmtByteGB(s.network.tx_total_byte)}</div>
                </div>
              </div>
            </>
          )}
        </Card>

        <Card title="Domains">
          <div className="grid grid-cols-3 gap-2 mb-3">
            <MiniStat value={domains.length} label="Total" color="slate" />
            <MiniStat value={enabled} label="Active" color="emerald" />
            <MiniStat value={sslEnabled} label="SSL" color="violet" />
          </div>
          <Link to="/domains" className="block text-xs text-brand-600 dark:text-brand-400 hover:underline font-medium">
            Manage all domains →
          </Link>
          <div className="mt-3 pt-3 border-t border-slate-100 dark:border-slate-700/60 grid grid-cols-2 gap-2">
            <QuickLink to="/firewall" label="Firewall" />
            <QuickLink to="/monitoring" label="Monitoring" />
            <QuickLink to="/tools-settings" label="Settings" />
          </div>
        </Card>
      </div>

      {/* Services as compact chips */}
      <Card title="Services" className="mb-3">
        {!s ? <Loading /> : (
          <div className="flex flex-wrap gap-1.5">
            {s.services.map((sv) => (
              <span key={sv.name} title={sv.name}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs border ${
                  sv.enabled
                    ? 'border-emerald-200 dark:border-emerald-800/60 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300'
                    : 'border-red-200 dark:border-red-800/60 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300'
                }`}>
                <span className={`w-1.5 h-1.5 rounded-full ${sv.enabled ? 'bg-emerald-500' : 'bg-red-500'}`} />
                {sv.label}
              </span>
            ))}
          </div>
        )}
      </Card>

      {/* Recent domains in a compact table */}
      <Card title="Recent Domains">
        {domains.length === 0 ? (
          <div className="py-5 text-center text-xs text-slate-400">No domains yet</div>
        ) : (
          <div className="overflow-x-auto -mx-1">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-[11px] uppercase tracking-wide text-slate-400 border-b border-slate-100 dark:border-slate-700/60">
                  <th className="text-left font-medium py-1.5 px-1">Domain</th>
                  <th className="text-left font-medium py-1.5 px-1">Status</th>
                  <th className="text-left font-medium py-1.5 px-1">SSL</th>
                  <th className="text-right font-medium py-1.5 px-1"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-50 dark:divide-slate-800">
                {domains.slice(0, 6).map((d) => (
                  <tr key={d.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                    <td className="py-2 px-1">
                      <Link to={`/subscriptions/${d.id}`} className="text-brand-600 dark:text-brand-400 hover:underline font-medium">{d.domain_name}</Link>
                    </td>
                    <td className="px-1">
                      <span className={`text-[10px] px-1.5 py-0.5 rounded uppercase font-semibold tracking-wide ${
                        d.status === 'active' ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-400'
                      }`}>{d.status}</span>
                    </td>
                    <td className="px-1 text-xs">
                      {d.ssl ? <span className="text-emerald-600 dark:text-emerald-400">● Protected</span> : <span className="text-amber-600 dark:text-amber-400">○ None</span>}
                    </td>
                    <td className="px-1 text-right">
                      <Link to={`/subscriptions/${d.id}`} className="text-xs text-slate-400 hover:text-brand-600 dark:hover:text-brand-400">Dashboard →</Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  )
}

/* ---------- Components ---------- */

const COLORS: Record<string, string> = { brand: '#f97316', emerald: '#10b981', violet: '#8b5cf6', sky: '#0ea5e9' }
function thresholdColor(value: number, baseColor: string) {
  if (value >= 85) return '#ef4444'
  if (value >= 70) return '#f59e0b'
  return COLORS[baseColor] || '#64748b'
}

function KpiRing({ label, percent, alt, color, ready }: { label: string; percent: number; alt: string; color: string; ready: boolean }) {
  const r = 26, c = 2 * Math.PI * r
  const val = Math.min(100, Math.max(0, percent))
  const off = c * (1 - val / 100)
  const stroke = thresholdColor(percent, color)
  return (
    <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-3.5 flex items-center gap-3">
      <div className="relative shrink-0 w-16 h-16">
        <svg viewBox="0 0 64 64" className="w-16 h-16 -rotate-90">
          <circle cx="32" cy="32" r={r} fill="none" className="stroke-slate-100 dark:stroke-slate-700" strokeWidth="5" />
          <circle cx="32" cy="32" r={r} fill="none" stroke={stroke} strokeWidth="5" strokeLinecap="round"
            strokeDasharray={c} strokeDashoffset={ready ? off : c} className="transition-all duration-700" />
        </svg>
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="text-sm font-bold text-slate-800 dark:text-slate-100">{ready ? `%${percent.toFixed(0)}` : '—'}</span>
        </div>
      </div>
      <div className="min-w-0">
        <div className="text-[11px] uppercase tracking-wide text-slate-400 dark:text-slate-500 font-semibold">{label}</div>
        <div className="text-xs text-slate-500 dark:text-slate-400 truncate mt-0.5" title={alt}>{alt}</div>
      </div>
    </div>
  )
}

function LoadCard({ cpu }: { cpu?: CPU }) {
  const cores = cpu?.cores || 1
  const color = (value: number) => value >= cores ? 'text-red-500' : value >= cores * 0.7 ? 'text-amber-500' : 'text-slate-800 dark:text-slate-100'
  return (
    <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-3.5">
      <div className="text-[11px] uppercase tracking-wide text-slate-400 dark:text-slate-500 font-semibold mb-1.5">Load Average</div>
      {!cpu ? (
        <div className="text-slate-300 dark:text-slate-600 text-xl font-mono">—</div>
      ) : (
        <>
          <div className="flex items-baseline gap-2">
            <span className={`text-2xl font-bold font-mono ${color(cpu.load_1m)}`}>{cpu.load_1m.toFixed(2)}</span>
            <span className="text-[11px] text-slate-400">1 min</span>
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-400 font-mono mt-1">
            {cpu.load_5m.toFixed(2)} · {cpu.load_15m.toFixed(2)} <span className="text-slate-400">/ {cores} cores</span>
          </div>
        </>
      )}
    </div>
  )
}

function Info({ label, val, mono, short }: { label: string; val?: string; mono?: boolean; short?: boolean }) {
  return (
    <div className="flex items-baseline gap-1.5 min-w-0">
      <span className="text-[10px] uppercase tracking-wide text-slate-400 dark:text-slate-500 font-semibold shrink-0">{label}</span>
      <span className={`text-xs text-slate-700 dark:text-slate-200 truncate ${mono ? 'font-mono' : 'font-medium'} ${short ? 'max-w-[160px]' : 'max-w-[220px]'}`} title={val || ''}>
        {val || '—'}
      </span>
    </div>
  )
}

function Card({ title, children, className }: { title: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={`bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 ${className || ''}`}>
      <h3 className="text-[11px] uppercase tracking-wide font-semibold text-slate-400 dark:text-slate-500 mb-3">{title}</h3>
      {children}
    </div>
  )
}

function MiniStat({ value, label, color }: { value: number; label: string; color: string }) {
  const r: Record<string, string> = { slate: 'text-slate-700 dark:text-slate-200', emerald: 'text-emerald-600 dark:text-emerald-400', violet: 'text-violet-600 dark:text-violet-400' }
  return (
    <div className="text-center py-2 rounded-lg bg-slate-50 dark:bg-slate-900/50">
      <div className={`text-xl font-bold ${r[color]}`}>{value}</div>
      <div className="text-[10px] uppercase tracking-wide text-slate-400 mt-0.5">{label}</div>
    </div>
  )
}

function QuickLink({ to, label }: { to: string; label: string }) {
  return (
    <Link to={to} className="px-2.5 py-1.5 text-xs text-center rounded-md bg-slate-50 dark:bg-slate-900/50 text-slate-600 dark:text-slate-300 hover:bg-brand-50 dark:hover:bg-brand-900/20 hover:text-brand-700 dark:hover:text-brand-300 transition font-medium">
      {label}
    </Link>
  )
}

function DiskRow({ d }: { d: Disk }) {
  const barColor = d.percent >= 85 ? 'bg-red-500' : d.percent >= 70 ? 'bg-amber-500' : 'bg-sky-500'
  return (
    <div>
      <div className="flex justify-between items-baseline text-xs mb-1">
        <span className="font-mono font-medium text-slate-700 dark:text-slate-200 truncate">{d.mount}</span>
        <span className="text-slate-400 shrink-0 ml-2">
          {fmtByteGB(d.used_byte)} / {fmtByteGB(d.total_byte)}
          <span className="ml-2 font-mono font-semibold text-slate-600 dark:text-slate-300">%{d.percent.toFixed(1)}</span>
        </span>
      </div>
      <div className="h-1.5 bg-slate-100 dark:bg-slate-700/60 rounded-full overflow-hidden">
        <div className={`h-full transition-all duration-700 ${barColor}`} style={{ width: `${Math.min(100, Math.max(1, d.percent))}%` }} />
      </div>
    </div>
  )
}

function Loading() { return <div className="py-4 text-center text-xs text-slate-400">Loading…</div> }

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400), hours = Math.floor((seconds % 86400) / 3600), minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  return `${minutes}m`
}
function fmtGB(kb: number): string {
  const mb = kb / 1024
  return mb < 1024 ? `${mb.toFixed(0)} MB` : `${(mb / 1024).toFixed(1)} GB`
}
function fmtByteGB(b: number): string {
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(0)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(0)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(1)} GB`
}
function fmtRate(bps: number): string {
  if (bps < 1024) return `${bps.toFixed(0)} B/s`
  if (bps < 1024 * 1024) return `${(bps / 1024).toFixed(0)} KB/s`
  return `${(bps / 1024 / 1024).toFixed(1)} MB/s`
}
