// Panel accounts (admin / reseller / customer) — the UI for the /users CRUD.
//
// Scope is enforced server-side (see internal/users): a reseller sees only the
// accounts beneath itself and may create customer accounts only. The role
// restrictions here mirror those rules for the UI; they are not a security
// boundary.
import { useEffect, useMemo, useState } from 'react'
import { api, apiError } from '@/lib/api'
import { useAuth } from '@/store/auth'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'
import ListToolbar from '@/components/ListToolbar'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

type User = {
  id: number
  username: string
  email: string
  full_name: string
  role: 'admin' | 'reseller' | 'user'
  status: 'active' | 'suspended'
  reseller_id: number | null
  two_fa: boolean
  last_login: string
  last_login_ip: string
  created_at: string
}

const ROLE_LABEL: Record<string, string> = {
  admin: 'Administrator',
  reseller: 'Reseller',
  user: 'Customer',
}
const ROLE_STYLE: Record<string, string> = {
  admin: 'bg-violet-50 text-violet-700 dark:bg-violet-900/20 dark:text-violet-300',
  reseller: 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-300',
  user: 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400',
}

type NewAccount = { username: string; password: string; email: string; full_name: string; role: string }
const EMPTY: NewAccount = { username: '', password: '', email: '', full_name: '', role: 'user' }

type ResellerLimit = {
  user_id: number
  max_customer: number
  max_domain: number
  disk_quota_mb: number
  traffic_quota_mb: number
  defined: boolean
  current_customer: number
  current_domain: number
  current_disk_mb: number
  current_traffic_mb: number
}

