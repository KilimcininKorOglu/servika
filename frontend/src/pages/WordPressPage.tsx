import { useEffect, useMemo, useState } from 'react'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import {
  responsiveTableActionCellClass,
  responsiveTableBodyClass,
  responsiveTableCellClass,
  responsiveTableClass,
  responsiveTableCodeCellClass,
  responsiveTableContainerClass,
  responsiveTableHeadClass,
  responsiveTableRowClass,
} from '@/lib/table'

type Domain = { id: number; domain_name: string }
type InstallationResult = { site_url: string; admin_url: string; admin_user: string; admin_password: string; version: string }
type Installation = {
  domain_id: number; domain_name: string; dir: string; version: string
  last_version: string; status: 'current' | 'outdated' | 'unknown'; install_date: string
  site_url: string; admin_url: string
}

const ROOT_DIRECTORY = '/ (root)'

/** Returns whether the directory value represents the domain root. */
function isRootDirectory(directory: string): boolean {
  return directory === ROOT_DIRECTORY
}

/** Returns the directory value for display. */
function displayDirectory(directory: string): string {
  return directory
}

export default function WordPressPage() {
  const [domains, setDomains] = useState<Domain[]>([])
  const [domainId, setDomainId] = useState<number | null>(null)
  const [installations, setInstallations] = useState<Installation[]>([])
  const [loadingInstallations, setLoadingInstallations] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [installing, setInstalling] = useState(false)
  const [result, setResult] = useState<InstallationResult | null>(null)
  const [busyKey, setBusyKey] = useState<string | null>(null)

  const [subdirectory, setSubdirectory] = useState('')
  const [siteTitle, setSiteTitle] = useState('')
  const [adminUser, setAdminUser] = useState('admin')
  const [adminEmail, setAdminEmail] = useState('')

  useEffect(() => {
    api.get<Domain[]>('/domains').then(response => {
      setDomains(response.data || [])
      if (response.data?.length) setDomainId(response.data[0].id)
    }).catch(cause => setError(apiError(cause)))
    listAll()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function listAll() {
    setLoadingInstallations(true)
    api.get<Installation[]>('/wordpress/all')
      .then(response => setInstallations(response.data || []))
      .catch(cause => setError(apiError(cause)))
      .finally(() => setLoadingInstallations(false))
  }

  async function install(event: React.FormEvent) {
    event.preventDefault()
    if (!domainId) return
    setError(null); setResult(null); setInstalling(true)
    try {
      const { data } = await api.post<InstallationResult>(`/domains/${domainId}/wordpress`, {
        sub_dir: subdirectory.trim(), site_title: siteTitle.trim(), admin_user: adminUser.trim(), admin_email: adminEmail.trim(),
      })
      setResult(data); setSiteTitle(''); setSubdirectory('')
      listAll()
    } catch (cause) { setError(apiError(cause, 'Installation failed')) }
    finally { setInstalling(false) }
  }

  async function update(installation: Installation) {
    const key = installation.domain_id + installation.dir
    setBusyKey(key); setError(null)
    try { await api.post(`/domains/${installation.domain_id}/wordpress/update`, { dir: installation.dir }); listAll() }
    catch (cause) { setError(apiError(cause, 'Could not update')) }
    finally { setBusyKey(null) }
  }

  async function remove(installation: Installation) {
    if (isRootDirectory(installation.dir)) { alert('WordPress in the root directory cannot be deleted from the panel.'); return }
    if (!confirm(`Delete WordPress at ${installation.domain_name}${installation.dir}?\nAll files and the database in this directory will be removed. This action cannot be undone.`)) return
    const key = installation.domain_id + installation.dir
    setBusyKey(key); setError(null)
    try {
      await api.delete(`/domains/${installation.domain_id}/wordpress`, { data: { dir: installation.dir, delete_db: true } })
      listAll()
    } catch (cause) { setError(apiError(cause, 'Could not delete')) }
    finally { setBusyKey(null) }
  }

  const selectedDomain = domains.find(domain => domain.id === domainId)
  const outdatedInstallations = useMemo(() => installations.filter(installation => installation.status === 'outdated'), [installations])

  return (
    <div className="px-4 py-4 sm:px-6 sm:py-5">
      <Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'WordPress' }]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">📝</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">WordPress</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">View and update all WordPress installations on the server, or create a new installation.</p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}

      {/* Security warning banner */}
      {!loadingInstallations && outdatedInstallations.length > 0 && (
        <div className="mb-4 px-4 py-3 rounded-2xl border border-amber-300 dark:border-amber-800 bg-amber-50 dark:bg-amber-900/20 flex items-start gap-3">
          <span className="text-lg leading-none">⚠️</span>
          <div className="text-sm text-amber-800 dark:text-amber-200">
            <strong>{outdatedInstallations.length} {outdatedInstallations.length === 1 ? 'installation has' : 'installations have'} an update available.</strong> Outdated WordPress versions contain known security vulnerabilities. Update them as soon as possible.
            <div className="mt-1 text-xs text-amber-700 dark:text-amber-300 font-mono">
              {outdatedInstallations.map(installation => `${installation.domain_name}${isRootDirectory(installation.dir) ? '' : installation.dir}`).join(', ')}
            </div>
          </div>
        </div>
      )}

      {/* Installation result with one-time credentials */}
      {result && (
        <div className="mb-4 rounded-2xl border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/15 p-4">
          <div className="flex items-center gap-2 text-sm font-semibold text-emerald-700 dark:text-emerald-300 mb-2">
            ✅ WordPress {result.version} installed
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1.5 text-sm">
            <Info label="Site" value={result.site_url} link />
            <Info label="Admin" value={result.admin_url} link />
            <Info label="User" value={result.admin_user} mono />
            <Info label="Password" value={result.admin_password} mono />
          </div>
          <p className="text-[11px] text-amber-700 dark:text-amber-400 mt-2">⚠ Save the password now. It will not be shown again.</p>
        </div>
      )}

      {/* Full-width table of all installations */}
      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden mb-6">
        <div className="flex items-center justify-between px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">Installed WordPress Sites {!loadingInstallations && <span className="text-slate-400 font-normal">({installations.length})</span>}</h3>
          <button onClick={listAll} disabled={loadingInstallations} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">↻ Refresh</button>
        </div>
        <div className={responsiveTableContainerClass}>
          <table className={responsiveTableClass}>
            <thead className={responsiveTableHeadClass}>
              <tr>
                <th className="text-left font-medium px-4 py-2.5">Domain</th>
                <th className="text-left font-medium px-4 py-2.5">Directory</th>
                <th className="text-left font-medium px-4 py-2.5">Version</th>
                <th className="text-left font-medium px-4 py-2.5">Status</th>
                <th className="text-left font-medium px-4 py-2.5 whitespace-nowrap">Installed</th>
                <th className="text-right font-medium px-4 py-2.5">Actions</th>
              </tr>
            </thead>
            <tbody className={responsiveTableBodyClass}>
              {loadingInstallations ? (
                <tr><td colSpan={6} className="px-4 py-10 text-center text-sm text-slate-400">Scanning installations... (checking versions and updates)</td></tr>
              ) : installations.length === 0 ? (
                <tr><td colSpan={6} className="px-4 py-10 text-center">
                  <div className="text-2xl mb-1">📝</div>
                  <p className="text-sm text-slate-500 dark:text-slate-400">No WordPress installations were found on the server.</p>
                  <p className="text-xs text-slate-400 mt-1">Use the form below to create a new installation.</p>
                </td></tr>
              ) : (
                installations.map(installation => {
                  const key = installation.domain_id + installation.dir
                  const isOutdated = installation.status === 'outdated'
                  return (
                    <tr key={key} className={`${responsiveTableRowClass} ${isOutdated ? 'bg-amber-50/50 dark:bg-amber-900/10' : ''}`}>
                      <td data-label="Domain" className={responsiveTableCellClass}>
                        <a href={installation.site_url} target="_blank" rel="noreferrer" className="font-medium text-slate-800 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400">{installation.domain_name}</a>
                      </td>
                      <td data-label="Directory" className={responsiveTableCodeCellClass}>{displayDirectory(installation.dir)}</td>
                      <td data-label="Version" className={responsiveTableCellClass}>
                        <span className="text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 font-mono font-semibold">{installation.version ? `v${installation.version}` : '-'}</span>
                      </td>
                      <td data-label="Status" className={responsiveTableCellClass}><StatusBadge installation={installation} /></td>
                      <td data-label="Installed" className={responsiveTableCodeCellClass}>{installation.install_date || '-'}</td>
                      <td className={responsiveTableActionCellClass}>
                        <div className="flex flex-wrap items-center justify-end gap-1.5">
                          <a href={installation.admin_url} target="_blank" rel="noreferrer" className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">Admin</a>
                          <button disabled={!!busyKey} onClick={() => update(installation)}
                            className={`text-xs px-2.5 py-1 rounded-md disabled:opacity-50 ${isOutdated ? 'bg-amber-500 hover:bg-amber-600 text-white' : 'border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700'}`}>
                            {busyKey === key ? '...' : isOutdated ? `Update to v${installation.last_version}` : 'Update'}
                          </button>
                          {!isRootDirectory(installation.dir) && (
                            <button disabled={!!busyKey} onClick={() => remove(installation)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">Delete</button>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* New installation */}
      <form onSubmit={install} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 max-w-2xl">
        <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">New Installation</h3>
        <div className="mb-3">
          <label className="block text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1.5">Domain</label>
          <select value={domainId ?? ''} onChange={event => setDomainId(Number(event.target.value))}
            className="w-full sm:w-80 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none">
            {domains.map(domain => <option key={domain.id} value={domain.id}>{domain.domain_name}</option>)}
          </select>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <Field label="Site Title" value={siteTitle} setValue={setSiteTitle} required placeholder="My Blog" />
          <Field label="Subdirectory (optional)" value={subdirectory} setValue={setSubdirectory} placeholder="blank = root, e.g. blog" mono />
          <Field label="Admin User" value={adminUser} setValue={setAdminUser} required mono />
          <Field label="Admin Email" value={adminEmail} setValue={setAdminEmail} required type="email" placeholder="admin@site.com" />
        </div>
        <button disabled={installing || !domainId} className="mt-3 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
          {installing ? 'Installing... (~30 sec)' : `Install WordPress${selectedDomain ? `, ${selectedDomain.domain_name}` : ''}`}
        </button>
      </form>
    </div>
  )
}

function StatusBadge({ installation }: { installation: Installation }) {
  if (installation.status === 'outdated') {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-amber-100 dark:bg-amber-900/40 text-amber-800 dark:text-amber-200 font-medium">
        <span className="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
        Update available{installation.last_version && ` to v${installation.last_version}`}
      </span>
    )
  }
  if (installation.status === 'current') {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300 font-medium">
        <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
        Up to date
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400 font-medium">
      Unknown
    </span>
  )
}

function Field({ label, value, setValue, required, placeholder, mono, type }: { label: string; value: string; setValue: (value: string) => void; required?: boolean; placeholder?: string; mono?: boolean; type?: string }) {
  return (
    <label className="block">
      <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{label}</span>
      <input value={value} onChange={event => setValue(event.target.value)} required={required} placeholder={placeholder} type={type || 'text'}
        className={`mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none ${mono ? 'font-mono' : ''}`} />
    </label>
  )
}
function Info({ label, value, mono, link }: { label: string; value: string; mono?: boolean; link?: boolean }) {
  return (
    <div className="flex items-baseline gap-1.5 min-w-0">
      <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold shrink-0">{label}</span>
      {link ? <a href={value} target="_blank" rel="noreferrer" className="text-xs text-brand-600 dark:text-brand-400 hover:underline truncate font-mono">{value}</a>
        : <span className={`text-xs text-slate-800 dark:text-slate-100 truncate ${mono ? 'font-mono' : ''}`}>{value}</span>}
    </div>
  )
}
