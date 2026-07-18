import { useEffect, useState } from 'react'
import { api } from '@/lib/api'

type Usage = {
  cpu: { percent: number; cores: number; load_1m: number; load_5m: number; load_15m: number }
  memory: { total_kb: number; used_kb: number; free_kb: number; percent: number }
  disk: { total_byte: number; used_byte: number; free_byte: number; percent: number; mount: string }
  uptime_sec: number
}

type Health = { status: string; version: string; time: string }

export default function ResourceCard() {
  const [u, setU] = useState<Usage | null>(null)
  const [s, setS] = useState<Health | null>(null)

  useEffect(() => {
    let on = true
    async function tick() {
      try {
        const r = await api.get<Usage>('/system/usage')
        if (on) setU(r.data)
      } catch { /* Ignore transient polling errors. */ }
      try {
        const r = await fetch('/healthz', { cache: 'no-store' })
        if (r.ok && on) setS(await r.json())
      } catch { /* Ignore transient polling errors. */ }
    }
    tick()
    const t = setInterval(tick, 4000)
    return () => { on = false; clearInterval(t) }
  }, [])

  return (
    <div className="space-y-4">
      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-4 flex items-center justify-between">
          Resource Usage
          {u && <span className="text-[10px] font-normal text-slate-400 dark:text-slate-500 uppercase tracking-wider">live</span>}
        </h3>

        {!u ? (
          <div className="text-sm text-slate-400 dark:text-slate-500 py-6 text-center">Loading…</div>
        ) : (
          <>
            <Bar
              label="CPU"
              percent={u.cpu.percent}
              alt={`${u.cpu.cores} core(s) · load ${u.cpu.load_1m.toFixed(2)} / ${u.cpu.load_5m.toFixed(2)} / ${u.cpu.load_15m.toFixed(2)}`}
              color="brand"
            />
            <Bar
              label="Memory"
              percent={u.memory.percent}
              alt={`${(u.memory.used_kb / 1024).toFixed(0)} MB / ${(u.memory.total_kb / 1024).toFixed(0)} MB`}
              color="emerald"
            />
            <Bar
              label="Disk"
              percent={u.disk.percent}
              alt={`${(u.disk.used_byte / 1e9).toFixed(1)} GB / ${(u.disk.total_byte / 1e9).toFixed(1)} GB`}
              color="violet"
            />
            <div className="mt-4 pt-3 border-t border-slate-100 dark:border-slate-800 text-xs text-slate-500 dark:text-slate-500 flex justify-between">
              <span>Uptime</span>
              <span className="font-mono text-slate-700 dark:text-slate-300">{formatUptime(u.uptime_sec)}</span>
            </div>
          </>
        )}
      </div>

      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">System Status</h3>
        {!s ? (
          <div className="text-sm text-slate-400 dark:text-slate-500">Waiting…</div>
        ) : (
          <div className="space-y-2 text-sm">
            <Row
              label="Backend"
              value={s.status === 'up' ? 'Running' : s.status}
              ok={s.status === 'up'}
            />
            <Row label="Version" value={s.version} ok />
            <Row label="Time" value={new Date(s.time).toLocaleTimeString('en-US')} ok />
          </div>
        )}
      </div>
    </div>
  )
}

function Bar({ label, percent, alt, color }: { label: string; percent: number; alt: string; color: string }) {
  const bg: Record<string, string> = {
    brand:   'bg-brand-500',
    emerald: 'bg-emerald-500',
    violet:  'bg-violet-500',
  }
  const barColor = percent >= 85 ? 'bg-red-500' : percent >= 70 ? 'bg-amber-500' : bg[color]
  return (
    <div className="mb-3 last:mb-0">
      <div className="flex justify-between text-sm">
        <span className="text-slate-700 dark:text-slate-300 font-medium">{label}</span>
        <span className="font-mono text-slate-900 dark:text-slate-100">%{percent.toFixed(1)}</span>
      </div>
      <div className="h-2 bg-slate-100 dark:bg-slate-800 rounded-full overflow-hidden my-1">
        <div className={`h-full transition-all duration-500 ${barColor}`} style={{ width: `${Math.min(100, Math.max(2, percent))}%` }}></div>
      </div>
      <div className="text-[11px] text-slate-500 dark:text-slate-500 font-mono">{alt}</div>
    </div>
  )
}

function Row({ label, value, ok }: { label: string; value: string; ok: boolean }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="text-slate-600 dark:text-slate-400 dark:text-slate-500">{label}</span>
      <span className={`font-medium flex items-center gap-1.5 ${ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'}`}>
        <span className={`w-1.5 h-1.5 rounded-full ${ok ? 'bg-emerald-500' : 'bg-red-500'}`}></span>
        {value}
      </span>
    </div>
  )
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}min`
  return `${minutes}min`
}