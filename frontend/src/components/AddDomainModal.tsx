import { useEffect, useState } from 'react'
import { api, apiError } from '@/lib/api'
import Modal from './Modal'

const PHP_FALLBACK = ['7.4', '8.1', '8.2', '8.3', '8.4']

type Plan = { id: number; name: string; php_version: string; is_default: boolean }
type Version = { version: string; description?: string }

export default function AddDomainModal({
  open, onClose, onAdded,
}: {
  open: boolean
  onClose: () => void
  onAdded: () => void
}) {
  const [domainName, setDomainName] = useState('')
  const [phpVersion, setPhpVersion] = useState('8.3')
  const [planId, setPlanId] = useState<number | ''>('')
  const [plans, setPlans] = useState<Plan[]>([])
  const [versions, setVersions] = useState<Version[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  // Fetch plans + installed PHP versions when the modal opens
  useEffect(() => {
    if (!open) return
    api.get<Plan[]>('/plans').then(r => {
      const list = r.data || []
      setPlans(list)
      // Pre-select the default plan and apply its PHP version
      const vars = list.find(p => p.is_default)
      if (vars) {
        setPlanId(vars.id)
        if (vars.php_version) setPhpVersion(vars.php_version)
      }
    }).catch(() => {})
    api.get<Version[]>('/php/versions').then(r => setVersions(r.data || [])).catch(() => {})
  }, [open])

  function handlePlanChange(v: string) {
    const idNum = v === '' ? '' : Number(v)
    setPlanId(idNum)
    if (idNum !== '') {
      const p = plans.find(x => x.id === idNum)
      if (p?.php_version) setPhpVersion(p.php_version)
    }
  }

  const phpOpts = Array.from(new Set([
    ...(versions.length ? versions.map(s => s.version) : PHP_FALLBACK),
    phpVersion,
  ].filter(Boolean)))

  const selectedPlan = planId === '' ? null : plans.find(p => p.id === planId)
  const isPlanPhpVersion = !!selectedPlan && selectedPlan.php_version === phpVersion

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null); setSuccess(null); setLoading(true)
    try {
      const payload: Record<string, unknown> = {
        domain_name: domainName.trim().toLowerCase(),
        php_version: phpVersion,
      }
      if (planId !== '') payload.plan_id = planId
      const { data } = await api.post('/domains', payload)
      setSuccess(`${data.domain_name} was created successfully (system user: ${data.system_user})`)
      setTimeout(() => {
        setDomainName('')
        setSuccess(null)
        onAdded()
        onClose()
      }, 1500)
    } catch (e) {
      setError(apiError(e, 'Unable to add domain'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal open={open} title="Add New Domain" onClose={onClose} width="md">
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">Domain Name</label>
          <input
            type="text"
            value={domainName}
            onChange={(e) => setDomainName(e.target.value)}
            placeholder="example.com"
            autoFocus
            required
            className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition text-sm"
          />
          <p className="text-xs text-slate-500 dark:text-slate-500 mt-1">Example: <code className="font-mono">site.com</code>, <code className="font-mono">customer-1.org</code></p>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">Plan (Package)</label>
            <select
              value={planId}
              onChange={(e) => handlePlanChange(e.target.value)}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition text-sm bg-white dark:bg-slate-800"
            >
              <option value="">No plan selected</option>
              {plans.map(p => (
                <option key={p.id} value={p.id}>{p.name}{p.is_default ? ' (default)' : ''}</option>
              ))}
            </select>
            <p className="text-xs text-slate-500 dark:text-slate-500 mt-1">Resource limits and the default PHP version come from this plan.</p>
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">PHP Version</label>
            <select
              value={phpVersion}
              onChange={(e) => setPhpVersion(e.target.value)}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition text-sm bg-white dark:bg-slate-800"
            >
              {phpOpts.map(v => <option key={v} value={v}>PHP {v}</option>)}
            </select>
            <p className="text-xs text-slate-500 dark:text-slate-500 mt-1">
              {isPlanPhpVersion ? <span className="text-brand-600 dark:text-brand-400">✓ From plan ({selectedPlan?.name})</span> : 'You can change this independently of the plan.'}
            </p>
          </div>
        </div>

        <div className="bg-sky-50 dark:bg-sky-900/20 border border-sky-200 rounded-md p-3 text-xs text-sky-800">
          <strong>Automatic provisioning:</strong> Linux user (<code className="font-mono">c_&lt;slug&gt;</code>) + home directory (<code className="font-mono">/home/c_&lt;slug&gt;/public_html</code>) + nginx vhost + welcome page
        </div>

        {error && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}
        {success && <div className="px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            disabled={loading}
            className="px-4 py-2 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 rounded-md text-sm transition"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={loading || !domainName.trim()}
            className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 rounded-md text-sm font-medium transition"
          >
            {loading ? 'Provisioning…' : 'Add Domain'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