export default function UsersPage() {
  const myRole = useAuth((s) => s.username?.role)
  const myID = useAuth((s) => s.username?.id)
  const isAdmin = myRole === 'admin'

  const [list, setList] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [query, setQuery] = useState('')

  const [creating, setCreating] = useState<NewAccount | null>(null)
  const [saving, setSaving] = useState(false)
  const [pwTarget, setPwTarget] = useState<User | null>(null)
  const [newPassword, setNewPassword] = useState('')
  const [toDelete, setToDelete] = useState<User | null>(null)
  const [limitTarget, setLimitTarget] = useState<User | null>(null)
  const [limit, setLimit] = useState<ResellerLimit | null>(null)
  const [limitLoading, setLimitLoading] = useState(false)

  async function fetchList() {
    setLoading(true)
    try {
      const r = await api.get<User[]>('/users')
      setList(Array.isArray(r.data) ? r.data : [])
      setError(null)
    } catch (e) {
      setError(apiError(e, 'Could not load accounts'))
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => { fetchList() }, [])

  const filtered = useMemo(() => {
    const t = query.trim().toLowerCase()
    if (!t) return list
    return list.filter((k) => `${k.username} ${k.email} ${k.full_name}`.toLowerCase().includes(t))
  }, [list, query])

  async function create() {
    if (!creating) return
    setSaving(true)
    setError(null)
    try {
      await api.post('/users', creating)
      setSuccess(`Account ${creating.username} created.`)
      setCreating(null)
      await fetchList()
    } catch (e) {
      setError(apiError(e, 'Could not create account'))
    } finally {
      setSaving(false)
    }
  }

  async function resetPassword() {
    if (!pwTarget) return
    setSaving(true)
    setError(null)
    try {
      await api.post(`/users/${pwTarget.id}/password`, { new: newPassword })
      setSuccess(`Password updated for ${pwTarget.username}.`)
      setPwTarget(null)
      setNewPassword('')
    } catch (e) {
      setError(apiError(e, 'Could not reset password'))
    } finally {
      setSaving(false)
    }
  }

  async function openLimits(k: User) {
    setLimitTarget(k)
    setLimit(null)
    setLimitLoading(true)
    setError(null)
    try {
      const r = await api.get<ResellerLimit>(`/users/${k.id}/limits`)
      setLimit(r.data)
    } catch (e) {
      setError(apiError(e, 'Could not load limits'))
      setLimitTarget(null)
    } finally {
      setLimitLoading(false)
    }
  }

  async function saveLimits() {
    if (!limitTarget || !limit) return
    setSaving(true)
    setError(null)
    try {
      await api.put(`/users/${limitTarget.id}/limits`, {
        max_customer: limit.max_customer,
        max_domain: limit.max_domain,
        disk_quota_mb: limit.disk_quota_mb,
        traffic_quota_mb: limit.traffic_quota_mb,
      })
      setSuccess(`Limits updated for ${limitTarget.username}.`)
      setLimitTarget(null)
      setLimit(null)
    } catch (e) {
      setError(apiError(e, 'Could not save limits'))
    } finally {
      setSaving(false)
    }
  }

  async function toggleStatus(k: User) {
    const target = k.status === 'active' ? 'suspended' : 'active'
    setError(null)
    try {
      await api.post(`/users/${k.id}/status`, { status: target })
      setSuccess(`${k.username} ${target === 'active' ? 'enabled' : 'suspended'}.`)
      await fetchList()
    } catch (e) {
      setError(apiError(e, 'Could not change status'))
    }
  }

  async function remove() {
    if (!toDelete) return
    try {
      await api.delete(`/users/${toDelete.id}`)
      setSuccess(`${toDelete.username} deleted.`)
      setToDelete(null)
      await fetchList()
    } catch (e) {
      setError(apiError(e, 'Could not delete'))
      setToDelete(null)
    }
  }

  // No destructive action on root or your own account — the server rejects it
  // too; hiding the button avoids a pointless error message.
  const protectedRow = (k: User) => k.id === 1 || k.id === myID

  return (
    <div className="max-w-6xl mx-auto px-4 py-6">
      <Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'Users' }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Users</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
          {isAdmin
            ? 'Panel accounts: administrators, resellers and customers.'
            : 'The customer accounts you created.'}
        </p>
      </div>

      <ListToolbar
        primary={{ label: isAdmin ? 'New Account' : 'New Customer', onClick: () => setCreating({ ...EMPTY, role: isAdmin ? 'reseller' : 'user' }) }}
        search={query}
        onSearchChange={setQuery}
      />

      {error && <div className="mb-4 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">{error}</div>}
      {success && <div className="mb-4 px-3 py-2 rounded-lg bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 text-sm">{success}</div>}

      {loading ? (
        <div className="py-16 text-center text-sm text-slate-400">Loading…</div>
      ) : list.length === 0 ? (
        <EmptyState
          title={isAdmin ? 'No other accounts yet' : 'No customer accounts yet'}
          description="Start by creating a new account."
          button={{ label: 'New Account', onClick: () => setCreating({ ...EMPTY, role: isAdmin ? 'reseller' : 'user' }) }}
        />
      ) : filtered.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-400">No account matches your search.</div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900/60">
              <tr>
                {['User', 'Full Name', 'Role', 'Status', '2FA', 'Last Login', ''].map((b, i) => (
                  <th key={i} className="px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 whitespace-nowrap">{b}</th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800 bg-white dark:bg-slate-950">
              {filtered.map((k) => (
                <tr key={k.id} className="hover:bg-slate-50 dark:hover:bg-slate-900/60 transition">
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    <span className="font-mono text-slate-900 dark:text-slate-100">{k.username}</span>
                    {k.id === 1 && <span className="ml-1.5 text-[10px] text-slate-400">(system)</span>}
                  </td>
                  <td className="px-3 py-2.5 text-slate-600 dark:text-slate-400 whitespace-nowrap">{k.full_name || '—'}</td>
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    <span className={`px-2 py-0.5 rounded text-xs ${ROLE_STYLE[k.role]}`}>{ROLE_LABEL[k.role] ?? k.role}</span>
                  </td>
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    {k.status === 'active'
                      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">Active</span>
                      : <span className="px-2 py-0.5 rounded text-xs bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">Suspended</span>}
                  </td>
                  <td className="px-3 py-2.5 whitespace-nowrap text-xs">
                    {k.two_fa ? <span className="text-emerald-600 dark:text-emerald-400">On</span> : <span className="text-slate-400">Off</span>}
                  </td>
                  <td className="px-3 py-2.5 whitespace-nowrap text-xs text-slate-500">
                    {k.last_login || '—'}
                    {k.last_login_ip && <span className="ml-1 opacity-60">({k.last_login_ip})</span>}
                  </td>
                  <td className="px-3 py-2.5 text-right whitespace-nowrap">
                    {k.id === 1 ? (
                      <span className="text-xs text-slate-400">system account</span>
                    ) : (
                      <>
                        <button onClick={() => { setPwTarget(k); setNewPassword('') }} className="text-xs text-brand-600 dark:text-brand-400 hover:underline mr-3">
                          Password
                        </button>
                        {/* Quota is meaningful only for resellers and only admins manage it. */}
                        {isAdmin && k.role === 'reseller' && (
                          <button onClick={() => openLimits(k)} className="text-xs text-sky-600 dark:text-sky-400 hover:underline mr-3">
                            Limits
                          </button>
                        )}
                        {!protectedRow(k) && (
                          <>
                            <button onClick={() => toggleStatus(k)} className="text-xs text-amber-600 dark:text-amber-400 hover:underline mr-3">
                              {k.status === 'active' ? 'Suspend' : 'Enable'}
                            </button>
                            <button onClick={() => setToDelete(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">
                              Delete
                            </button>
                          </>
                        )}
                      </>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* New account */}
      <Modal open={creating !== null} title={isAdmin ? 'New Account' : 'New Customer Account'} onClose={() => setCreating(null)}>
        {creating && (
          <div className="space-y-3">
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Username</label>
              <input
                value={creating.username}
                onChange={(e) => setCreating({ ...creating, username: e.target.value })}
                placeholder="example_reseller"
                className="w-full px-3 py-2 text-sm font-mono rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
              <p className="mt-1 text-[11px] text-slate-400">3-32 characters, starts with a lowercase letter; may contain letters, digits, _ and -.</p>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Password</label>
              <input
                type="text"
                value={creating.password}
                onChange={(e) => setCreating({ ...creating, password: e.target.value })}
                className="w-full px-3 py-2 text-sm font-mono rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
              <p className="mt-1 text-[11px] text-slate-400">At least 8 characters. The password is shown only now — hand it to the user.</p>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Role</label>
                <select
                  value={creating.role}
                  onChange={(e) => setCreating({ ...creating, role: e.target.value })}
                  disabled={!isAdmin}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 disabled:opacity-60 focus:outline-none focus:ring-1 focus:ring-brand-500"
                >
                  {isAdmin && <option value="admin">Administrator</option>}
                  {isAdmin && <option value="reseller">Reseller</option>}
                  <option value="user">Customer</option>
                </select>
                {!isAdmin && <p className="mt-1 text-[11px] text-slate-400">Resellers can create customer accounts only.</p>}
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Email</label>
                <input
                  type="email"
                  value={creating.email}
                  onChange={(e) => setCreating({ ...creating, email: e.target.value })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Full Name</label>
              <input
                value={creating.full_name}
                onChange={(e) => setCreating({ ...creating, full_name: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button onClick={() => setCreating(null)} className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition">Cancel</button>
              <button onClick={create} disabled={saving} className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition">
                {saving ? 'Creating…' : 'Create'}
              </button>
            </div>
          </div>
        )}
      </Modal>

      {/* Password reset */}
      <Modal open={pwTarget !== null} title="Set Password" onClose={() => setPwTarget(null)}>
        {pwTarget && (
          <div className="space-y-3">
            <p className="text-sm text-slate-600 dark:text-slate-400">
              New password for <span className="font-mono">{pwTarget.username}</span>.
              {pwTarget.role === 'user' && (
                <span className="block mt-1.5 text-xs text-slate-500">
                  This is the customer's panel account password. Once set, the customer can also log in with this
                  username and password instead of their FTP credentials.
                </span>
              )}
            </p>
            <input
              type="text"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder="At least 8 characters"
              className="w-full px-3 py-2 text-sm font-mono rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
            />
            <div className="flex justify-end gap-2 pt-2">
              <button onClick={() => setPwTarget(null)} className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition">Cancel</button>
              <button onClick={resetPassword} disabled={saving || newPassword.length < 8} className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition">
                {saving ? 'Saving…' : 'Set Password'}
              </button>
            </div>
          </div>
        )}
      </Modal>

      {/* Reseller limits */}
      <Modal open={limitTarget !== null} title="Reseller Limits" onClose={() => { setLimitTarget(null); setLimit(null) }}>
        {limitLoading ? (
          <div className="py-8 text-center text-sm text-slate-400">Loading…</div>
        ) : limit && limitTarget ? (
          <div className="space-y-4">
            <p className="text-sm text-slate-600 dark:text-slate-400">
              Upper bounds for <span className="font-mono">{limitTarget.username}</span>.
              <span className="block mt-1 text-xs text-slate-500">
                <strong>0 = unlimited.</strong> If all are 0 the limit record is removed entirely.
              </span>
            </p>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  Max customers
                </label>
                <input
                  type="number"
                  min={0}
                  value={limit.max_customer}
                  onChange={(e) => setLimit({ ...limit, max_customer: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">
                  {limit.current_customer} in use now
                  {limit.max_customer > 0 && limit.current_customer > limit.max_customer && (
                    <span className="text-amber-600 dark:text-amber-400"> — limit is below current usage</span>
                  )}
                </p>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  Max domains
                </label>
                <input
                  type="number"
                  min={0}
                  value={limit.max_domain}
                  onChange={(e) => setLimit({ ...limit, max_domain: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">
                  {limit.current_domain} in use now
                  {limit.max_domain > 0 && limit.current_domain > limit.max_domain && (
                    <span className="text-amber-600 dark:text-amber-400"> — limit is below current usage</span>
                  )}
                </p>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  Disk quota (MB)
                </label>
                <input
                  type="number"
                  min={0}
                  value={limit.disk_quota_mb}
                  onChange={(e) => setLimit({ ...limit, disk_quota_mb: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">
                  {limit.current_disk_mb} MB in use now
                  {limit.disk_quota_mb > 0 && limit.current_disk_mb > limit.disk_quota_mb && (
                    <span className="text-amber-600 dark:text-amber-400"> — quota is below current usage</span>
                  )}
                </p>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  Traffic quota (MB)
                </label>
                <input
                  type="number"
                  min={0}
                  value={limit.traffic_quota_mb}
                  onChange={(e) => setLimit({ ...limit, traffic_quota_mb: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">
                  {limit.current_traffic_mb} MB used now
                  {limit.traffic_quota_mb > 0 && limit.current_traffic_mb > limit.traffic_quota_mb && (
                    <span className="text-amber-600 dark:text-amber-400"> — quota is below current usage</span>
                  )}
                </p>
              </div>
            </div>

            {!limit.defined && (
              <div className="px-3 py-2 rounded-lg bg-slate-50 dark:bg-slate-900 text-xs text-slate-500 dark:text-slate-400">
                No limit is defined for this reseller — currently unlimited.
              </div>
            )}

            <p className="text-[11px] text-slate-400">
              Lowering a limit below current usage does not delete existing accounts; it only blocks new additions.
            </p>

            <div className="flex justify-end gap-2 pt-1">
              <button onClick={() => { setLimitTarget(null); setLimit(null) }} className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition">
                Cancel
              </button>
              <button onClick={saveLimits} disabled={saving} className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition">
                {saving ? 'Saving…' : 'Save'}
              </button>
            </div>
          </div>
        ) : null}
      </Modal>

      <ConfirmDialog
        open={toDelete !== null}
        title="Delete account"
        message={`Account ${toDelete?.username ?? ''} will be deleted.${toDelete?.role === 'reseller' ? ' Accounts beneath this reseller are not deleted; they are transferred to the administrator.' : ''}`}
        confirmText="Delete"
        dangerous
        onConfirm={remove}
        onCancel={() => setToDelete(null)}
      />
    </div>
  )
}
