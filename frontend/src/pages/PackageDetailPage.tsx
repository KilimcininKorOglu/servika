import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Plan = {
  id: number; name: string; description: string
  disk_quota_mb: number; traffic_quota_mb: number
  max_domain: number; max_db: number; max_email: number; max_ftp: number
  cpu_percent: number; ram_mb: number; max_process: number
  inode_quota: number; io_weight: number; mysql_max_connections: number
  pm_max_children: number; php_version: string
  fastcgi_cache: boolean; client_max_body_mb: number; nginx_extra_directives: string
  is_default: boolean; created_at: string
}
type Domain = { id: number; domain_name: string; system_user: string; status: string; created_at: string }
type GetResponse = { plan: Plan; domain_count: number }
type Version = { version: string; description?: string }

export default function PackageDetailPage() {
  const { id } = useParams()
  const [plan, setPlan] = useState<Plan | null>(null)
  const [domainCount, setDomainCount] = useState(0)
  const [domains, setDomains] = useState<Domain[]>([])
  const [versions, setVersions] = useState<Version[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [processing, setProcessing] = useState(false)

  function load() {
    if (!id) return
    setLoading(true); setError(null)
    Promise.all([
      api.get<GetResponse>(`/plans/${id}`),
      api.get<Domain[]>(`/plans/${id}/domains`),
    ]).then(([planResponse, domainsResponse]) => {
      setPlan(planResponse.data.plan)
      setDomainCount(planResponse.data.domain_count)
      setDomains(domainsResponse.data || [])
    }).catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [id])
  useEffect(() => {
    api.get<Version[]>('/php/versions').then(response => setVersions(response.data || [])).catch(() => {})
  }, [])

  async function save() {
    if (!plan) return
    setProcessing(true); setError(null); setSuccess(null)
    try {
      await api.put(`/plans/${id}`, plan)
      setSuccess(`"${plan.name}" was saved. Use “Reapply” below to apply it to assigned domains.`)
      setTimeout(() => setSuccess(null), 6000)
      load()
    } catch (e) {
      setError(apiError(e, 'Failed to save'))
    } finally {
      setProcessing(false)
    }
  }

  async function reapplyForDomain(domainId: number) {
    if (!plan) return
    setProcessing(true)
    try {
      await api.put(`/domains/${domainId}/plan`, { plan_id: plan.id })
      setSuccess(`✓ Resource limits were reapplied for ${domains.find(domain => domain.id === domainId)?.domain_name}`)
      setTimeout(() => setSuccess(null), 4000)
    } catch (e) {
      setError(apiError(e))
    } finally { setProcessing(false) }
  }

  function updatePlan<K extends keyof Plan>(key: K, value: Plan[K]) {
    if (!plan) return
    setPlan({ ...plan, [key]: value })
  }

  if (loading) return <div className="px-6 py-5 text-slate-400">Loading…</div>
  if (!plan) return <div className="px-6 py-5"><div className="text-sm text-red-600">{error || 'Plan not found'}</div></div>

  // Include installed PHP versions and the plan's current value even when it is not installed.
  const phpOptions = Array.from(new Set([
    ...versions.map(version => version.version),
    plan.php_version,
    ...(versions.length === 0 ? ['7.4', '8.1', '8.2', '8.3', '8.4'] : []),
  ].filter(Boolean)))

  return (
    <div className="px-6 py-5">
      <div className="max-w-5xl mx-auto">
        <Breadcrumb items={[
          { label: 'Home', href: '/' },
          { label: 'Tools and Settings', href: '/tools-settings' },
          { label: 'Service Plans', href: '/tools/packages' },
          { label: plan.name },
        ]} />

        {/* Sticky header and save action */}
        <div className="sticky top-0 z-10 -mx-2 px-2 py-3 mb-4 bg-slate-50/85 dark:bg-slate-900/85 backdrop-blur border-b border-slate-200/70 dark:border-slate-800 flex items-center justify-between gap-4">
          <div className="min-w-0">
            <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100 flex items-center gap-2 truncate">
              {plan.name}
              {plan.is_default && <span className="shrink-0 text-[10px] uppercase font-semibold tracking-wider bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 px-1.5 py-0.5 rounded">Default</span>}
            </h1>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 truncate">
              {plan.description || 'No description'} · Used by <span className="font-mono">{domainCount}</span> domains
            </p>
          </div>
          <button onClick={save} disabled={processing}
            className="shrink-0 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-lg shadow-sm">
            {processing ? 'Saving…' : 'Save Changes'}
          </button>
        </div>

        {error && <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
        {success && <div className="mb-4 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

        {/* General settings */}
        <Card title="General" icon="⚙️">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Field label="Plan Name">
              <input value={plan.name} onChange={e => updatePlan('name', e.target.value)} className={inputClass} />
            </Field>
            <Field label="Default Plan">
              <label className="flex items-center gap-2 h-[38px] px-3 border border-slate-200 dark:border-slate-700 rounded-lg bg-slate-50/60 dark:bg-slate-900/40 cursor-pointer">
                <input type="checkbox" checked={plan.is_default} onChange={e => updatePlan('is_default', e.target.checked)} className="rounded" />
                <span className="text-sm text-slate-700 dark:text-slate-300">Assign automatically to new domains</span>
              </label>
            </Field>
            <Field label="Description" span={2}>
              <textarea value={plan.description} onChange={e => updatePlan('description', e.target.value)} rows={2} className={inputClass} />
            </Field>
          </div>
        </Card>

        {/* Defaults inherited by new domains */}
        <Card title="Defaults" icon="🧩" subtitle="Initial values applied when a new domain associated with this plan is created.">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <Field label="PHP Version" hint="New domains on this plan are provisioned with this PHP version.">
              <select value={plan.php_version} onChange={e => updatePlan('php_version', e.target.value)} className={inputClass}>
                {phpOptions.map(version => <option key={version} value={version}>PHP {version}</option>)}
              </select>
            </Field>
          </div>
        </Card>

        {/* Resource limits */}
        <Card title="Resource Limits" icon="📊" subtitle="Enforced at the system level using systemd cgroup, xfs_quota, and MariaDB GRANT. After saving, use “Reapply” for assigned domains.">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <Field label="CPU %" hint="100 = 1 core (systemd CPUQuota)">
              <input type="number" min={10} max={2000} value={plan.cpu_percent} onChange={e => updatePlan('cpu_percent', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
            <Field label="RAM (MB)" hint="Hard MemoryMax; MemoryHigh is automatically set to 90%">
              <input type="number" min={64} value={plan.ram_mb} onChange={e => updatePlan('ram_mb', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
            <Field label="Max Processes" hint="systemd TasksMax, including PHP-FPM workers">
              <input type="number" min={5} value={plan.max_process} onChange={e => updatePlan('max_process', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
            <Field label="MySQL Connections" hint="MAX_USER_CONNECTIONS">
              <input type="number" min={1} value={plan.mysql_max_connections} onChange={e => updatePlan('mysql_max_connections', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
            <Field label="PHP-FPM max_children" hint="0 = Automatic (max(4, RAM/64)). Keeps per-tenant PHP-FPM consistent with the memory limit.">
              <input type="number" min={0} value={plan.pm_max_children} onChange={e => updatePlan('pm_max_children', Number(e.target.value) || 0)} placeholder="0 = Automatic" className={numberInputClass} />
            </Field>
            <Field label="Disk (MB)" hint="0 = unlimited">
              <input type="number" min={0} value={plan.disk_quota_mb} onChange={e => updatePlan('disk_quota_mb', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
            <Field label="Traffic (MB/month)" hint="0 = unlimited">
              <input type="number" min={0} value={plan.traffic_quota_mb} onChange={e => updatePlan('traffic_quota_mb', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
            <Field label="Inode Quota" hint="Total number of files and directories">
              <input type="number" min={1000} value={plan.inode_quota} onChange={e => updatePlan('inode_quota', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
            <Field label="I/O Weight" hint="systemd IOWeight (1-1000)">
              <input type="number" min={1} max={1000} value={plan.io_weight} onChange={e => updatePlan('io_weight', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
          </div>
        </Card>

        {/* Numeric limits, excluding email */}
        <Card title="Numeric Limits" icon="🔢" subtitle="Number of resources that can be created under the account associated with this plan. 0 = unlimited.">
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
            <Field label="Domains">
              <input type="number" min={0} value={plan.max_domain} onChange={e => updatePlan('max_domain', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
            <Field label="Databases">
              <input type="number" min={0} value={plan.max_db} onChange={e => updatePlan('max_db', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
            <Field label="FTP Accounts">
              <input type="number" min={0} value={plan.max_ftp} onChange={e => updatePlan('max_ftp', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
          </div>
        </Card>

        {/* Web server settings for nginx */}
        <Card title="Web Server (nginx)" icon="🛠️" subtitle="New domains on this plan are provisioned with these nginx settings. Additional directives are validated with “nginx -t” when saved.">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
            <Field label="FastCGI Cache" hint="Caches dynamic PHP output in nginx for high-traffic sites">
              <label className="flex items-center gap-2 h-[38px] px-3 border border-slate-200 dark:border-slate-700 rounded-lg bg-slate-50/60 dark:bg-slate-900/40 cursor-pointer">
                <input type="checkbox" checked={plan.fastcgi_cache} onChange={e => updatePlan('fastcgi_cache', e.target.checked)} className="rounded" />
                <span className="text-sm text-slate-700 dark:text-slate-300">Enable for new domains</span>
              </label>
            </Field>
            <Field label="Upload Size Limit (MB)" hint="nginx client_max_body_size, the maximum file upload size">
              <input type="number" min={1} max={4096} value={plan.client_max_body_mb} onChange={e => updatePlan('client_max_body_mb', Number(e.target.value) || 0)} className={numberInputClass} />
            </Field>
          </div>
          <Field label="Additional nginx Directives" hint="Added to the server{} block and validated when saved.">
            <textarea
              value={plan.nginx_extra_directives || ''}
              onChange={e => updatePlan('nginx_extra_directives', e.target.value)}
              rows={6}
              spellCheck={false}
              placeholder={'# Example:\nadd_header X-Robots-Tag "noindex" always;\nlocation = /health { return 200 "ok"; }'}
              className={inputClass + ' font-mono text-xs leading-relaxed'}
            />
          </Field>
          <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">
            ⓘ When you save, the directives are tested in a temporary server block using <code className="font-mono">nginx -t</code>. If they are invalid, the plan is <strong>not saved</strong> and the nginx error output appears above.
          </p>
        </Card>

        {/* Assigned domains */}
        <Card title={`Assigned Domains (${domains.length})`} icon="🌐" subtitle="After updating the plan, use “Reapply” to update the domain's cgroup, quota, and MySQL limits.">
          {domains.length === 0 ? (
            <div className="text-sm text-slate-400 py-6 text-center">No domains are assigned to this plan yet.</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-xs uppercase tracking-wider text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-700">
                  <tr>
                    <th className="text-left py-2">Domain</th>
                    <th className="text-left">System User</th>
                    <th className="text-left">Status</th>
                    <th className="text-left">Created</th>
                    <th className="text-right">Action</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {domains.map(domain => (
                    <tr key={domain.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/60">
                      <td className="py-2"><Link to={`/subscriptions/${domain.id}`} className="text-brand-600 dark:text-brand-400 font-medium">{domain.domain_name}</Link></td>
                      <td className="font-mono text-xs">{domain.system_user}</td>
                      <td>
                        <span className={`text-[10px] uppercase tracking-wider px-2 py-0.5 rounded font-semibold ${
                          domain.status === 'active' ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-slate-100 dark:bg-slate-700 text-slate-500'
                        }`}>{domain.status}</span>
                      </td>
                      <td className="font-mono text-xs text-slate-500">{domain.created_at}</td>
                      <td className="text-right">
                        <button onClick={() => reapplyForDomain(domain.id)} disabled={processing}
                          className="text-xs px-2 py-1 border border-slate-300 dark:border-slate-600 rounded-md hover:bg-slate-50 dark:hover:bg-slate-800">
                          Reapply
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      </div>
    </div>
  )
}

const inputClass = 'w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-800 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none'
const numberInputClass = inputClass + ' font-mono'

function Card({ title, subtitle, icon, children }: { title: string; subtitle?: string; icon?: string; children: React.ReactNode }) {
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
      <div className="flex items-center gap-2 mb-1">
        {icon && <span className="text-base leading-none" aria-hidden>{icon}</span>}
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{title}</h3>
      </div>
      {subtitle && <p className="text-xs text-slate-500 dark:text-slate-400 mb-4 max-w-2xl">{subtitle}</p>}
      {children}
    </div>
  )
}

function Field({ label, hint, span, children }: { label: string; hint?: string; span?: number; children: React.ReactNode }) {
  return (
    <label className={`block ${span === 2 ? 'sm:col-span-2' : ''}`}>
      <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{label}</span>
      {hint && <span className="text-[10px] text-slate-400 dark:text-slate-500 ml-1 cursor-help" title={hint}>ⓘ</span>}
      <div className="mt-1.5">{children}</div>
    </label>
  )
}
