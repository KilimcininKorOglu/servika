// Server-wide certificate overview. Its real purpose is not missing one that
// is about to expire: the list arrives sorted by nearest expiry date.
import { Link } from 'react-router-dom'
import OverviewList, { type Column, type Badge } from '@/components/OverviewList'

type Row = {
  domain_id: number
  domain_name: string
  status: string
  ssl_enabled: boolean
  ssl_expiry: string
  remaining_days: number | null
}

// Let's Encrypt issues 90-day certificates and renews 30 days out; under 14
// days means "renewal did not run", hence the separate threshold.
function RemainingBadge({ days }: { days: number | null }) {
  if (days === null) return <span className="text-slate-400">—</span>
  const danger = 'px-2 py-0.5 rounded text-xs font-medium bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
  const warn = 'px-2 py-0.5 rounded text-xs font-medium bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
  if (days < 0) return <span className={danger}>expired {Math.abs(days)}d ago</span>
  if (days <= 14) return <span className={danger}>{days} days</span>
  if (days <= 30) return <span className={warn}>{days} days</span>
  return <span className="text-slate-600 dark:text-slate-400">{days} days</span>
}

const columns: Column<Row>[] = [
  {
    title: 'Domain',
    cell: (s) => (
      <Link to={`/subscriptions/${s.domain_id}/ssl`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
        {s.domain_name}
      </Link>
    ),
  },
  {
    title: 'SSL',
    cell: (s) => (s.ssl_enabled
      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">Enabled</span>
      : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">None</span>),
  },
  { title: 'Expiry', cell: (s) => (s.ssl_expiry || <span className="text-slate-400">—</span>) },
  { title: 'Remaining', cell: (s) => <RemainingBadge days={s.remaining_days} /> },
  {
    title: 'Status',
    cell: (s) => (s.status === 'active'
      ? <span className="text-xs text-slate-500">Active</span>
      : <span className="text-xs text-amber-600">Passive</span>),
  },
]

export default function SslOverviewPage() {
  return (
    <OverviewList<Row>
      title="SSL Certificates"
      icon="🔒"
      description="Server-wide certificate status, sorted by nearest expiry."
      endpoint="/overview/ssl"
      columns={columns}
      searchField={(s) => s.domain_name}
      rowKey={(s) => s.domain_id}
      emptyMessage="No domains found."
      summary={(list): Badge[] => {
        const expiring = list.filter((s) => s.remaining_days !== null && s.remaining_days <= 14).length
        const active = list.filter((s) => s.ssl_enabled).length
        return [
          { label: 'With SSL', value: active },
          ...(expiring > 0 ? [{ label: 'Expiring ≤14d', value: expiring, tone: 'danger' as const }] : []),
        ]
      }}
    />
  )
}
