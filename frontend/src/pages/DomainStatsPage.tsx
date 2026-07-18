import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type KeyValue = { name: string; count: number }
type Day = { date: string; request: number }
type Summary = {
  domain_name: string; has_log: boolean
  total_requests: number; total_bandwidth_mb: number; unique_ip: number; bot_ratio: number
  status_group: Record<string, number>
  top_paths: KeyValue[]; top_ip: KeyValue[]; agg_status: KeyValue[]; daily: Day[]; last_requests: string[]
}

export default function DomainStatsPage() {
  const { id } = useParams()
  const [summary, setSummary] = useState<Summary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  function load() {
    if (!id) return
    setLoading(true); setError(null)
    api.get<Summary>(`/domains/${id}/stats`)
      .then(response => setSummary(response.data))
      .catch(error => setError(apiError(error)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [id])

  if (loading) return <div className="px-6 py-5 text-slate-400">Loading…</div>
  if (!summary) return <div className="px-6 py-5"><div className="text-sm text-red-600">{error || 'Not found'}</div></div>

  const maxDailyRequests = Math.max(1, ...summary.daily.map(day => day.request))
  const statusBar: Record<string, string> = { '2xx': 'bg-emerald-500', '3xx': 'bg-sky-500', '4xx': 'bg-amber-500', '5xx': 'bg-rose-500' }

  return (
    <div className="px-6 py-5">
      <div className="max-w-5xl mx-auto">
        <Breadcrumb items={[
          { label: 'Home', href: '/' },
          { label: 'Domains', href: '/domains' },
          { label: summary.domain_name, href: `/subscriptions/${id}` },
          { label: 'Statistics' },
        ]} />

        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Traffic Statistics</h1>
            <p className="text-sm text-slate-500 dark:text-slate-400 mt-1"><span className="font-mono">{summary.domain_name}</span>, nginx access log analysis.</p>
          </div>
          <button onClick={load} className="text-sm px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800">↻ Refresh</button>
        </div>

        {!summary.has_log || summary.total_requests === 0 ? (
          <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-10 text-center text-sm text-slate-400">
            No access log data yet. It will appear here once the site starts receiving traffic.
          </div>
        ) : (
          <>
            {/* KPI cards */}
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-4">
              <KPI label="Total Requests" value={summary.total_requests.toLocaleString('en-US')} color="indigo" />
              <KPI label="Bandwidth Usage" value={`${summary.total_bandwidth_mb.toFixed(1)} MB`} color="sky" />
              <KPI label="Unique IPs" value={summary.unique_ip.toLocaleString('en-US')} color="emerald" />
              <KPI label="Bot Ratio" value={`${summary.bot_ratio}%`} color={summary.bot_ratio >= 50 ? 'rose' : 'violet'} />
            </div>

            {/* Status distribution */}
            <Card title="HTTP Status Distribution">
              <div className="space-y-2">
                {(['2xx', '3xx', '4xx', '5xx'] as const).map(group => {
                  const count = summary.status_group[group] || 0
                  const ratio = summary.total_requests ? Math.round(count / summary.total_requests * 100) : 0
                  return (
                    <div key={group} className="flex items-center gap-3">
                      <span className="w-10 text-xs font-mono text-slate-500">{group}</span>
                      <div className="flex-1 h-3 rounded-full bg-slate-100 dark:bg-slate-700/50 overflow-hidden">
                        <div className={`h-full rounded-full ${statusBar[group]}`} style={{ width: Math.max(ratio, count > 0 ? 2 : 0) + '%' }} />
                      </div>
                      <span className="w-24 text-right text-xs font-mono text-slate-600 dark:text-slate-300">{count.toLocaleString('en-US')} <span className="text-slate-400">{ratio}%</span></span>
                    </div>
                  )
                })}
              </div>
            </Card>

            {/* Daily requests over seven days */}
            {summary.daily.length > 0 && (
              <Card title="Daily Requests (last 7 days)">
                <div className="flex items-end gap-2 h-32">
                  {summary.daily.map(day => (
                    <div key={day.date} className="flex-1 flex flex-col items-center gap-1">
                      <div className="w-full flex items-end justify-center" style={{ height: '100px' }}>
                        <div className="w-full max-w-[36px] rounded-t bg-gradient-to-t from-brand-600 to-brand-400" style={{ height: Math.max(4, day.request / maxDailyRequests * 100) + '%' }} title={`${day.request} requests`} />
                      </div>
                      <span className="text-[10px] text-slate-400 font-mono">{day.date.split('/')[0]}</span>
                      <span className="text-[10px] text-slate-600 dark:text-slate-300 font-mono">{day.request}</span>
                    </div>
                  ))}
                </div>
              </Card>
            )}

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <Card title="Most Requested Paths">
                <Table rows={summary.top_paths} unit="requests" mono />
              </Card>
              <Card title="Most Active IPs">
                <Table rows={summary.top_ip} unit="requests" mono />
              </Card>
            </div>

            <Card title="Latest Requests">
              <div className="font-mono text-xs space-y-1 max-h-64 overflow-y-auto">
                {summary.last_requests.map((request, index) => {
                  const code = request.slice(0, 3)
                  const color = code[0] === '5' ? 'text-rose-500' : code[0] === '4' ? 'text-amber-500' : code[0] === '3' ? 'text-sky-500' : 'text-emerald-500'
                  return <div key={index} className="truncate"><span className={color}>{code}</span><span className="text-slate-500 dark:text-slate-400">{request.slice(3)}</span></div>
                })}
              </div>
            </Card>
          </>
        )}

        <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to subscription</Link></div>
      </div>
    </div>
  )
}

function KPI({ label, value, color }: { label: string; value: string; color: string }) {
  const map: Record<string, string> = { indigo: 'text-indigo-500', sky: 'text-sky-500', emerald: 'text-emerald-500', violet: 'text-violet-500', rose: 'text-rose-500' }
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-4 shadow-sm">
      <div className="text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-400">{label}</div>
      <div className={`text-2xl font-bold font-mono mt-1 ${map[color] || 'text-slate-700'}`}>{value}</div>
    </div>
  )
}
function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
      <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{title}</h3>
      {children}
    </div>
  )
}
function Table({ rows, unit, mono }: { rows: KeyValue[]; unit: string; mono?: boolean }) {
  const max = Math.max(1, ...rows.map(row => row.count))
  if (!rows.length) return <div className="text-sm text-slate-400 py-3">No data.</div>
  return (
    <div className="space-y-1.5">
      {rows.map((row, index) => (
        <div key={index} className="relative flex items-center justify-between text-xs py-1 px-2 rounded overflow-hidden">
          <div className="absolute inset-0 bg-brand-500/10 dark:bg-brand-500/15" style={{ width: (row.count / max * 100) + '%' }} />
          <span className={`relative truncate ${mono ? 'font-mono' : ''} text-slate-700 dark:text-slate-200`} title={row.name}>{row.name}</span>
          <span className="relative shrink-0 ml-2 font-mono text-slate-500 dark:text-slate-400">{row.count.toLocaleString('en-US')} {unit}</span>
        </div>
      ))}
    </div>
  )
}
