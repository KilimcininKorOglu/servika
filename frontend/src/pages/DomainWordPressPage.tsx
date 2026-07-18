import { useCallback, useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Install = { dir: string; site_url: string; admin_url: string; version: string }
type Result = { site_url: string; admin_url: string; admin_user: string; admin_password: string; version: string }
type Status = { version: string; update_available: boolean; target_version: string; php: string; db_mb: string; maintenance: boolean }
type Package = { name: string; status: string; version: string; update: string; update_version: string }
type User = { ID: number; user_login: string; user_email: string; display_name: string; roles: string }

export default function DomainWordPressPage() {
  const { id } = useParams()
  const [items, setItems] = useState<Install[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [installing, setInstalling] = useState(false)
  const [result, setResult] = useState<Result | null>(null)
  const [formOpen, setFormOpen] = useState(false)

  const [domainName, setDomainName] = useState('')
  const [subdirectory, setSubdirectory] = useState('')
  const [title, setTitle] = useState('')
  const [adminUser, setAdminUser] = useState('admin')
  const [adminEmail, setAdminEmail] = useState('')

  useEffect(() => {
    if (!id) return
    api.get<{ domain_name: string }>(`/domains/${id}`).then(r => setDomainName(r.data.domain_name || '')).catch(() => {})
  }, [id])

  const list = useCallback(() => {
    if (!id) return
    setLoading(true)
    api.get<Install[]>(`/domains/${id}/wordpress`).then(r => setItems(r.data || [])).catch(() => setItems([])).finally(() => setLoading(false))
  }, [id])
  useEffect(() => { list() }, [list])

  async function install(e: React.FormEvent) {
    e.preventDefault()
    setError(null); setResult(null); setInstalling(true)
    try {
      const { data } = await api.post<Result>(`/domains/${id}/wordpress`, {
        sub_dir: subdirectory.trim(), site_title: title.trim(), admin_user: adminUser.trim(), admin_email: adminEmail.trim(),
      })
      setResult(data); setTitle(''); setSubdirectory(''); setFormOpen(false)
      list()
    } catch (error) { setError(apiError(error, 'Installation failed')) }
    finally { setInstalling(false) }
  }

  const emptyState = !loading && items.length === 0

  return (
    <div className="px-6 py-6 max-w-5xl">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: domainName || 'Subscription', href: `/subscriptions/${id}` },
        { label: 'WordPress' },
      ]} />
      <div className="flex items-center justify-between gap-4 mb-6 flex-wrap">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 tracking-tight">WordPress Toolkit</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">Manage versions, plugins, themes, and users in one place.</p>
        </div>
        {!emptyState && !formOpen && (
          <button onClick={() => { setFormOpen(true); setResult(null) }}
            className="inline-flex items-center gap-1.5 px-4 py-2.5 rounded-full bg-slate-900 dark:bg-white text-white dark:text-slate-900 text-sm font-medium hover:bg-slate-800 dark:hover:bg-slate-100 transition">
            <span className="text-base leading-none">+</span> New WordPress
          </button>
        )}
      </div>

      {error && <div className="mb-4 px-4 py-3 bg-red-50 dark:bg-red-900/20 border border-red-100 dark:border-red-800/60 rounded-2xl text-sm text-red-600 dark:text-red-300">{error}</div>}

      {result && <InstallResult s={result} close={() => setResult(null)} />}

      {loading ? (
        <div className="rounded-2xl border border-slate-200/70 dark:border-slate-700/60 bg-white dark:bg-slate-800/40 p-10 text-center text-sm text-slate-400">Loading…</div>
      ) : emptyState ? (
        <div className="rounded-2xl border border-dashed border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/40 p-12 text-center mb-5">
          <div className="w-12 h-12 mx-auto rounded-2xl bg-slate-100 dark:bg-slate-700/50 flex items-center justify-center text-2xl mb-3">📝</div>
          <p className="text-base font-medium text-slate-800 dark:text-slate-100">No WordPress installation exists on this domain yet</p>
          <p className="text-sm text-slate-400 mt-1">Install it with one click using the form below.</p>
        </div>
      ) : (
        <div className="space-y-5">
          {items.map(k => <Toolkit key={k.dir} id={id!} installation={k} onChange={list} />)}
        </div>
      )}

      {(emptyState || formOpen) && (
        <div className="mt-5">
          <InstallForm title={title} setTitle={setTitle} subdirectory={subdirectory} setSubdirectory={setSubdirectory}
            adminUser={adminUser} setAdminUser={setAdminUser} adminEmail={adminEmail} setAdminEmail={setAdminEmail}
            install={install} installing={installing} close={emptyState ? undefined : () => setFormOpen(false)} />
        </div>
      )}

      <div className="mt-6"><Link to={`/subscriptions/${id}`} className="text-sm text-slate-500 hover:text-slate-800 dark:hover:text-slate-200 transition">← Back to subscription</Link></div>
    </div>
  )
}

