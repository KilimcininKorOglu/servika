import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import DomainList, { type Domain } from '@/components/DomainList'
import DomainDashboard from '@/components/DomainDashboard'
import ResourceCard from '@/components/ResourceCard'
import { useAuth } from '@/store/auth'

export default function DashboardPage() {
  const username = useAuth((s) => s.username)
  const [params, setParams] = useSearchParams()
  const [domains, setDomains] = useState<Domain[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    api.get<Domain[]>('/domains')
      .then((r) => {
        setDomains(r.data)
        if (!params.get('domain') && r.data.length > 0) {
          setParams({ domain: String(r.data[0].id) }, { replace: true })
        }
      })
      .catch((e) => setError(apiError(e)))
      .finally(() => setLoading(false))
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const selectedId = Number(params.get('domain')) || domains[0]?.id
  const selected = domains.find((d) => d.id === selectedId) || domains[0]

  function selectDomain(id: number) {
    setParams({ domain: String(id) })
  }

  return (
    <div className="px-6 py-5 max-w-[1600px]">
      <div className="mb-5 flex items-baseline justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Dashboard</h1>
          <p className="text-sm text-slate-500 dark:text-slate-500 mt-0.5">
            Welcome, <span className="text-slate-700 dark:text-slate-300 font-medium">{username?.full_name || username?.name}</span>
          </p>
        </div>
        {selected && (
          <div className="text-right text-xs text-slate-500 dark:text-slate-500">
            <span className="block">Selected domain</span>
            <span className="text-brand-700 dark:text-brand-300 font-mono font-semibold text-sm">{selected.domain_name}</span>
          </div>
        )}
      </div>

      {error && (
        <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">
          {error}
        </div>
      )}

      <div className="grid grid-cols-12 gap-5">
        <aside className="col-span-12 lg:col-span-3">
          <DomainList items={domains} selectedId={selected?.id} onSelect={selectDomain} loading={loading} />
        </aside>

        <section className="col-span-12 lg:col-span-6">
          {selected ? (
            <DomainDashboard domain={selected} />
          ) : (
            <div className="bg-white dark:bg-slate-800 border-2 border-dashed border-slate-200 dark:border-slate-700 rounded-2xl p-12 text-center text-slate-500 dark:text-slate-500">
              {loading ? 'Loading…' : 'No domains yet. Add one from the left panel.'}
            </div>
          )}
        </section>

        <aside className="col-span-12 lg:col-span-3">
          <ResourceCard />
        </aside>
      </div>
    </div>
  )
}