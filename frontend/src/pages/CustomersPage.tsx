// Customer records (customers table) — the /customers CRUD endpoints have
// existed since the start but no UI was ever written; adding a customer was
// only possible over the API. Domains link to these records via
// domains.customer_id.
//
// NOTE: these are NOT panel login accounts — they are billing/contact records.
// The only panel login is the single admin (root); customers reach their own
// domains via FTP identity at /cp.
import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'
import ListToolbar from '@/components/ListToolbar'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

type Customer = {
  id: number
  name: string
  email: string
  plan_id: number | null
  status: string
  notes: string
  created_at: string
}

type Plan = { id: number; name: string }

const EMPTY: Customer = { id: 0, name: '', email: '', plan_id: null, status: 'active', notes: '', created_at: '' }

export default function CustomersPage() {
  const { t } = useTranslation('CustomersPage')
  // Global search (TopBar) deep-links here with ?q=<email|name>; seed the filter
  // from it and keep it in sync when the param changes without a remount.
  const [searchParams] = useSearchParams()
  const [list, setList] = useState<Customer[]>([])
  const [plans, setPlans] = useState<Plan[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [query, setQuery] = useState(() => searchParams.get('q') || '')

  useEffect(() => { setQuery(searchParams.get('q') || '') }, [searchParams])

  const [editing, setEditing] = useState<Customer | null>(null)
  const [saving, setSaving] = useState(false)
  const [toDelete, setToDelete] = useState<Customer | null>(null)

  async function fetchList() {
    setLoading(true)
    try {
      const r = await api.get<Customer[]>('/customers')
      setList(Array.isArray(r.data) ? r.data : [])
      setError(null)
    } catch (e) {
      setError(apiError(e, t('errors.loadFailed')))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList()
    api.get<Plan[]>('/plans')
      .then((r) => setPlans(Array.isArray(r.data) ? r.data : []))
      .catch(() => {})
  }, [])

  const filtered = useMemo(() => {
    const t = query.trim().toLowerCase()
    if (!t) return list
    return list.filter((m) => `${m.name} ${m.email} ${m.notes}`.toLowerCase().includes(t))
  }, [list, query])

  async function save() {
    if (!editing) return
    const name = editing.name.trim()
    const email = editing.email.trim()
    if (!name || !email) {
      setError(t('errors.nameEmailRequired'))
      return
    }
    setSaving(true)
    setError(null)
    try {
      const body = { name, email, plan_id: editing.plan_id, status: editing.status, notes: editing.notes }
      if (editing.id === 0) {
        await api.post('/customers', body)
        setSuccess(t('toast.added', { name }))
      } else {
        await api.put(`/customers/${editing.id}`, body)
        setSuccess(t('toast.updated', { name }))
      }
      setEditing(null)
      await fetchList()
    } catch (e) {
      setError(apiError(e, t('errors.saveFailed')))
    } finally {
      setSaving(false)
    }
  }

  async function remove() {
    if (!toDelete) return
    try {
      await api.delete(`/customers/${toDelete.id}`)
      setSuccess(t('toast.deleted', { name: toDelete.name }))
      setToDelete(null)
      await fetchList()
    } catch (e) {
      setError(apiError(e, t('errors.deleteFailed')))
      setToDelete(null)
    }
  }

  const planName = (id: number | null) =>
    id === null ? '—' : (plans.find((p) => p.id === id)?.name ?? `#${id}`)

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[{ label: t('breadcrumbHome'), href: '/' }, { label: t('title') }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
          {t('subtitle')}
        </p>
      </div>

      <ListToolbar
        primary={{ label: t('newCustomer'), onClick: () => setEditing({ ...EMPTY }) }}
        search={query}
        onSearchChange={setQuery}
      />

      {error && (
        <div className="mb-4 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">{error}</div>
      )}
      {success && (
        <div className="mb-4 px-3 py-2 rounded-lg bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 text-sm">{success}</div>
      )}

      {loading ? (
        <div className="py-16 text-center text-sm text-slate-400">{t('loading')}</div>
      ) : list.length === 0 ? (
        <EmptyState
          title={t('empty.title')}
          description={t('empty.description')}
          button={{ label: t('newCustomer'), onClick: () => setEditing({ ...EMPTY }) }}
        />
      ) : filtered.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-400">{t('noMatch')}</div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900/60">
              <tr>
                {['name', 'email', 'plan', 'status', 'created', ''].map((b, i) => (
                  <th key={i} className="px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 whitespace-nowrap">
                    {b ? t(`columns.${b}`) : ''}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800 bg-white dark:bg-slate-950">
              {filtered.map((m) => (
                <tr key={m.id} className="hover:bg-slate-50 dark:hover:bg-slate-900/60 transition">
                  <td className="px-3 py-2.5 font-medium text-slate-900 dark:text-slate-100 whitespace-nowrap">{m.name}</td>
                  <td className="px-3 py-2.5 text-slate-600 dark:text-slate-400 whitespace-nowrap">{m.email}</td>
                  <td className="px-3 py-2.5 text-slate-600 dark:text-slate-400 whitespace-nowrap">{planName(m.plan_id)}</td>
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    {m.status === 'active'
                      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{t('status.active')}</span>
                      : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">{t('status.passive')}</span>}
                  </td>
                  <td className="px-3 py-2.5 text-xs text-slate-500 whitespace-nowrap">{m.created_at}</td>
                  <td className="px-3 py-2.5 text-right whitespace-nowrap">
                    <button onClick={() => setEditing({ ...m })} className="text-xs text-brand-600 dark:text-brand-400 hover:underline mr-3">
                      {t('actions.edit')}
                    </button>
                    <button onClick={() => setToDelete(m)} className="text-xs text-red-600 dark:text-red-400 hover:underline">
                      {t('actions.delete')}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Modal
        open={editing !== null}
        title={editing?.id ? t('modal.editTitle') : t('modal.newTitle')}
        onClose={() => setEditing(null)}
      >
        {editing && (
          <div className="space-y-3">
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('modal.name')}</label>
              <input
                value={editing.name}
                onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('modal.email')}</label>
              <input
                type="email"
                value={editing.email}
                onChange={(e) => setEditing({ ...editing, email: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('modal.plan')}</label>
                <select
                  value={editing.plan_id ?? ''}
                  onChange={(e) => setEditing({ ...editing, plan_id: e.target.value === '' ? null : Number(e.target.value) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                >
                  <option value="">{t('modal.noPlan')}</option>
                  {plans.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('modal.status')}</label>
                <select
                  value={editing.status}
                  onChange={(e) => setEditing({ ...editing, status: e.target.value })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                >
                  <option value="active">{t('status.active')}</option>
                  <option value="passive">{t('status.passive')}</option>
                </select>
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('modal.notes')}</label>
              <input
                value={editing.notes}
                onChange={(e) => setEditing({ ...editing, notes: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button
                onClick={() => setEditing(null)}
                className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition"
              >
                {t('modal.cancel')}
              </button>
              <button
                onClick={save}
                disabled={saving}
                className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition"
              >
                {saving ? t('modal.saving') : t('modal.save')}
              </button>
            </div>
          </div>
        )}
      </Modal>

      <ConfirmDialog
        open={toDelete !== null}
        title={t('delete.title')}
        message={t('delete.message', { name: toDelete?.name ?? '' })}
        confirmText={t('delete.confirm')}
        dangerous
        onConfirm={remove}
        onCancel={() => setToDelete(null)}
      />
    </div>
  )
}