// ================= Toolkit: single installation card =================

type ToolkitTab = 'overview' | 'extensions' | 'themes' | 'users'
const TABS: { k: ToolkitTab; name: string }[] = [
  { k: 'overview', name: 'Overview' }, { k: 'extensions', name: 'Plugins' },
  { k: 'themes', name: 'Themes' }, { k: 'users', name: 'Users' },
]

function Toolkit({ id, installation, onChange }: { id: string; installation: Install; onChange: () => void }) {
  const dir = installation.dir
  const isRoot = dir === '/ (root)'
  const [tab, setTab] = useState<ToolkitTab>('overview')
  const [status, setStatus] = useState<Status | null>(null)
  const [extensions, setPlugins] = useState<Package[] | null>(null)
  const [themes, setThemes] = useState<Package[] | null>(null)
  const [users, setUsers] = useState<User[] | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [output, setOutput] = useState<string | null>(null)
  const [passwordResult, setPasswordResult] = useState<{ username: string; password: string } | null>(null)

  const qp = { params: { dir } }

  const loadStatus = useCallback(() => {
    api.get<Status>(`/domains/${id}/wordpress/status`, qp).then(r => setStatus(r.data)).catch(() => setStatus(null))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, dir])
  useEffect(() => { loadStatus() }, [loadStatus])

  useEffect(() => {
    if (tab === 'extensions' && extensions === null) api.get<Package[]>(`/domains/${id}/wordpress/plugins`, qp).then(r => setPlugins(r.data || [])).catch(() => setPlugins([]))
    if (tab === 'themes' && themes === null) api.get<Package[]>(`/domains/${id}/wordpress/themes`, qp).then(r => setThemes(r.data || [])).catch(() => setThemes([]))
    if (tab === 'users' && users === null) api.get<User[]>(`/domains/${id}/wordpress/users`, qp).then(r => setUsers(r.data || [])).catch(() => setUsers([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab])

  async function run(key: string, request: () => Promise<{ output?: string }>, successMessage: string, after?: () => void) {
    setBusy(key); setError(null); setSuccess(null); setOutput(null)
    try {
      const data = await request()
      setSuccess(successMessage)
      if (data?.output) setOutput(data.output)
      after?.()
    } catch (error) { setError(apiError(error, 'Operation failed')) }
    finally { setBusy(null) }
  }

  const updateVersion = () => run('version', async () => (await api.post(`/domains/${id}/wordpress/update`, { dir })).data, 'WordPress core updated.', () => { loadStatus(); onChange() })
  const updateAll = () => run('all', async () => (await api.post(`/domains/${id}/wordpress/tool`, { dir, action: 'update-all' })).data, 'Core, plugins, and themes updated.', () => { loadStatus(); setPlugins(null); setThemes(null); onChange() })
  const maintenanceToggle = () => run('maintenance', async () => (await api.post(`/domains/${id}/wordpress/tool`, { dir, action: status?.maintenance ? 'maintenance-off' : 'maintenance-on' })).data, status?.maintenance ? 'Maintenance mode disabled.' : 'Maintenance mode enabled.', loadStatus)
  const clearCache = () => run('cache', async () => (await api.post(`/domains/${id}/wordpress/tool`, { dir, action: 'cache-clear' })).data, 'Cache cleared.')
  const repair = () => run('repair', async () => (await api.post(`/domains/${id}/wordpress/repair`, { dir })).data, 'Core repair completed.', loadStatus)

  const updatePackage = (type: 'plugin' | 'theme', name: string) => run(`${type}:${name}`, async () => (await api.post(`/domains/${id}/wordpress/${type}`, { dir, action: 'update', name })).data, `${name} updated.`, () => { type === 'plugin' ? setPlugins(null) : setThemes(null) })
  const packageAll = (type: 'plugin' | 'theme') => run(`${type}:all`, async () => (await api.post(`/domains/${id}/wordpress/${type}`, { dir, action: 'update-all' })).data, 'All updates completed.', () => { type === 'plugin' ? setPlugins(null) : setThemes(null) })
  const pluginToggle = (plugin: Package) => run(`plugin:${plugin.name}`, async () => (await api.post(`/domains/${id}/wordpress/plugin`, { dir, action: plugin.status === 'active' ? 'passive' : 'active', name: plugin.name })).data, `${plugin.name} ${plugin.status === 'active' ? 'deactivated' : 'activated'}.`, () => setPlugins(null))
  const activateTheme = (theme: Package) => run(`theme:${theme.name}`, async () => (await api.post(`/domains/${id}/wordpress/theme`, { dir, action: 'active', name: theme.name })).data, `${theme.name} activated.`, () => setThemes(null))

  async function resetPassword(user: User) {
    if (!confirm(`Generate a new password for "${user.user_login}"?\nThe current password will become invalid.`)) return
    setBusy(`pw:${user.ID}`); setError(null); setSuccess(null)
    try {
      const { data } = await api.post<{ password: string; username: string }>(`/domains/${id}/wordpress/user-password`, { dir, user_id: user.ID })
      setPasswordResult({ username: data.username || user.user_login, password: data.password })
    } catch (error) { setError(apiError(error, 'Could not reset password')) }
    finally { setBusy(null) }
  }

  async function remove() {
    if (isRoot) { alert('WordPress installations in the root directory cannot be deleted from the panel.'); return }
    if (!confirm(`Delete the WordPress installation under ${dir}?\nAll files in this directory and its database will be removed. This cannot be undone.`)) return
    setBusy('remove'); setError(null)
    try { await api.delete(`/domains/${id}/wordpress`, { data: { dir, delete_db: true } }); onChange() }
    catch (err) { setError(apiError(err, 'Could not remove installation')) }
    finally { setBusy(null) }
  }

  const pluginUpdates = (extensions || []).filter(p => p.update === 'available').length
  const themeUpdates = (themes || []).filter(p => p.update === 'available').length
  const badge: Record<string, number> = { extensions: pluginUpdates, themes: themeUpdates }

  return (
    <div className="rounded-2xl border border-slate-200/70 dark:border-slate-700/60 bg-white dark:bg-slate-800/40 overflow-hidden">
      {/* Header strip */}
      <div className="flex items-center justify-between gap-3 px-5 pt-5 pb-4 flex-wrap">
        <div className="flex items-center gap-2.5 min-w-0">
          <div className="w-9 h-9 rounded-xl bg-slate-100 dark:bg-slate-700/50 flex items-center justify-center text-lg shrink-0">📝</div>
          <div className="min-w-0">
            <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">WordPress <span className="text-slate-400 font-normal font-mono text-xs">· {dir}</span></div>
            <div className="text-xs text-slate-400 mt-0.5 truncate">{installation.site_url}</div>
          </div>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {installation.admin_url && (
            <a href={installation.admin_url} target="_blank" rel="noreferrer"
              className="inline-flex items-center gap-1 px-3.5 py-2 rounded-full bg-slate-900 dark:bg-white text-white dark:text-slate-900 text-xs font-medium hover:bg-slate-800 dark:hover:bg-slate-100 transition">
              Admin panel <span className="opacity-70">↗</span>
            </a>
          )}
          {!isRoot && <button disabled={!!busy} onClick={remove} className="px-3 py-2 rounded-full border border-slate-200 dark:border-slate-700 text-xs text-slate-500 hover:border-red-300 hover:text-red-600 dark:hover:border-red-800 dark:hover:text-red-400 disabled:opacity-50 transition">{busy === 'remove' ? '…' : 'Remove'}</button>}
        </div>
      </div>

      {/* Metrics */}
      <div className="px-5 grid grid-cols-2 lg:grid-cols-4 gap-3">
        <Metric label="Version" v={status?.version ? status.version : '…'}
          pill={status ? (status.update_available ? { t: `↑ ${status.target_version}`, c: 'amber' } : { t: 'up to date', c: 'green' }) : undefined} />
        <Metric label="PHP" v={status?.php || '…'} />
        <Metric label="Database" v={status ? `${status.db_mb} MB` : '…'} />
        <Metric label="Maintenance mode" v={status?.maintenance ? 'On' : 'Off'}
          pill={status?.maintenance ? { t: 'active', c: 'amber' } : undefined} />
      </div>

      {/* Segmented tabs */}
      <div className="px-5 pt-5">
        <div className="inline-flex items-center gap-1 p-1 rounded-full bg-slate-100 dark:bg-slate-900/50">
          {TABS.map(t => (
            <button key={t.k} onClick={() => setTab(t.k)}
              className={`px-3.5 py-1.5 rounded-full text-sm font-medium transition ${tab === t.k
                ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100 shadow-sm'
                : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'}`}>
              {t.name}
              {!!badge[t.k] && badge[t.k] > 0 && <span className="ml-1.5 text-[10px] px-1.5 py-0.5 rounded-full bg-amber-100 dark:bg-amber-900/50 text-amber-700 dark:text-amber-300 font-semibold align-middle">{badge[t.k]}</span>}
            </button>
          ))}
        </div>
      </div>

      <div className="p-5">
        {error && <div className="mb-4 px-3.5 py-2.5 bg-red-50 dark:bg-red-900/20 border border-red-100 dark:border-red-800/60 rounded-xl text-xs text-red-600 dark:text-red-300">{error}</div>}
        {success && <div className="mb-4 px-3.5 py-2.5 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-100 dark:border-emerald-800/60 rounded-xl text-xs text-emerald-600 dark:text-emerald-300">{success}</div>}

        {tab === 'overview' && (
          <div>
            <div className="flex flex-wrap gap-2">
              {status?.update_available && <Btn onClick={updateVersion} waiting={busy === 'version'} type="primary">Update version · v{status.target_version}</Btn>}
              <Btn onClick={updateAll} waiting={busy === 'all'} type={status?.update_available ? 'outline' : 'primary'}>Update all</Btn>
              <Btn onClick={maintenanceToggle} waiting={busy === 'maintenance'}>{status?.maintenance ? 'Disable maintenance mode' : 'Enable maintenance mode'}</Btn>
              <Btn onClick={clearCache} waiting={busy === 'cache'}>Clear cache</Btn>
              <Btn onClick={repair} waiting={busy === 'repair'}>Repair core</Btn>
            </div>
            {output && <Output text={output} />}
            {!output && <p className="text-xs text-slate-400 mt-4">Quick maintenance actions. Available version updates appear in the metric above.</p>}
          </div>
        )}

        {tab === 'extensions' && (
          <PackageTable type="plugin" items={extensions} busy={busy}
            onUpdateAll={() => packageAll('plugin')} onUpdate={(p) => updatePackage('plugin', p.name)} onToggle={pluginToggle} />
        )}
        {tab === 'themes' && (
          <PackageTable type="theme" items={themes} busy={busy}
            onUpdateAll={() => packageAll('theme')} onUpdate={(p) => updatePackage('theme', p.name)} onActivate={activateTheme} />
        )}
        {tab === 'users' && <UserList items={users} busy={busy} onReset={resetPassword} />}

        {tab !== 'overview' && output && <Output text={output} />}
      </div>

      {passwordResult && <PasswordModal s={passwordResult} close={() => setPasswordResult(null)} />}
    </div>
  )
}

// ================= Components =================

function Metric({ label, v, pill }: { label: string; v: string; pill?: { t: string; c: 'green' | 'amber' | 'red' } }) {
  return (
    <div className="rounded-2xl bg-slate-50 dark:bg-slate-900/40 p-4">
      <div className="text-xs text-slate-400 font-medium">{label}</div>
      <div className="flex items-center gap-2 mt-1.5">
        <span className="text-xl font-semibold text-slate-900 dark:text-slate-100 tracking-tight">{v}</span>
        {pill && <StatusPill t={pill.t} c={pill.c} />}
      </div>
    </div>
  )
}

function StatusPill({ t, c }: { t: string; c: 'green' | 'amber' | 'red' | 'slate' }) {
  const cls = {
    green: 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-600 dark:text-emerald-300',
    amber: 'bg-amber-50 dark:bg-amber-900/30 text-amber-600 dark:text-amber-300',
    red: 'bg-red-50 dark:bg-red-900/30 text-red-600 dark:text-red-300',
    slate: 'bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-300',
  }[c]
  return <span className={`text-[11px] font-medium px-2 py-0.5 rounded-full ${cls}`}>{t}</span>
}

function Btn({ onClick, waiting, children, type }: { onClick: () => void; waiting: boolean; children: React.ReactNode; type?: 'primary' | 'outline' }) {
  const cls = type === 'primary'
    ? 'bg-slate-900 dark:bg-white text-white dark:text-slate-900 hover:bg-slate-800 dark:hover:bg-slate-100 border-transparent'
    : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-700/50'
  return (
    <button onClick={onClick} disabled={waiting} className={`text-sm px-4 py-2 rounded-full border font-medium disabled:opacity-50 transition ${cls}`}>
      {waiting ? 'Processing…' : children}
    </button>
  )
}

function Output({ text }: { text: string }) {
  const clean = text.replace(/\[[0-9;]*m/g, '')
  return (
    <details className="mt-4 group" open>
      <summary className="text-xs text-slate-400 cursor-pointer select-none hover:text-slate-600 dark:hover:text-slate-300">Operation output</summary>
      <pre className="mt-2 max-h-44 overflow-auto text-[12px] leading-relaxed bg-slate-50 dark:bg-slate-900/60 border border-slate-100 dark:border-slate-700/60 text-slate-600 dark:text-slate-300 rounded-xl p-3 whitespace-pre-wrap break-words">{clean}</pre>
    </details>
  )
}

function PackageTable({ type, items, busy, onUpdateAll, onUpdate, onToggle, onActivate }: {
  type: 'plugin' | 'theme'; items: Package[] | null; busy: string | null
  onUpdateAll: () => void; onUpdate: (p: Package) => void; onToggle?: (p: Package) => void; onActivate?: (p: Package) => void
}) {
  if (items === null) return <div className="text-sm text-slate-400 py-4">Loading…</div>
  if (items.length === 0) return <div className="text-sm text-slate-400 py-4">No {type === 'plugin' ? 'plugins' : 'themes'} found.</div>
  const updatable = items.filter(p => p.update === 'available').length
  return (
    <div>
      {updatable > 0 && (
        <div className="flex items-center justify-between mb-4 px-4 py-3 rounded-2xl bg-amber-50 dark:bg-amber-900/15 border border-amber-100 dark:border-amber-800/50">
          <span className="text-sm text-amber-700 dark:text-amber-300 font-medium">{updatable} {updatable === 1 ? 'update' : 'updates'} available</span>
          <button disabled={!!busy} onClick={onUpdateAll} className="text-sm px-4 py-1.5 rounded-full bg-slate-900 dark:bg-white text-white dark:text-slate-900 font-medium hover:bg-slate-800 dark:hover:bg-slate-100 disabled:opacity-50 transition">{busy === `${type}:all` ? '…' : 'Update all'}</button>
        </div>
      )}
      <div className="divide-y divide-slate-100 dark:divide-slate-700/50">
        {items.map(p => {
          const enabled = p.status === 'active'
          const updateAvailable = p.update === 'available'
          return (
            <div key={p.name} className="flex items-center justify-between gap-3 py-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-slate-800 dark:text-slate-100 truncate">{p.name}</span>
                  <StatusPill t={enabled ? 'Active' : 'Inactive'} c={enabled ? 'green' : 'slate'} />
                </div>
                <div className="text-xs text-slate-400 mt-0.5">
                  Version {p.version}{updateAvailable && <span className="text-amber-600 dark:text-amber-400"> → {p.update_version} available</span>}
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {updateAvailable && <button disabled={!!busy} onClick={() => onUpdate(p)} className="text-xs px-3 py-1.5 rounded-full bg-amber-500 hover:bg-amber-600 text-white font-medium disabled:opacity-50 transition">{busy === `${type}:${p.name}` ? '…' : 'Update'}</button>}
                {onToggle && <button disabled={!!busy} onClick={() => onToggle(p)} className="text-xs px-3 py-1.5 rounded-full border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700/50 disabled:opacity-50 transition">{busy === `plugin:${p.name}` ? '…' : enabled ? 'Deactivate' : 'Activate'}</button>}
                {onActivate && !enabled && <button disabled={!!busy} onClick={() => onActivate(p)} className="text-xs px-3 py-1.5 rounded-full border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700/50 disabled:opacity-50 transition">{busy === `theme:${p.name}` ? '…' : 'Activate'}</button>}
                {onActivate && enabled && <StatusPill t="Active theme" c="green" />}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function UserList({ items, busy, onReset }: { items: User[] | null; busy: string | null; onReset: (u: User) => void }) {
  if (items === null) return <div className="text-sm text-slate-400 py-4">Loading…</div>
  if (items.length === 0) return <div className="text-sm text-slate-400 py-4">No users found.</div>
  return (
    <div className="divide-y divide-slate-100 dark:divide-slate-700/50">
      {items.map(u => (
        <div key={u.ID} className="flex items-center justify-between gap-3 py-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="w-9 h-9 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center text-xs font-semibold text-slate-500 dark:text-slate-300 shrink-0">
              {(u.display_name || u.user_login).slice(0, 1).toUpperCase()}
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-slate-800 dark:text-slate-100 truncate">{u.user_login}</span>
                <StatusPill t={u.roles} c="slate" />
              </div>
              <div className="text-xs text-slate-400 truncate">{u.user_email}</div>
            </div>
          </div>
          <button disabled={!!busy} onClick={() => onReset(u)} className="text-xs px-3 py-1.5 rounded-full border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700/50 disabled:opacity-50 transition shrink-0">{busy === `pw:${u.ID}` ? '…' : 'Reset password'}</button>
        </div>
      ))}
    </div>
  )
}

// ================= Installation form, result, and modal =================

function InstallForm(props: {
  title: string; setTitle: (value: string) => void; subdirectory: string; setSubdirectory: (value: string) => void
  adminUser: string; setAdminUser: (value: string) => void; adminEmail: string; setAdminEmail: (value: string) => void
  install: (event: React.FormEvent) => void; installing: boolean; close?: () => void
}) {
  return (
    <form onSubmit={props.install} className="rounded-2xl border border-slate-200/70 dark:border-slate-700/60 bg-white dark:bg-slate-800/40 p-5">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">New WordPress installation</h3>
        {props.close && <button type="button" onClick={props.close} className="text-xs text-slate-400 hover:text-slate-600">✕ Close</button>}
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Input label="Site title" value={props.title} setValue={props.setTitle} required placeholder="My Blog" />
        <Input label="Subdirectory (optional)" value={props.subdirectory} setValue={props.setSubdirectory} placeholder="blank = root · example: blog" mono />
        <Input label="Admin username" value={props.adminUser} setValue={props.setAdminUser} required mono />
        <Input label="Admin email" value={props.adminEmail} setValue={props.setAdminEmail} required type="email" placeholder="admin@site.com" />
      </div>
      <button disabled={props.installing} className="mt-5 px-5 py-2.5 rounded-full bg-slate-900 dark:bg-white text-white dark:text-slate-900 text-sm font-medium hover:bg-slate-800 dark:hover:bg-slate-100 disabled:opacity-50 transition">
        {props.installing ? 'Installing… (~30 sec)' : 'Install WordPress'}
      </button>
    </form>
  )
}

function Input({ label, value, setValue, required, placeholder, mono, type }: { label: string; value: string; setValue: (value: string) => void; required?: boolean; placeholder?: string; mono?: boolean; type?: string }) {
  return (
    <label className="block">
      <span className="text-xs text-slate-500 dark:text-slate-400 font-medium">{label}</span>
      <input value={value} onChange={event => setValue(event.target.value)} required={required} placeholder={placeholder} type={type || 'text'}
        className={`mt-1.5 w-full px-3.5 py-2.5 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm text-slate-800 dark:text-slate-100 placeholder:text-slate-300 dark:placeholder:text-slate-600 focus:border-slate-400 dark:focus:border-slate-500 focus:ring-4 focus:ring-slate-100 dark:focus:ring-slate-800 outline-none transition ${mono ? 'font-mono' : ''}`} />
    </label>
  )
}

function InstallResult({ s, close }: { s: Result; close: () => void }) {
  return (
    <div className="mb-5 rounded-2xl border border-emerald-100 dark:border-emerald-800/60 bg-emerald-50/60 dark:bg-emerald-900/15 p-5">
      <div className="flex items-center justify-between mb-3">
        <div className="text-sm font-semibold text-emerald-700 dark:text-emerald-300">WordPress {s.version} installed</div>
        <button onClick={close} className="text-xs text-emerald-600/70 hover:text-emerald-700">✕</button>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-2 text-sm">
        <ResultRow label="Site" value={s.site_url} link />
        <ResultRow label="Admin" value={s.admin_url} link />
        <ResultRow label="Username" value={s.admin_user} mono />
        <ResultRow label="Password" value={s.admin_password} mono />
      </div>
      <p className="text-xs text-amber-700 dark:text-amber-400 mt-3">Save the password now. It will not be shown again.</p>
    </div>
  )
}

function ResultRow({ label, value, mono, link }: { label: string; value: string; mono?: boolean; link?: boolean }) {
  return (
    <div className="flex items-baseline gap-2 min-w-0">
      <span className="text-xs text-slate-400 shrink-0 w-16">{label}</span>
      {link ? <a href={value} target="_blank" rel="noreferrer" className="text-sm text-slate-700 dark:text-slate-200 hover:underline truncate">{value}</a>
        : <span className={`text-sm text-slate-800 dark:text-slate-100 truncate ${mono ? 'font-mono' : ''}`}>{value}</span>}
    </div>
  )
}

function PasswordModal({ s, close }: { s: { username: string; password: string }; close: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/30 backdrop-blur-sm p-4" onClick={close}>
      <div className="bg-white dark:bg-slate-800 rounded-2xl border border-slate-200 dark:border-slate-700 p-6 max-w-sm w-full shadow-xl" onClick={e => e.stopPropagation()}>
        <div className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-1">New password created</div>
        <div className="text-xs text-slate-400 mb-4">User: <span className="font-mono text-slate-600 dark:text-slate-300">{s.username}</span></div>
        <div className="flex items-center gap-2">
          <code className="flex-1 px-3.5 py-3 bg-slate-50 dark:bg-slate-900 rounded-xl text-sm font-mono text-slate-800 dark:text-slate-100 break-all border border-slate-100 dark:border-slate-700">{s.password}</code>
          <button onClick={() => navigator.clipboard?.writeText(s.password)} className="text-xs px-3.5 py-3 rounded-xl border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700/50 transition">Copy</button>
        </div>
        <p className="text-xs text-amber-600 dark:text-amber-400 mt-3">This password will not be shown again. Save it now.</p>
        <button onClick={close} className="mt-5 w-full py-2.5 rounded-full bg-slate-900 dark:bg-white text-white dark:text-slate-900 text-sm font-medium hover:bg-slate-800 dark:hover:bg-slate-100 transition">OK</button>
      </div>
    </div>
  )
}
