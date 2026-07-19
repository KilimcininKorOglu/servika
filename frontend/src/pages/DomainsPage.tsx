import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'

type Domain = {
  id: number; domain_name: string; system_user: string
  size_kb: number; traffic_kb: number; status: string; suspended?: boolean
  php_version?: string; is_demo?: boolean
  created_at?: string; plan_id?: number; plan_name?: string
}
type Plan = { id: number; name: string; disk_quota_mb?: number }
type PHPVer = { version: string; description?: string }
type CreateResult = {
  domain_name: string; system_user: string; ftp_user: string; ftp_host: string
  db_host: string; db_user: string; db_name: string
  created_passwords: { ftp: string; db: string }
}

function fmtKB(kb: number) {
  if (kb < 1024) return kb + ' KB'
  if (kb < 1024 * 1024) return (kb / 1024).toFixed(1) + ' MB'
  return (kb / 1024 / 1024).toFixed(2) + ' GB'
}

export default function DomainsPage() {
  const [items, setItems] = useState<Domain[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [processing, setProcessing] = useState(false)
  const [deleteConfirmationOpen, setDeleteConfirmationOpen] = useState(false)

  const [plans, setPlans] = useState<Plan[]>([])
  const [phpVersions, setPhpVersions] = useState<PHPVer[]>([])
  const [modalLoading, setModalLoading] = useState(false)
  const [modalReady, setModalReady] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [creationResult, setCreationResult] = useState<CreateResult | null>(null)
  const [formDomainName, setFormDomainName] = useState('')
  const [formPhpVersion, setFormPhpVersion] = useState('8.3')
  const [formPlanId, setFormPlanId] = useState<number | ''>('')

  // The domain list depends only on /domains. /plans and /php/versions (which can be
  // slow due to dnf discovery) are loaded lazily when the create modal opens.
  // The list renders as soon as domains arrive and never blocks on dnf.
  function load() {
    setLoading(true)
    api.get<Domain[]>('/domains')
      .then(r => setItems(r.data))
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [])

  // Load plans + PHP versions for the create modal — separate from the list load.
  // Called lazily when the modal first opens; cached after the first successful fetch.
  function loadModalData() {
    if (modalReady || modalLoading) return
    setModalLoading(true)
    Promise.all([
      api.get<Plan[]>('/plans').catch(() => ({ data: [] })),
      api.get<PHPVer[]>('/php/versions').catch(() => ({ data: [] })),
    ]).then(([plansResponse, phpVersionsResponse]) => {
      const pl = plansResponse.data as Plan[]
      setPlans(pl)
      setPhpVersions(phpVersionsResponse.data as PHPVer[])
      setModalReady(true)
      // If no plan has been selected yet (modal opened before data arrived) pick the default.
      setFormPlanId(prev => {
        if (prev !== '') return prev
        const d = pl.find(p => p.name === 'Starter') || pl[0]
        return d ? d.id : ''
      })
    }).finally(() => setModalLoading(false))
  }

  function openCreate() {
    setError(null); setSuccess(null); setCreationResult(null)
    // Default plan = "Starter" (if data has already arrived, pick it now; otherwise
    // loadModalData sets it once the fetch completes).
    const defaultPlan = plans.find(plan => plan.name === 'Starter') || plans[0]
    setFormDomainName(''); setFormPhpVersion('8.3'); setFormPlanId(defaultPlan ? defaultPlan.id : '')
    setCreateOpen(true)
    loadModalData() // lazy: fetch plans/php versions if they haven't been loaded yet
  }

  async function submitCreate(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const domainName = formDomainName.trim().toLowerCase()
    if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/.test(domainName)) {
      setError('Invalid domain name. For example: example.com or panel.example.com')
      return
    }
    setCreating(true)
    try {
      const request: { domain_name: string; php_version: string; plan_id?: number } = { domain_name: domainName, php_version: formPhpVersion }
      if (formPlanId !== '') request.plan_id = formPlanId
      const response = await api.post<CreateResult>('/domains', request)
      setCreateOpen(false)
      setCreationResult(response.data)
      setSuccess(`✓ "${domainName}" created. The Linux user, nginx vhost, PHP-FPM pool, FTP account, MySQL database, and DNS zone are ready.`)
      setTimeout(() => setSuccess(null), 8000)
      load()
    } catch (error) {
      setError(apiError(error, 'Could not create domain'))
    } finally {
      setCreating(false)
    }
  }

  async function copyToClipboard(text: string) {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text); return true
      }
    } catch {}
    try {
      const ta = document.createElement('textarea')
      ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0'
      document.body.appendChild(ta); ta.select(); document.execCommand('copy')
      document.body.removeChild(ta); return true
    } catch {}
    try { window.prompt('Press Ctrl+C to copy, then press Enter:', text); return true } catch {}
    return false
  }

  const filtered = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    if (!normalizedQuery) return items
    return items.filter(domain => domain.domain_name.toLowerCase().includes(normalizedQuery) || domain.system_user.toLowerCase().includes(normalizedQuery))
  }, [items, query])

  function toggleSelection(id: number) {
    setSelected(prev => {
      const nextSelection = new Set(prev)
      if (nextSelection.has(id)) nextSelection.delete(id); else nextSelection.add(id)
      return nextSelection
    })
  }
  function selectAllItems(shouldSelect: boolean) {
    if (shouldSelect) setSelected(new Set(filtered.map(d => d.id)))
    else setSelected(new Set())
  }

  async function bulkDelete() {
    setDeleteConfirmationOpen(false); setProcessing(true); setError(null)
    const ids = Array.from(selected); let successCount = 0
    for (const id of ids) {
      try { await api.delete(`/domains/${id}`); successCount++ } catch {}
    }
    setSelected(new Set()); setSuccess(`✓ ${successCount}/${ids.length} domains deleted`)
    setTimeout(() => setSuccess(null), 4000)
    setProcessing(false); load()
  }

  async function changeStatus(newStatus: 'active' | 'passive') {
    setProcessing(true); setError(null)
    const ids = Array.from(selected)
    try {
      await api.post('/domains/bulk/status', { ids, status: newStatus })
      setSuccess(`✓ ${ids.length} domains changed to "${newStatus}"`)
      setTimeout(() => setSuccess(null), 4000)
      setSelected(new Set()); load()
    } catch (error) { setError(apiError(error, 'Could not change status')) }
    finally { setProcessing(false) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'Domains' }]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-2">Domains</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        View all registered domains, change their status in bulk, or delete them.
      </p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {/* Toolbar */}
      <div className="flex items-center gap-2 mb-3 flex-wrap">
        <div className="flex-1 max-w-md">
          <input type="text" value={query} onChange={e => setQuery(e.target.value)}
            placeholder="🔍 Search domains..."
            className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm focus:border-brand-500 outline-none" />
        </div>
        <span className="text-xs text-slate-500 dark:text-slate-500">{filtered.length} / {items.length}</span>
        <button onClick={openCreate}
          className="ml-auto inline-flex items-center gap-1.5 text-sm px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-md font-medium shadow-sm">
          <span className="text-base leading-none">+</span> New Domain
        </button>
      </div>

      {/* Bulk action bar */}
      {selected.size > 0 && (
        <div className="mb-3 px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-300 dark:border-amber-700 rounded-md flex items-center gap-2 flex-wrap">
          <span className="text-sm font-semibold text-amber-800 dark:text-amber-200">{selected.size} selected</span>
          <button onClick={() => changeStatus('active')} disabled={processing}
            className="text-xs px-3 py-1.5 bg-emerald-600 hover:bg-emerald-700 text-white rounded">
            ▶ Activate
          </button>
          <button onClick={() => changeStatus('passive')} disabled={processing}
            className="text-xs px-3 py-1.5 bg-slate-600 hover:bg-slate-700 text-white rounded">
            ⏸ Deactivate
          </button>
          <button onClick={() => setDeleteConfirmationOpen(true)} disabled={processing}
            className="text-xs px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white rounded font-medium">
            🗑 Delete ({selected.size})
          </button>
          <button onClick={() => setSelected(new Set())} disabled={processing}
            className="text-xs px-3 py-1.5 border border-amber-300 dark:border-amber-700 text-amber-800 dark:text-amber-200 hover:bg-amber-100 dark:bg-amber-900/30 rounded">
            Clear selection
          </button>
        </div>
      )}

      {loading ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div>
      ) : items.length === 0 ? (
        <EmptyState title="No domains yet"
          description="Start by adding your first domain. A Linux user, nginx vhost, PHP-FPM pool, FTP account, MySQL database, and DNS zone will be created automatically."
          button={{ label: 'Create Domain', onClick: openCreate }} />
      ) : (
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
          <table className="w-full">
            <thead className="bg-slate-50 dark:bg-slate-900 text-xs uppercase tracking-wider text-slate-500 dark:text-slate-500 border-b border-slate-200 dark:border-slate-700">
              <tr>
                <th className="px-3 py-2.5 w-10 text-center">
                  <input type="checkbox"
                    checked={filtered.length > 0 && selected.size === filtered.length}
                    ref={ref => { if (ref) ref.indeterminate = selected.size > 0 && selected.size < filtered.length }}
                    onChange={e => selectAllItems(e.target.checked)}
                    className="cursor-pointer" />
                </th>
                <th className="text-left px-4 py-2.5">Domain Name</th>
                <th className="text-left px-4 py-2.5">System User</th>
                <th className="text-left px-4 py-2.5">Plan</th>
                <th className="text-left px-4 py-2.5">PHP</th>
                <th className="text-left px-4 py-2.5">Disk</th>
                <th className="text-left px-4 py-2.5">Status</th>
                <th className="text-left px-4 py-2.5">Created</th>
                <th className="text-right px-4 py-2.5">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {filtered.map(d => {
                return (
                  <tr key={d.id} className={`hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 transition ${selected.has(d.id) ? 'bg-brand-50 dark:bg-brand-900/20' : ''}`}>
                    <td className="px-3 py-2.5 text-center">
                      <input type="checkbox" checked={selected.has(d.id)}
                        onChange={() => toggleSelection(d.id)} className="cursor-pointer" />
                    </td>
                    <td className="px-4 py-2.5">
                      <Link to={`/subscriptions/${d.id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">
                        {d.domain_name}
                      </Link>
                      {d.is_demo && <span className="ml-2 text-[10px] uppercase tracking-wider bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 px-1.5 py-0.5 rounded">DEMO</span>}
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500">{d.system_user}</td>
                    <td className="px-4 py-2.5 text-sm">
                      {d.plan_name ? <span className="text-slate-700 dark:text-slate-300">{d.plan_name}</span> : <span className="text-slate-400 dark:text-slate-500 italic">—</span>}
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500">{d.php_version || '-'}</td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500">{fmtKB(d.size_kb)}</td>
                    <td className="px-4 py-2.5">
                      <span className={`text-[10px] uppercase tracking-wider px-2 py-0.5 rounded font-semibold ${
                        d.status === 'active' ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-500'
                      }`}>{d.status}</span>
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500 whitespace-nowrap">{d.created_at || '-'}</td>
                    <td className="px-4 py-2.5 text-right whitespace-nowrap">
                      <Link to={`/subscriptions/${d.id}/subdomains`} className="text-xs text-slate-500 dark:text-slate-400 hover:text-brand-600 dark:hover:text-brand-400 mr-3">+ Subdomain</Link>
                      <Link to={`/subscriptions/${d.id}`} className="text-xs text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300">Manage →</Link>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Domain creation modal */}
      {createOpen && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => !creating && setCreateOpen(false)}>
          <form onSubmit={submitCreate} className="bg-white dark:bg-slate-800 rounded-2xl w-full max-w-lg p-5 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-1">Create New Domain</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">
              The Linux user, nginx vhost, PHP-FPM pool, FTP account, MySQL database, and DNS zone are configured automatically.
            </p>

            {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}

            <div className="space-y-3">
              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Domain name <span className="text-red-500">*</span></label>
                <input
                  type="text"
                  value={formDomainName}
                  onChange={e => setFormDomainName(e.target.value)}
                  placeholder="example.com"
                  autoFocus
                  required
                  disabled={creating}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/15 outline-none"
                />
                <div className="text-[11px] text-slate-400 dark:text-slate-500 mt-1">Lowercase letters, numbers, hyphens, and periods. For example: <span className="font-mono">site.com</span> or <span className="font-mono">panel.site.com</span>.</div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">
                  PHP Version
                  {modalLoading && phpVersions.length === 0 && <span className="ml-2 text-[11px] text-slate-400 dark:text-slate-500">Loading…</span>}
                </label>
                <select
                  value={formPhpVersion}
                  onChange={e => setFormPhpVersion(e.target.value)}
                  disabled={creating}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm focus:border-brand-500 outline-none bg-white dark:bg-slate-800"
                >
                  {phpVersions.length === 0
                    ? <option value="8.3">PHP 8.3 (default)</option>
                    : phpVersions.map(p => (
                        <option key={p.version} value={p.version}>PHP {p.version}{p.description ? ` — ${p.description}` : ''}</option>
                      ))
                  }
                </select>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">
                  Service Plan
                  {modalLoading && plans.length === 0 && <span className="ml-2 text-[11px] text-slate-400 dark:text-slate-500">Loading…</span>}
                </label>
                <select
                  value={formPlanId}
                  onChange={e => setFormPlanId(e.target.value === '' ? '' : Number(e.target.value))}
                  disabled={creating}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm focus:border-brand-500 outline-none bg-white dark:bg-slate-800"
                >
                  <option value="">— (none) —</option>
                  {plans.map(p => (
                    <option key={p.id} value={p.id}>{p.name}</option>
                  ))}
                </select>
              </div>
            </div>

            <div className="flex justify-end gap-2 mt-5">
              <button type="button" onClick={() => setCreateOpen(false)} disabled={creating}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-sm rounded">Cancel</button>
              <button type="submit" disabled={creating || !formDomainName.trim()}
                className="px-4 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm rounded font-medium inline-flex items-center gap-2">
                {creating && (
                  <svg className="animate-spin w-3.5 h-3.5" viewBox="0 0 24 24" fill="none">
                    <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" opacity="0.3"/>
                    <path d="M22 12a10 10 0 0 1-10 10" stroke="currentColor" strokeWidth="3"/>
                  </svg>
                )}
                {creating ? 'Creating…' : 'Create'}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Creation result modal with FTP and database passwords */}
      {creationResult && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => setCreationResult(null)}>
          <div className="bg-white dark:bg-slate-800 rounded-2xl w-full max-w-lg p-5 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-emerald-700 dark:text-emerald-300 mb-1">✓ Domain Created</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">
              <span className="font-mono text-slate-700 dark:text-slate-300">{creationResult.domain_name}</span> is ready. The passwords below are shown <strong>only once</strong>. Store them securely.
            </p>

            <div className="space-y-3">
              <div className="border border-slate-200 dark:border-slate-700 rounded-md p-3 bg-slate-50 dark:bg-slate-900">
                <div className="text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-500 font-semibold mb-2">FTP</div>
                <CopyRow label="Host" value={creationResult.ftp_host || '—'} copy={copyToClipboard} />
                <CopyRow label="Username" value={creationResult.ftp_user} copy={copyToClipboard} />
                <CopyRow label="Password" value={creationResult.created_passwords.ftp} copy={copyToClipboard} password />
              </div>

              <div className="border border-slate-200 dark:border-slate-700 rounded-md p-3 bg-slate-50 dark:bg-slate-900">
                <div className="text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-500 font-semibold mb-2">MySQL Database</div>
                <CopyRow label="Host" value={creationResult.db_host || 'localhost'} copy={copyToClipboard} />
                <CopyRow label="Database" value={creationResult.db_name} copy={copyToClipboard} />
                <CopyRow label="Username" value={creationResult.db_user} copy={copyToClipboard} />
                <CopyRow label="Password" value={creationResult.created_passwords.db} copy={copyToClipboard} password />
              </div>

              <div className="text-[11px] text-slate-500 dark:text-slate-500 italic">
                System user: <span className="font-mono">{creationResult.system_user}</span>
              </div>
            </div>

            <div className="flex justify-end mt-5">
              <button onClick={() => setCreationResult(null)}
                className="px-4 py-1.5 bg-slate-700 hover:bg-slate-800 text-white text-sm rounded">OK</button>
            </div>
          </div>
        </div>
      )}

      {/* Bulk deletion confirmation */}
      {deleteConfirmationOpen && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => setDeleteConfirmationOpen(false)}>
          <div className="bg-white dark:bg-slate-800 rounded-2xl w-full max-w-md p-5 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-red-700 dark:text-red-300 mb-2">⚠ Bulk Domain Deletion</h3>
            <p className="text-sm text-slate-700 dark:text-slate-300 mb-3">
              <span className="font-semibold">{selected.size}</span> domains and all dependent resources (Linux users, home directories, databases, FTP accounts, vhosts, and DNS zones) will be <strong>permanently</strong> deleted.
            </p>
            <ul className="text-xs font-mono text-slate-500 dark:text-slate-500 bg-slate-50 dark:bg-slate-900 rounded p-2 max-h-40 overflow-auto mb-4">
              {Array.from(selected).slice(0, 8).map(id => {
                const d = items.find(x => x.id === id)
                return <li key={id} className="truncate">{d?.domain_name || '?'}</li>
              })}
              {selected.size > 8 && <li className="text-slate-400 dark:text-slate-500 italic">+ {selected.size - 8} more…</li>}
            </ul>
            <div className="flex justify-end gap-2">
              <button onClick={() => setDeleteConfirmationOpen(false)}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-sm rounded">Cancel</button>
              <button onClick={bulkDelete} disabled={processing}
                className="px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white text-sm rounded font-medium">
                Yes, Delete
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function CopyRow({ label, value, copy, password }: { label: string; value: string; copy: (text: string) => Promise<boolean>; password?: boolean }) {
  const [copied, setCopied] = useState(false)
  const [visible, setVisible] = useState(!password)
  async function handleClick() {
    const ok = await copy(value)
    if (ok) { setCopied(true); setTimeout(() => setCopied(false), 1500) }
  }
  return (
    <div className="flex items-center gap-2 text-xs py-1">
      <span className="w-24 text-slate-500 dark:text-slate-500 shrink-0">{label}</span>
      <code
        onClick={handleClick}
        className={`flex-1 font-mono px-2 py-1 rounded border cursor-pointer select-all transition ${
          copied ? 'border-emerald-300 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300' : 'border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:border-brand-400 text-slate-800 dark:text-slate-200'
        }`}
        title="Click to copy"
      >
        {password && !visible ? '••••••••••' : value}
      </code>
      {password && (
        <button type="button" onClick={() => setVisible(s => !s)}
          className="text-[10px] px-1.5 py-0.5 rounded border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-400 dark:text-slate-500 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800">
          {visible ? 'Hide' : 'Show'}
        </button>
      )}
      {copied && <span className="text-[10px] text-emerald-600 dark:text-emerald-400 font-semibold">Copied</span>}
    </div>
  )
}