import { useEffect, useState } from 'react'
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

  async function add() {
    try {
      const { data } = await api.post(`/domains/${id}/databases`, {})
      alert(`New database:\n\nName: ${data.db_name}\nUser: ${data.db_user}\nPassword: ${data.db_pass}\n\nSave this password!`)
      load()
    } catch (e) {
      alert(apiError(e, 'Could not add item'))
    }
  }

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
        <button onClick={add} className="px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md">+ New Database</button>
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