// Server-wide database overview. Sizes come from the root mysql client because
// the panel DSN cannot see other schemas (see internal/overview.dbSizes); 0
// means "size unavailable" and renders as "—".
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import OverviewList, { type Column, type Badge } from '@/components/OverviewList'

type Row = {
  id: number
  domain_id: number
  domain_name: string
  db_name: string
  db_user: string
  db_host: string
  size_kb: number
  created_at: string
}

function humanSize(kb: number): string {
  if (kb <= 0) return '—'
  if (kb < 1024) return `${kb} KB`
  const mb = kb / 1024
  if (mb < 1024) return `${mb.toFixed(1)} MB`
  return `${(mb / 1024).toFixed(2)} GB`
}

export default function DatabasesOverviewPage() {
  const { t } = useTranslation('DatabasesOverviewPage')
  const columns: Column<Row>[] = [
    {
      title: t('column.domain'),
      cell: (s) => (
        <Link to={`/subscriptions/${s.domain_id}/databases`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
          {s.domain_name}
        </Link>
      ),
    },
    { title: t('column.database'), cell: (s) => <span className="font-mono text-xs">{s.db_name}</span> },
    { title: t('column.user'), cell: (s) => <span className="font-mono text-xs">{s.db_user}</span> },
    { title: t('column.size'), cell: (s) => humanSize(s.size_kb) },
    { title: t('column.created'), cell: (s) => (s.created_at || <span className="text-slate-400">—</span>) },
  ]

  return (
    <OverviewList<Row>
      title={t('title')}
      icon="🗄️"
      description={t('description')}
      endpoint="/overview/databases"
      columns={columns}
      searchField={(s) => `${s.domain_name} ${s.db_name} ${s.db_user}`}
      rowKey={(s) => s.id}
      emptyMessage={t('emptyMessage')}
      summary={(list): Badge[] => {
        const totalKB = list.reduce((n, s) => n + Math.max(0, s.size_kb), 0)
        return [
          { label: t('summary.databases'), value: list.length },
          { label: t('summary.totalSize'), value: humanSize(totalKB) },
        ]
      }}
    />
  )
}
