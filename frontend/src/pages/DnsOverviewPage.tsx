// Server-wide DNS overview: record counts per domain and DNSSEC status, so
// "which domain is missing an MX record" is one screen instead of a walk.
import { Link } from 'react-router-dom'
import OverviewList, { type Column, type Badge } from '@/components/OverviewList'

type Row = {
  domain_id: number
  domain_name: string
  status: string
  record_count: number
  a_count: number
  mx_count: number
  txt_count: number
  disabled_count: number
  dnssec_active: boolean
}

const columns: Column<Row>[] = [
  {
    title: 'Domain',
    cell: (s) => (
      <Link to={`/subscriptions/${s.domain_id}/dns`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
        {s.domain_name}
      </Link>
    ),
  },
  { title: 'Records', cell: (s) => s.record_count },
  { title: 'A', cell: (s) => s.a_count },
  {
    title: 'MX',
    cell: (s) => (s.mx_count === 0
      ? <span className="px-2 py-0.5 rounded text-xs bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">0</span>
      : s.mx_count),
  },
  { title: 'TXT', cell: (s) => s.txt_count },
  {
    title: 'DNSSEC',
    cell: (s) => (s.dnssec_active
      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">On</span>
      : <span className="text-xs text-slate-400">Off</span>),
  },
]

export default function DnsOverviewPage() {
  return (
    <OverviewList<Row>
      title="DNS Management"
      icon="🌐"
      description="Record counts and DNSSEC status across every domain."
      endpoint="/overview/dns"
      columns={columns}
      searchField={(s) => s.domain_name}
      rowKey={(s) => s.domain_id}
      emptyMessage="No domains found."
      summary={(list): Badge[] => {
        const noMX = list.filter((s) => s.mx_count === 0).length
        const dnssec = list.filter((s) => s.dnssec_active).length
        return [
          { label: 'DNSSEC on', value: dnssec },
          ...(noMX > 0 ? [{ label: 'No MX', value: noMX, tone: 'warn' as const }] : []),
        ]
      }}
    />
  )
}
