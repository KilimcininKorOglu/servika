export type Domain = {
  id: number
  domain_name: string
  php_version: string
  ssl: boolean
  ssl_expiry?: string
  status: 'active' | 'passive' | string
  suspended?: boolean
  system_user: string
  size_kb: number
  traffic_kb: number
  created_at: string
  ipv4: string
  ftp_host: string
  ftp_user: string
  db_host: string
  db_user: string
  db_name: string
  web_root: string
  notes?: string
  ssh_access?: boolean
}

export default function DomainList({
  items, selectedId, onSelect, loading,
}: {
  items: Domain[]
  selectedId?: number
  onSelect: (id: number) => void
  loading?: boolean
}) {
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
      <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700 flex items-center justify-between">
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
          Domains {!loading && <span className="text-slate-400 dark:text-slate-500 font-normal">({items.length})</span>}
        </h3>
        <button
          type="button"
          className="text-xs px-2 py-1 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded font-medium shadow-sm transition"
          title="Will be available in F2"
          disabled
        >
          + Add Domain
        </button>
      </div>

      <ul className="max-h-[640px] overflow-auto divide-y divide-slate-100 dark:divide-slate-800">
        {loading && (
          <li className="px-4 py-6 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</li>
        )}
        {!loading && items.length === 0 && (
          <li className="px-4 py-6 text-center text-sm text-slate-500 dark:text-slate-500">No domains yet</li>
        )}
        {items.map((d) => {
          const isSelected = d.id === selectedId
          return (
            <li key={d.id}>
              <button
                type="button"
                onClick={() => onSelect(d.id)}
                className={`w-full text-left px-4 py-3 transition ${
                  isSelected ? 'bg-brand-50 dark:bg-brand-900/20 border-l-4 border-brand-500 pl-3' : 'hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border-l-4 border-transparent'
                }`}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className={`text-sm font-medium truncate ${isSelected ? 'text-brand-700 dark:text-brand-300' : 'text-slate-900 dark:text-slate-100'}`}>
                    {d.domain_name}
                  </span>
                  <span
                    className={`text-[10px] px-1.5 py-0.5 rounded uppercase font-semibold tracking-wider flex-shrink-0 ${
                      d.status === 'active'
                        ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300'
                        : 'bg-slate-200 text-slate-600 dark:text-slate-400 dark:text-slate-500'
                    }`}
                  >
                    {d.status}
                  </span>
                </div>
                <div className="flex items-center gap-3 mt-1 text-xs text-slate-500 dark:text-slate-500">
                  <span className="font-mono">PHP {d.php_version}</span>
                  {d.ssl ? (
                    <span className="text-emerald-600 dark:text-emerald-400 flex items-center gap-1">
                      <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>SSL
                    </span>
                  ) : (
                    <span className="text-amber-600 dark:text-amber-400 flex items-center gap-1">
                      <span className="w-1.5 h-1.5 rounded-full bg-amber-400"></span>No SSL
                    </span>
                  )}
                  <span className="ml-auto">{Math.round(d.size_kb / 1024)} MB</span>
                </div>
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}