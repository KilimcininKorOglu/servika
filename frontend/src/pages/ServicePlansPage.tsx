import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ListToolbar from '@/components/ListToolbar'
import EmptyState from '@/components/EmptyState'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

type Plan = {
  id: number
  name: string
  description: string
  disk_quota_mb: number
  traffic_quota_mb: number
  max_domain: number
  max_db: number
  max_email: number
  max_ftp: number
  php_version: string
  fastcgi_cache: boolean
  client_max_body_mb: number
  nginx_extra_directives: string
  waf_enabled: boolean
  waf_mode: string
  waf_paranoia: number
  is_default: boolean
  created_at: string
}
type Version = { version: string; description?: string }

export default function ServicePlansPage() {
  const [items, setItems] = useState<Plan[]>([])
  const [versions, setVersions] = useState<Version[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [modal, setModal] = useState<Plan | null>(null)
  const [planToDelete, setPlanToDelete] = useState<Plan | null>(null)

  function load() {
    setLoading(true); setError(null)
    api.get<Plan[]>('/plans')
      .then(response => setItems(response.data))
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [])
  useEffect(() => {
    api.get<Version[]>('/php/versions').then(response => setVersions(response.data || [])).catch(() => {})
  }, [])

  async function remove() {
    if (!planToDelete) return
    try {
      await api.delete(`/plans/${planToDelete.id}`)
      setPlanToDelete(null); load()
    } catch (e) {
      alert(apiError(e, 'Failed to delete'))
    }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'Service Plans' }]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-2">Service Plans</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">
        Define service plans for domains. Each domain is associated with a plan, and resources such as disk,
        traffic, PHP version, database count, and subdomain limits are configured per plan.
      </p>

      <ListToolbar
        primary={{ label: 'Add Plan', onClick: () => setModal({} as Plan) }}
        buttons={[]}
      />

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}

      {loading ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div>
      ) : items.length === 0 ? (
        <EmptyState
          title="No service plans yet"
          description="Start by defining your first plan."
          button={{ label: 'Add Plan', onClick: () => setModal({} as Plan) }}
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {items.map(plan => (
            <div key={plan.id} className={`bg-white dark:bg-slate-800 border rounded-2xl p-5 shadow-sm ${plan.is_default ? 'border-brand-400 ring-2 ring-brand-100 dark:ring-brand-900/40' : 'border-slate-200 dark:border-slate-700'}`}>
              <div className="flex items-start justify-between mb-2">
                <div className="min-w-0">
                  <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-100 flex items-center gap-2">
                    {plan.name}
                    {plan.is_default && <span className="text-[10px] uppercase tracking-wider bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 px-1.5 py-0.5 rounded font-semibold">Default</span>}
                  </h3>
                  {plan.description && <p className="text-sm text-slate-500 dark:text-slate-500 mt-0.5">{plan.description}</p>}
                </div>
                {plan.php_version && <span className="shrink-0 text-[11px] font-mono font-semibold bg-slate-100 dark:bg-slate-700/60 text-slate-600 dark:text-slate-300 px-2 py-0.5 rounded">PHP {plan.php_version}</span>}
              </div>

              <dl className="grid grid-cols-2 gap-y-1.5 text-sm mt-4">
                <Row label="Disk" value={formatLimit(plan.disk_quota_mb, 'MB')} />
                <Row label="Traffic" value={formatLimit(plan.traffic_quota_mb, 'MB/month')} />
                <Row label="Domains" value={formatLimit(plan.max_domain, 'domains')} />
                <Row label="Databases" value={formatLimit(plan.max_db, 'databases')} />
                <Row label="FTP" value={formatLimit(plan.max_ftp, 'accounts')} />
              </dl>

              <div className="mt-4 flex gap-2">
                <Link to={`/tools/packages/${plan.id}`} className="flex-1 text-center text-sm px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-md">
                  Details & Resource Limits
                </Link>
                <button onClick={() => setPlanToDelete(plan)} className="text-sm px-3 py-1.5 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 rounded-md">Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}

      {modal && (
        <PlanModal
          plan={modal}
          versions={versions}
          onClose={() => setModal(null)}
          onSave={() => { setModal(null); load() }}
        />
      )}

      <ConfirmDialog
        open={!!planToDelete}
        title="Delete plan"
        message={`Delete the plan "${planToDelete?.name}"?`}
        dangerous
        confirmText="Yes, delete"
        onConfirm={remove}
        onCancel={() => setPlanToDelete(null)}
      />
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt className="text-slate-500 dark:text-slate-500">{label}</dt>
      <dd className="text-slate-800 dark:text-slate-200 text-right font-mono">{value}</dd>
    </>
  )
}

function formatLimit(value: number, unit: string) {
  if (value <= 0) return 'unlimited'
  if (unit.startsWith('MB') && value >= 1024) return `${(value / 1024).toFixed(1)} G${unit.slice(2)}`
  return `${value.toLocaleString('en-US')} ${unit}`
}

function PlanModal({ plan, versions, onClose, onSave }: { plan: Plan; versions: Version[]; onClose: () => void; onSave: () => void }) {
  const newItem = !plan.id
  const [form, setForm] = useState<Plan>({
    id: plan.id || 0,
    name: plan.name || '',
    description: plan.description || '',
    disk_quota_mb: plan.disk_quota_mb || 1024,
    traffic_quota_mb: plan.traffic_quota_mb || 10240,
    max_domain: plan.max_domain || 1,
    max_db: plan.max_db || 1,
    max_email: plan.max_email || 0,
    max_ftp: plan.max_ftp || 2,
    php_version: plan.php_version || '8.3',
    fastcgi_cache: plan.fastcgi_cache || false,
    client_max_body_mb: plan.client_max_body_mb || 64,
    nginx_extra_directives: plan.nginx_extra_directives || '',
    waf_enabled: plan.waf_enabled || false,
    waf_mode: plan.waf_mode || 'on',
    waf_paranoia: plan.waf_paranoia || 1,
    is_default: plan.is_default || false,
    created_at: '',
  })
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const phpOptions = Array.from(new Set([
    ...versions.map(s => s.version),
    form.php_version,
    ...(versions.length === 0 ? ['7.4', '8.1', '8.2', '8.3', '8.4'] : []),
  ].filter(Boolean)))

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setProcessing(true); setError(null)
    try {
      if (newItem) await api.post('/plans', form)
      else await api.put(`/plans/${form.id}`, form)
      onSave()
    } catch (e) {
      setError(apiError(e, 'Failed to save'))
    } finally {
      setProcessing(false)
    }
  }

  return (
    <Modal open={true} title={newItem ? 'New Plan' : 'Edit Plan'} onClose={onClose} width="lg">
      <form onSubmit={submit} className="space-y-4">
        <div className="grid grid-cols-2 gap-3">
          <TextField label="Plan Name" value={form.name} setValue={value => setForm({ ...form, name: value })} required />
          <TextField label="Description" value={form.description} setValue={value => setForm({ ...form, description: value })} />
        </div>
        <div className="grid grid-cols-3 gap-3">
          <Count label="Disk (MB)" value={form.disk_quota_mb} setValue={value => setForm({ ...form, disk_quota_mb: value })} />
          <Count label="Traffic (MB)" value={form.traffic_quota_mb} setValue={value => setForm({ ...form, traffic_quota_mb: value })} />
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">PHP Version</label>
            <select value={form.php_version} onChange={e => setForm({ ...form, php_version: e.target.value })}
              className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-800 rounded text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none">
              {phpOptions.map(v => <option key={v} value={v}>PHP {v}</option>)}
            </select>
          </div>
          <Count label="Max Domains" value={form.max_domain} setValue={value => setForm({ ...form, max_domain: value })} />
          <Count label="Max Databases" value={form.max_db} setValue={value => setForm({ ...form, max_db: value })} />
          <Count label="Max FTP Accounts" value={form.max_ftp} setValue={value => setForm({ ...form, max_ftp: value })} />
        </div>
        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
          <input type="checkbox" checked={form.is_default} onChange={e => setForm({ ...form, is_default: e.target.checked })} className="rounded" />
          Default plan for new domains
        </label>

        {/* WAF (ModSecurity + OWASP CRS) plan default */}
        <div className="border-t border-slate-200 dark:border-slate-700 pt-3">
          <h4 className="text-sm font-semibold text-slate-700 dark:text-slate-300 mb-2">WAF Default (ModSecurity + OWASP CRS)</h4>
          <div className="grid grid-cols-3 gap-3">
            <label className="flex items-center gap-2 h-[38px] px-3 border border-slate-200 dark:border-slate-700 rounded-lg bg-slate-50/60 dark:bg-slate-900/40 cursor-pointer">
              <input type="checkbox" checked={form.waf_enabled} onChange={e => setForm({ ...form, waf_enabled: e.target.checked })} className="rounded" />
              <span className="text-sm text-slate-700 dark:text-slate-300">Enabled in this plan</span>
            </label>
            <select value={form.waf_mode} onChange={e => setForm({ ...form, waf_mode: e.target.value })}
              disabled={!form.waf_enabled}
              className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-800 rounded text-sm disabled:opacity-50">
              <option value="on">Block (On)</option>
              <option value="detect">Detect (log only)</option>
            </select>
            <select value={form.waf_paranoia} onChange={e => setForm({ ...form, waf_paranoia: Number(e.target.value) || 1 })}
              disabled={!form.waf_enabled}
              className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-800 rounded text-sm disabled:opacity-50">
              <option value={1}>Level 1 (Low)</option>
              <option value={2}>Level 2 (Medium)</option>
              <option value={3}>Level 3 (High)</option>
              <option value={4}>Level 4 (Strict)</option>
            </select>
          </div>
        </div>
        <p className="text-xs text-slate-500 dark:text-slate-500">0 = unlimited. Disk and traffic values are in MB. New domains on this plan are provisioned with the selected PHP version.</p>

        {error && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-700 dark:text-red-300">{error}</div>}

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onClose} className="px-4 py-2 border border-slate-200 dark:border-slate-700 rounded-md text-sm">Cancel</button>
          <button type="submit" disabled={processing || !form.name.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm rounded-md">{processing ? 'Saving…' : (newItem ? 'Add' : 'Update')}</button>
        </div>
      </form>
    </Modal>
  )
}

function TextField({ label, value, setValue, required }: { label: string; value: string; setValue: (value: string) => void; required?: boolean }) {
  return (
    <div>
      <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">{label}</label>
      <input type="text" value={value} onChange={e => setValue(e.target.value)} required={required}
        className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
    </div>
  )
}
function Count({ label, value, setValue }: { label: string; value: number; setValue: (value: number) => void }) {
  return (
    <div>
      <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">{label}</label>
      <input type="number" min={0} value={value} onChange={e => setValue(parseInt(e.target.value) || 0)}
        className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
    </div>
  )
}
