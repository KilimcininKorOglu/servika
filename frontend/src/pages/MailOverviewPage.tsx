// Server-wide mail overview: mailbox and alias counts per domain, so total
// mail footprint is visible without opening each domain.
import { Link } from 'react-router-dom'
import OverviewList, { type Column, type Badge } from '@/components/OverviewList'

type Row = {
  domain_id: number
  domain_name: string
  mail_enabled: boolean
  mail_status: string
  mailbox_count: number
  alias_count: number
  suspended_mailbox_count: number
}

const columns: Column<Row>[] = [
  {
    title: 'Domain',
    cell: (s) => (
      <Link to={`/subscriptions/${s.domain_id}/mail`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
        {s.domain_name}
      </Link>
    ),
  },
  {
    title: 'Mail',
    cell: (s) => (s.mail_enabled
      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{s.mail_status}</span>
      : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">None</span>),
  },
  { title: 'Mailboxes', cell: (s) => s.mailbox_count },
  { title: 'Aliases', cell: (s) => s.alias_count },
  {
    title: 'Suspended',
    cell: (s) => (s.suspended_mailbox_count > 0
      ? <span className="px-2 py-0.5 rounded text-xs bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">{s.suspended_mailbox_count}</span>
      : <span className="text-xs text-slate-400">0</span>),
  },
]

export default function MailOverviewPage() {
  return (
    <OverviewList<Row>
      title="Email Accounts"
      icon="✉️"
      description="Mailbox and alias counts across every domain."
      endpoint="/overview/mail"
      columns={columns}
      searchField={(s) => s.domain_name}
      rowKey={(s) => s.domain_id}
      emptyMessage="No domains found."
      summary={(list): Badge[] => {
        const boxes = list.reduce((n, s) => n + s.mailbox_count, 0)
        const active = list.filter((s) => s.mail_enabled).length
        return [
          { label: 'Mail domains', value: active },
          { label: 'Total mailboxes', value: boxes },
        ]
      }}
    />
  )
}
