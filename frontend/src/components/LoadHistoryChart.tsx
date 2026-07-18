import { useEffect, useState } from 'react'
import { api } from '@/lib/api'

type Point = { ts: string; load_1m: number; load_5m: number; load_15m: number; memory: number }
type Response = { hour: number; cores: number; points: Point[] }

const INTERVALS = [
  { label: '1h', hours: 1 },
  { label: '6h', hours: 6 },
  { label: '24h', hours: 24 },
  { label: '7d', hours: 168 },
]

// SVG coordinate plane.
const W = 1000, H = 260, ML = 44, MR = 14, MT = 14, MB = 28

export default function LoadHistoryChart() {
  const [hours, setHours] = useState(24)
  const [d, setD] = useState<Response | null>(null)
  const [forbidden, setForbidden] = useState(false)

  useEffect(() => {
    let on = true
    async function tick() {
      try {
        const r = await api.get<Response>(`/system/load-history?hour=${hours}`)
        if (on) { setD(r.data); setForbidden(false) }
      } catch (e: any) {
        if (on && e?.response?.status === 403) setForbidden(true)
      }
    }
    tick()
    const t = setInterval(tick, 30000)
    return () => { on = false; clearInterval(t) }
  }, [hours])

  if (forbidden) return null // Administrators only.

  const pts = d?.points || []
  const coreCount = d?.cores || 0
  const latest = pts[pts.length - 1]

  // Y scale: cover the highest load or core count with 15% headroom.
  const maxVal = Math.max(0.5, coreCount, ...pts.flatMap(p => [p.load_1m, p.load_5m, p.load_15m]))
  const yMax = Math.ceil(maxVal * 1.15 * 10) / 10
  const cw = W - ML - MR, ch = H - MT - MB
  const xAt = (i: number) => ML + (pts.length <= 1 ? 0 : (i / (pts.length - 1)) * cw)
  const yAt = (v: number) => MT + (1 - Math.min(v, yMax) / yMax) * ch
  const line = (key: keyof Point) => pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${xAt(i).toFixed(1)},${yAt(p[key] as number).toFixed(1)}`).join(' ')
  const area1 = pts.length
    ? `M${xAt(0).toFixed(1)},${(H - MB).toFixed(1)} ` +
      pts.map((p, i) => `L${xAt(i).toFixed(1)},${yAt(p.load_1m).toFixed(1)}`).join(' ') +
      ` L${xAt(pts.length - 1).toFixed(1)},${(H - MB).toFixed(1)} Z`
    : ''

  // Axis labels.
  const yTicks = [0, yMax / 2, yMax]
  const xLabel = (ts: string) => hours > 24 ? ts.slice(5, 10).replace('-', '/') : ts.slice(11, 16)
  const xTickIdx = pts.length > 1
    ? [0, Math.floor(pts.length / 3), Math.floor((2 * pts.length) / 3), pts.length - 1]
    : []

  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3 mb-3">
        <div>
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">System Load History</h3>
          <p className="text-[11px] text-slate-400 dark:text-slate-500">
            Load average (1 / 5 / 15min){coreCount ? ` · ${coreCount} cores` : ''}
          </p>
        </div>
        <div className="flex items-center gap-1 bg-slate-100 dark:bg-slate-900/50 rounded-lg p-0.5">
          {INTERVALS.map(a => (
            <button key={a.hours} onClick={() => setHours(a.hours)}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${hours === a.hours
                ? 'bg-white dark:bg-slate-700 text-brand-700 dark:text-brand-300 shadow-sm'
                : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'}`}>
              {a.label}
            </button>
          ))}
        </div>
      </div>

      {/* Current values */}
      {latest && (
        <div className="flex flex-wrap gap-4 mb-3 text-xs">
          <Value color="#6366f1" label="1min" v={latest.load_1m} coreCount={coreCount} />
          <Value color="#f59e0b" label="5min" v={latest.load_5m} coreCount={coreCount} />
          <Value color="#94a3b8" label="15min" v={latest.load_15m} coreCount={coreCount} />
        </div>
      )}

      {pts.length === 0 ? (
        <div className="h-[220px] flex flex-col items-center justify-center text-center text-sm text-slate-400 dark:text-slate-500">
          <div className="text-2xl mb-2">📈</div>
          No data has been collected yet. Samples are recorded every minute, and the chart will populate shortly.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <svg viewBox={`0 0 ${W} ${H}`} className="w-full" style={{ minWidth: 320 }} preserveAspectRatio="none">
            <defs>
              <linearGradient id="load1grad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#6366f1" stopOpacity="0.28" />
                <stop offset="100%" stopColor="#6366f1" stopOpacity="0.02" />
              </linearGradient>
            </defs>
            {/* Horizontal grid and Y-axis labels */}
            {yTicks.map((v, i) => (
              <g key={i}>
                <line x1={ML} y1={yAt(v)} x2={W - MR} y2={yAt(v)} className="stroke-slate-100 dark:stroke-slate-700/60" strokeWidth="1" />
                <text x={ML - 6} y={yAt(v) + 3} textAnchor="end" className="fill-slate-400" fontSize="11">{v.toFixed(1)}</text>
              </g>
            ))}
            {/* Core-count reference line, representing 100% capacity */}
            {coreCount > 0 && coreCount <= yMax && (
              <g>
                <line x1={ML} y1={yAt(coreCount)} x2={W - MR} y2={yAt(coreCount)} stroke="#ef4444" strokeWidth="1" strokeDasharray="4 4" opacity="0.6" />
                <text x={W - MR} y={yAt(coreCount) - 4} textAnchor="end" fill="#ef4444" fontSize="10" opacity="0.8">{coreCount} cores</text>
              </g>
            )}
            {/* X-axis labels */}
            {xTickIdx.map((idx, i) => (
              <text key={i} x={xAt(idx)} y={H - 8} textAnchor={i === 0 ? 'start' : i === xTickIdx.length - 1 ? 'end' : 'middle'}
                className="fill-slate-400" fontSize="11">{xLabel(pts[idx].ts)}</text>
            ))}
            {/* Data series */}
            <path d={area1} fill="url(#load1grad)" />
            <path d={line('load_15m')} fill="none" stroke="#94a3b8" strokeWidth="1.4" strokeLinejoin="round" opacity="0.85" />
            <path d={line('load_5m')} fill="none" stroke="#f59e0b" strokeWidth="1.6" strokeLinejoin="round" opacity="0.9" />
            <path d={line('load_1m')} fill="none" stroke="#6366f1" strokeWidth="2" strokeLinejoin="round" />
          </svg>
        </div>
      )}
    </div>
  )
}

function Value({ color, label, v, coreCount }: { color: string; label: string; v: number; coreCount: number }) {
  const overCapacity = coreCount > 0 && v > coreCount
  return (
    <div className="flex items-center gap-1.5">
      <span className="w-2.5 h-2.5 rounded-sm" style={{ background: color }} />
      <span className="text-slate-500 dark:text-slate-400">{label}</span>
      <span className={`font-mono font-semibold ${overCapacity ? 'text-red-600 dark:text-red-400' : 'text-slate-800 dark:text-slate-100'}`}>
        {v.toFixed(2)}
      </span>
    </div>
  )
}
