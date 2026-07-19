import { useEffect, useMemo, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ConfirmDialog from '@/components/ConfirmDialog'
import Modal from '@/components/Modal'

type Domain = { id: number; domain_name: string; system_user: string }
type DB = {
  id: number; domain_id: number; db_name: string; db_user: string;
  db_host: string; db_pass: string; created_at: string
}

export default function DomainDatabasesPage() {
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [databases, setDatabases] = useState<DB[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [databaseToDelete, setDatabaseToDelete] = useState<DB | null>(null)
  const [pwResetFor, setPwResetFor] = useState<DB | null>(null)
  const [addOpen, setAddOpen] = useState(false)
  const [passwordVisibility, setPasswordVisibility] = useState<Record<number, boolean>>({})
  const [copiedValue, setCopiedValue] = useState<number | null>(null)

  function load() {
    if (!id) return
    setLoading(true)
    api.get<DB[]>(`/domains/${id}/databases`)
      .then(r => setDatabases(r.data))
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }
  async function openPma(d: DB) {
    try {
      const { data } = await api.post<{ signon_url: string }>(`/databases/${d.id}/pma-token`)
      window.open(data.signon_url, '_blank', 'noopener')
    } catch (e) {
      alert(apiError(e, 'Could not obtain phpMyAdmin token'))
    }
  }

  useEffect(() => {
    if (id) api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    load()
  }, [id])

  async function remove() {
    if (!databaseToDelete) return
    try { await api.delete(`/databases/${databaseToDelete.id}`); setDatabaseToDelete(null); load() }
    catch (e) { alert(apiError(e, 'Deletion failed')) }
  }

  function copy(d: DB) {
    navigator.clipboard.writeText(d.db_pass)
    setCopiedValue(d.id)
    setTimeout(() => setCopiedValue(null), 1500)
  }

  // Unique existing DB users for this domain (used for the existing-user selector).
  const existingUsers = useMemo(
    () => Array.from(new Set(databases.map(d => d.db_user))),
    [databases],
  )

  return (
    <div className="px-6 py-5 max-w-[1300px]">
      <Breadcrumb items={[
        { label: 'Home', href: '/' }, { label: 'Domains', href: '/domains' },
        { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: 'Databases' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Databases</h1>
      {domain && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5"><Link to={`/subscriptions/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">{domain.domain_name}</Link></p>}

      <div className="flex items-center gap-2 mb-4">
        <button onClick={() => setAddOpen(true)} className="px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md">+ New Database</button>
        <button onClick={load} className="px-3 py-2 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md">↻ Refresh</button>
        <span className="ml-auto text-sm text-slate-500 dark:text-slate-500">{databases.length} databases</span>
      </div>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}

      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
        {loading ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div> :
         databases.length === 0 ? <div className="py-12 text-center text-sm text-slate-500 dark:text-slate-500">No databases yet</div> :
        <table className="w-full">
          <thead className="bg-slate-50 dark:bg-slate-900 text-xs uppercase tracking-wider text-slate-500 dark:text-slate-500 border-b border-slate-200 dark:border-slate-700">
            <tr>
              <th className="text-left px-4 py-2.5">Database</th>
              <th className="text-left px-4 py-2.5">Username</th>
              <th className="text-left px-4 py-2.5">Host</th>
              <th className="text-left px-4 py-2.5">Password</th>
              <th className="text-left px-4 py-2.5">Created</th>
              <th className="text-right px-4 py-2.5">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {databases.map(d => (
              <tr key={d.id} className="hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800">
                <td className="px-4 py-2.5 text-sm font-mono text-slate-800 dark:text-slate-200">{d.db_name}</td>
                <td className="px-4 py-2.5 text-sm font-mono text-slate-600 dark:text-slate-400 dark:text-slate-500">{d.db_user}</td>
                <td className="px-4 py-2.5 text-sm font-mono text-slate-600 dark:text-slate-400 dark:text-slate-500">{d.db_host}:3306</td>
                <td className="px-4 py-2.5 text-sm">
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => setPasswordVisibility({ ...passwordVisibility, [d.id]: !passwordVisibility[d.id] })}
                      className="font-mono text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 rounded"
                      title={passwordVisibility[d.id] ? 'Hide' : 'Show'}
                    >
                      {passwordVisibility[d.id] ? d.db_pass : '••••••••'}
                    </button>
                    {passwordVisibility[d.id] && (
                      <button onClick={() => copy(d)} className="text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 hover:bg-brand-100 dark:bg-brand-900/30 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 rounded" title="Copy">
                        {copiedValue === d.id ? '✓' : '⧉'}
                      </button>
                    )}
                  </div>
                </td>
                <td className="px-4 py-2.5 text-sm text-slate-600 dark:text-slate-400 dark:text-slate-500">{d.created_at}</td>
                <td className="px-4 py-2.5 text-right space-x-1">
                  <button onClick={() => openPma(d)} className="text-sm text-indigo-600 dark:text-indigo-400 hover:bg-indigo-50 dark:bg-indigo-900/20 px-2 py-1 rounded" title="Open phpMyAdmin in a new tab">🔓 phpMyAdmin</button>
                  <button onClick={() => setPwResetFor(d)} className="text-sm text-brand-600 dark:text-brand-400 hover:bg-brand-50 dark:hover:bg-brand-900/30 dark:bg-brand-900/20 px-2 py-1 rounded">🔑 Reset Password</button>
                  <button onClick={() => setDatabaseToDelete(d)} className="text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 px-2 py-1 rounded">Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>}
      </div>

      {addOpen && domain && (
        <NewDatabaseModal
          domainId={Number(id)}
          systemUser={domain.system_user}
          existingUsers={existingUsers}
          onClose={() => setAddOpen(false)}
          onDone={() => { setAddOpen(false); load() }}
        />
      )}

      {pwResetFor && (
        <PwResetModal
          db={pwResetFor}
          onClose={() => setPwResetFor(null)}
          onDone={() => { setPwResetFor(null); load() }}
        />
      )}

      <ConfirmDialog
        open={!!databaseToDelete}
        title="Delete database"
        message={`"${databaseToDelete?.db_name}" database and its user will be permanently deleted. This action cannot be undone.`}
        dangerous
        confirmText="Yes, delete"
        onConfirm={remove}
        onCancel={() => setDatabaseToDelete(null)}
      />
    </div>
  )
}

// generateStrongPassword builds a browser-side strong password (mixed letters+digits, passes the
// server-side strength check).
function generateStrongPassword(n = 20): string {
  const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789'
  const buf = new Uint32Array(n)
  ;(window.crypto || (window as unknown as { msCrypto: Crypto }).msCrypto).getRandomValues(buf)
  let s = ''
  for (let i = 0; i < n; i++) s += alphabet[buf[i] % alphabet.length]
  return s
}

type NewDatabaseModalProps = {
  domainId: number
  systemUser: string
  existingUsers: string[]
  onClose: () => void
  onDone: () => void
}

const SUFFIX_RE = /^[a-z0-9_]{1,32}$/

function NewDatabaseModal({ domainId, systemUser, existingUsers, onClose, onDone }: NewDatabaseModalProps) {
  const prefix = systemUser + '_'
  const [auto, setAuto] = useState(true)
  const [dbSuffix, setDbSuffix] = useState('')
  const [userMode, setUserMode] = useState<'new' | 'existing'>('new')
  const [userSuffix, setUserSuffix] = useState('')
  const [existingUser, setExistingUser] = useState(existingUsers[0] || '')
  const [password, setPassword] = useState('')
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<{ db_name: string; db_user: string; db_pass: string } | null>(null)

  const dbNamePreview = prefix + (dbSuffix || '…')
  const userPreview = prefix + (userSuffix || '…')
  const passwordStrengthIssue =
    password !== '' && (password.length < 12 || !/[A-Za-z]/.test(password) || !/[0-9]/.test(password))

  function localValidate(): string | null {
    if (auto) return null
    if (!SUFFIX_RE.test(dbSuffix)) return 'Database suffix: lowercase letters, digits, underscore only; 1-32 characters'
    if ((prefix + dbSuffix).length > 64) return 'Database name too long (prefix + suffix must be at most 64 characters)'
    if (userMode === 'new') {
      if (!SUFFIX_RE.test(userSuffix)) return 'User suffix: lowercase letters, digits, underscore only; 1-32 characters'
      if ((prefix + userSuffix).length > 64) return 'User name too long (prefix + suffix must be at most 64 characters)'
      if (password !== '' && passwordStrengthIssue) return 'Password must be at least 12 characters with letters and digits'
    } else {
      if (!existingUser) return 'Select an existing user'
    }
    return null
  }

  async function create() {
    const v = localValidate()
    if (v) { setError(v); return }
    setProcessing(true); setError(null)
    try {
      const body: Record<string, unknown> = auto
        ? { auto: true }
        : {
            db_suffix: dbSuffix,
            user_mode: userMode,
            ...(userMode === 'new'
              ? { user_suffix: userSuffix, password }
              : { existing_user: existingUser }),
          }
      const { data } = await api.post(`/domains/${domainId}/databases`, body)
      setResult({ db_name: data.db_name, db_user: data.db_user, db_pass: data.db_pass })
    } catch (e) {
      setError(apiError(e, 'Creation failed'))
    } finally {
      setProcessing(false)
    }
  }

  const inputCls = 'w-full px-3 py-2 border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 rounded-md text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none disabled:opacity-50'

  return (
    <Modal open={true} title="New Database" onClose={result ? onDone : onClose} width="lg">
      {result ? (
        <div className="space-y-4">
          <div className="bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md p-4 space-y-3">
            <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium">✓ Database created</p>
            <p className="text-xs text-emerald-700 dark:text-emerald-300">Save these in a secure place. You may not be able to view the password in plain text later:</p>
            <ResultRow label="Database" value={result.db_name} />
            <ResultRow label="User" value={result.db_user} />
            <ResultRow label="Password" value={result.db_pass} />
          </div>
          <div className="flex justify-end">
            <button onClick={onDone} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded-md">Done</button>
          </div>
        </div>
      ) : (
        <div className="space-y-5">
          <label className="flex items-center gap-3 cursor-pointer select-none">
            <input type="checkbox" checked={auto} onChange={e => setAuto(e.target.checked)} className="h-4 w-4 accent-brand-600" />
            <span className="text-sm text-slate-700 dark:text-slate-300">
              <strong className="font-medium">Automatic</strong> — let the panel generate the database name, user, and password
            </span>
          </label>

          {!auto && (
            <div className="space-y-5 pt-1">
              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">Database name</label>
                <div className="flex items-stretch">
                  <span className="inline-flex items-center px-3 rounded-l-md border border-r-0 border-slate-300 dark:border-slate-600 bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 text-sm font-mono select-none">{prefix}</span>
                  <input value={dbSuffix} onChange={e => setDbSuffix(e.target.value.toLowerCase())} placeholder="blog" className={inputCls + ' rounded-l-none'} />
                </div>
                <p className="mt-1 text-xs text-slate-400 dark:text-slate-500 font-mono">→ {dbNamePreview}</p>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1.5">Database user</label>
                <div className="flex gap-4 mb-2">
                  <label className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
                    <input type="radio" name="userMode" checked={userMode === 'new'} onChange={() => setUserMode('new')} className="accent-brand-600" />
                    New user
                  </label>
                  <label className={'flex items-center gap-1.5 text-sm cursor-pointer ' + (existingUsers.length ? 'text-slate-700 dark:text-slate-300' : 'text-slate-400 dark:text-slate-600 cursor-not-allowed')}>
                    <input type="radio" name="userMode" disabled={!existingUsers.length} checked={userMode === 'existing'} onChange={() => setUserMode('existing')} className="accent-brand-600" />
                    Select existing user
                  </label>
                </div>

                {userMode === 'new' ? (
                  <>
                    <div className="flex items-stretch">
                      <span className="inline-flex items-center px-3 rounded-l-md border border-r-0 border-slate-300 dark:border-slate-600 bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 text-sm font-mono select-none">{prefix}</span>
                      <input value={userSuffix} onChange={e => setUserSuffix(e.target.value.toLowerCase())} placeholder="bloguser" className={inputCls + ' rounded-l-none'} />
                    </div>
                    <p className="mt-1 text-xs text-slate-400 dark:text-slate-500 font-mono">→ {userPreview}</p>
                  </>
                ) : (
                  <select value={existingUser} onChange={e => setExistingUser(e.target.value)} className={inputCls}>
                    {existingUsers.map(u => <option key={u} value={u}>{u}</option>)}
                  </select>
                )}
              </div>

              {userMode === 'new' && (
                <div>
                  <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">Password <span className="text-slate-400 dark:text-slate-500">(leave blank to generate one)</span></label>
                  <div className="flex gap-2">
                    <input type="text" value={password} onChange={e => setPassword(e.target.value)} placeholder="At least 12 characters, letters and digits" className={inputCls} />
                    <button type="button" onClick={() => setPassword(generateStrongPassword())} className="whitespace-nowrap px-3 py-2 bg-white dark:bg-slate-800 border border-brand-600 text-brand-700 dark:text-brand-300 hover:bg-brand-50 dark:hover:bg-brand-900/30 text-sm rounded-md">Generate</button>
                  </div>
                  {passwordStrengthIssue && <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">Password must be at least 12 characters with letters and digits.</p>}
                </div>
              )}
            </div>
          )}

          {error && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-700 dark:text-red-300">{error}</div>}

          <div className="flex justify-end gap-2 pt-1">
            <button onClick={onClose} disabled={processing} className="px-4 py-2 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 rounded-md text-sm">Cancel</button>
            <button onClick={create} disabled={processing} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">{processing ? 'Creating…' : 'Create'}</button>
          </div>
        </div>
      )}
    </Modal>
  )
}

function ResultRow({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-center gap-2">
      <span className="w-24 shrink-0 text-xs text-emerald-700 dark:text-emerald-300">{label}</span>
      <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-1.5 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{value}</code>
      <button onClick={() => { navigator.clipboard.writeText(value); setCopied(true); setTimeout(() => setCopied(false), 1500) }} className="px-2.5 py-1.5 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">{copied ? '✓' : 'Copy'}</button>
    </div>
  )
}

function PwResetModal({ db, onClose, onDone }: { db: DB; onClose: () => void; onDone: () => void }) {
  const [customPassword, setCustomPassword] = useState('')
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [newPassword, setNewPassword] = useState<string | null>(null)

  async function reset(random: boolean) {
    if (!random && customPassword.length < 6) {
      setError('Password must be at least 6 characters')
      return
    }
    setProcessing(true); setError(null)
    try {
      const body = random ? {} : { password: customPassword }
      const { data } = await api.put(`/databases/${db.id}/password`, body)
      setNewPassword(data.db_pass)
    } catch (e) {
      setError(apiError(e, 'Password reset failed'))
    } finally {
      setProcessing(false)
    }
  }

  return (
    <Modal open={true} title={`Reset Password — ${db.db_name}`} onClose={newPassword ? onDone : onClose} width="md">
      {!newPassword ? (
        <div className="space-y-4">
          <div className="text-sm text-slate-600 dark:text-slate-400 dark:text-slate-500">
            <strong className="font-mono">{db.db_user}</strong> user's password is updated in MariaDB and the panel at the same time.
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Custom password (leave blank to generate one)</label>
            <input
              type="text"
              value={customPassword}
              onChange={e => setCustomPassword(e.target.value)}
              placeholder="At least 6 characters"
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none"
            />
          </div>
          {error && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-700 dark:text-red-300">{error}</div>}
          <div className="flex justify-end gap-2 pt-2">
            <button onClick={onClose} disabled={processing} className="px-4 py-2 border border-slate-200 dark:border-slate-700 rounded-md text-sm">Cancel</button>
            <button onClick={() => reset(false)} disabled={processing || !customPassword} className="px-4 py-2 bg-white dark:bg-slate-800 border border-brand-600 text-brand-700 dark:text-brand-300 hover:bg-brand-50 dark:hover:bg-brand-900/30 dark:bg-brand-900/20 disabled:opacity-50 rounded-md text-sm">Use This Password</button>
            <button onClick={() => reset(true)} disabled={processing} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">{processing ? 'Resetting…' : 'Generate Random Password'}</button>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          <div className="bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md p-4">
            <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium mb-2">✓ Password updated</p>
            <p className="text-xs text-emerald-700 dark:text-emerald-300 mb-2">Save this in a secure place. You cannot view it again later.</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-2 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{newPassword}</code>
              <button onClick={() => navigator.clipboard.writeText(newPassword)} className="px-3 py-2 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">Copy</button>
            </div>
          </div>
          <div className="flex justify-end">
            <button onClick={onDone} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded-md">Done</button>
          </div>
        </div>
      )}
    </Modal>
  )
}