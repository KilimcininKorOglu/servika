import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ConfirmDialog from '@/components/ConfirmDialog'
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

type Domain = { id: number; domain_name: string; system_user: string }
type Backup = { id: number; domain_id: number; type: string; file: string; size_b: number; notes: string; created_at: string }
type Schedule = { freq: 'none' | 'daily' | 'weekly'; hour: number; retention: number; last_backup_at?: string }
type Destination = {
  missing?: boolean
  id?: number; type?: 'ftp' | 'sftp'; host?: string; port?: number
  username?: string; remote_dir?: string; active?: boolean
  last_upload?: string; last_status?: string; last_error?: string
}

export default function DomainBackupsPage() {
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [backups, setBackups] = useState<Backup[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [processing, setProcessing] = useState(false)
  const [backupToDelete, setBackupToDelete] = useState<Backup | null>(null)
  const [restoreBackup, setRestoreBackup] = useState<Backup | null>(null)

  const [sched, setSched] = useState<Schedule>({ freq: 'none', hour: 3, retention: 7 })
  const [scheduleSaving, setScheduleSaving] = useState(false)

  const [dest, setDest] = useState<Destination>({ missing: true })
  const [destForm, setDestForm] = useState({ type: 'sftp' as 'ftp'|'sftp', host: '', port: 22, username: '', password: '', remote_dir: '/', active: true })
  const [destinationSaving, setDestinationSaving] = useState(false)
  const [destTest, setDestTest] = useState<{ ok: boolean; error?: string } | null>(null)

  function load() {
    if (!id) return
    setLoading(true)
    Promise.all([
      api.get<Backup[]>(`/domains/${id}/backups`),
      api.get<Schedule>(`/domains/${id}/backup-schedule`).catch(() => ({ data: { freq: 'none', hour: 3, retention: 7 } as Schedule })),
      api.get<Destination>(`/domains/${id}/backup-destination`).catch(() => ({ data: { missing: true } as Destination })),
    ]).then(([y, s, d]) => {
      setBackups(y.data)
      setSched(s.data)
      setDest(d.data)
      if (!d.data.missing) {
        setDestForm({
          type: (d.data.type || 'sftp') as 'ftp'|'sftp',
          host: d.data.host || '',
          port: d.data.port || (d.data.type === 'ftp' ? 21 : 22),
          username: d.data.username || '',
          password: '',  // Security: leave blank unless the user chooses to enter it again.
          remote_dir: d.data.remote_dir || '/',
          active: !!d.data.active,
        })
      }
    })
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }

  async function saveDest() {
    setDestinationSaving(true); setError(null); setSuccess(null); setDestTest(null)
    try {
      const r = await api.put<Destination>(`/domains/${id}/backup-destination`, destForm)
      setDest(r.data)
      setSuccess('Remote destination saved')
      setTimeout(() => setSuccess(null), 4000)
    } catch (e) {
      setError(apiError(e, 'Could not save destination'))
    } finally {
      setDestinationSaving(false)
    }
  }

  async function testDestination() {
    setDestinationSaving(true); setDestTest(null)
    try {
      const r = await api.post<{ ok: boolean; error?: string }>(`/domains/${id}/backup-destination/test`, destForm)
      setDestTest(r.data)
      setTimeout(() => setDestTest(null), 8000)
    } catch (e) {
      setDestTest({ ok: false, error: apiError(e) })
    } finally {
      setDestinationSaving(false)
    }
  }

  async function destDelete() {
    if (!confirm('Delete the remote backup destination? Existing backups are unaffected, but future automatic uploads will stop.')) return
    setDestinationSaving(true)
    try {
      await api.delete(`/domains/${id}/backup-destination`)
      setDest({ missing: true })
      setDestForm({ type: 'sftp', host: '', port: 22, username: '', password: '', remote_dir: '/', active: true })
      setSuccess('Remote destination deleted')
      setTimeout(() => setSuccess(null), 4000)
    } catch (e) {
      setError(apiError(e))
    } finally {
      setDestinationSaving(false)
    }
  }
  useEffect(() => {
    if (id) api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    load()
  }, [id])

  async function saveSchedule(newSchedule: Schedule) {
    setScheduleSaving(true); setError(null); setSuccess(null)
    try {
      const r = await api.put<{ schedule: Schedule }>(`/domains/${id}/backup-schedule`, newSchedule)
      setSched(r.data.schedule)
      setSuccess(newSchedule.freq === 'none'
        ? 'Automatic backups disabled'
        : `Automatic backups enabled: ${newSchedule.freq === 'daily' ? 'Daily' : 'Weekly'}, ${String(newSchedule.hour).padStart(2,'0')}:00; the latest ${newSchedule.retention} backups will be retained`)
      setTimeout(() => setSuccess(null), 5000)
    } catch (e) {
      setError(apiError(e, 'Could not save schedule'))
    } finally {
      setScheduleSaving(false)
    }
  }

  async function create() {
    setProcessing(true); setError(null); setSuccess(null)
    try {
      await api.post(`/domains/${id}/backups`)
      setSuccess('Backup created')
      load()
    } catch (e) {
      setError(apiError(e, 'Could not create backup'))
    } finally {
      setProcessing(false)
    }
  }

  async function remove() {
    if (!backupToDelete) return
    try {
      await api.delete(`/domains/${id}/backups/${backupToDelete.id}`)
      setBackupToDelete(null); load()
    } catch (e) {
      alert(apiError(e))
    }
  }

  async function restore() {
    if (!restoreBackup) return
    setProcessing(true); setError(null); setSuccess(null)
    try {
      const { data } = await api.post(`/domains/${id}/backups/${restoreBackup.id}/restore`)
      setSuccess(`Restored: ${data.domain_name}, ${data.db_import || ''}`)
      setRestoreBackup(null)
    } catch (e) {
      setError(apiError(e, 'Restore failed'))
    } finally {
      setProcessing(false)
    }
  }

  function download(y: Backup) {
    const tok = localStorage.getItem('servika.token') || ''
    fetch(`/api/v1/domains/${id}/backups/${y.id}/download`, { headers: { Authorization: `Bearer ${tok}` } })
      .then(r => r.blob())
      .then(blob => {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = y.file
        a.click()
      })
  }

  return (
    <div className="px-4 py-4 sm:px-6 sm:py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' }, { label: 'Domains', href: '/domains' },
        { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: 'Backups' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Backups</h1>
      {domain && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        <Link to={`/subscriptions/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">{domain.domain_name}</Link>
        {', '}home + MySQL dump = tar.gz, {sched.freq === 'none'
          ? 'Automatic backups disabled'
          : `${sched.freq === 'daily' ? 'Daily' : 'Weekly'} ${String(sched.hour).padStart(2,'0')}:00, latest ${sched.retention} automatic backups retained`}
      </p>}

      {/* Automatic backup schedule */}
      <div className="mb-5 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <div className="flex flex-col gap-3 mb-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Automatic Backup Schedule</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">
              When enabled, the panel checks hourly. A backup is created at the selected hour, and backups beyond retention are deleted.
            </p>
          </div>
          {sched.last_backup_at && (
            <div className="text-xs text-slate-500 dark:text-slate-500">Latest automatic backup: <span className="font-mono">{sched.last_backup_at.replace('T',' ').replace('Z','')}</span></div>
          )}
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          {(['none','daily','weekly'] as const).map(f => {
            const isSelected = sched.freq === f
            const meta: Record<string,{name:string;icon:string;description:string;color:string}> = {
              none: { name:'Disabled', icon:'⏸', description:'No automatic backups. Only manual “Back Up Now” runs.', color:'slate' },
              daily: { name:'Daily', icon:'🌙', description:'Creates a backup daily at the selected hour and retains the latest N backups.', color:'emerald' },
              weekly: { name:'Weekly', icon:'📅', description:'Creates a backup every seven days to reduce disk usage.', color:'indigo' },
            }
            const m = meta[f]
            const color: Record<string,string> = {
              slate:   isSelected ? 'border-slate-500 bg-slate-100 dark:bg-slate-800 ring-2 ring-slate-400/20'      : 'border-slate-200 dark:border-slate-700 hover:border-slate-400 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800',
              emerald: isSelected ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/20 ring-2 ring-emerald-500/20': 'border-slate-200 dark:border-slate-700 hover:border-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-900/30 dark:bg-emerald-900/20',
              indigo:  isSelected ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-900/20 ring-2 ring-indigo-500/20'   : 'border-slate-200 dark:border-slate-700 hover:border-indigo-300 hover:bg-indigo-50 dark:bg-indigo-900/20',
            }
            return (
              <button key={f} type="button" disabled={scheduleSaving || isSelected}
                onClick={() => saveSchedule({ ...sched, freq: f })}
                className={`text-left p-3 border rounded-lg transition disabled:cursor-default ${color[m.color]}`}>
                <div className="flex items-center justify-between mb-1">
                  <span className="text-base leading-none">{m.icon}</span>
                  {isSelected && <span className="text-[10px] uppercase tracking-wider font-semibold text-emerald-700 dark:text-emerald-300">● Active</span>}
                </div>
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{m.name}</div>
                <div className="text-[11px] text-slate-600 dark:text-slate-400 dark:text-slate-500 mt-1 leading-snug">{m.description}</div>
              </button>
            )
          })}
        </div>

        {sched.freq !== 'none' && (
          <div className="mt-4 grid grid-cols-1 sm:grid-cols-2 gap-3">
            <label className="block">
              <span className="text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500">Run time (local)</span>
              <select
                value={sched.hour}
                onChange={e => saveSchedule({ ...sched, hour: Number(e.target.value) })}
                disabled={scheduleSaving}
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm bg-white dark:bg-slate-800">
                {Array.from({length:24},(_,i)=>i).map(h =>
                  <option key={h} value={h}>{String(h).padStart(2,'0')}:00</option>
                )}
              </select>
            </label>
            <label className="block">
              <span className="text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500">Backups to retain</span>
              <input type="number" min={1} max={90} value={sched.retention}
                onChange={e => setSched(s => ({...s, retention: Math.max(1, Math.min(90, Number(e.target.value)||1))}))}
                onBlur={() => saveSchedule(sched)}
                disabled={scheduleSaving}
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
              <span className="text-[10px] text-slate-500 dark:text-slate-500 mt-0.5 block">Older automatic backups beyond this count are deleted. Manual backups are retained.</span>
            </label>
          </div>
        )}
      </div>

      {/* Remote backup destination (FTP/SFTP) */}
      <div className="mb-5 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <div className="flex flex-col gap-3 mb-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Remote Backup Destination (FTP / SFTP)</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">
              After creation, backups are uploaded to the remote server in the background for off-site protection against disk failure.
            </p>
          </div>
          {!dest.missing && dest.last_status && (
            <span className={`text-[10px] uppercase tracking-wider font-semibold px-2 py-1 rounded ${
              dest.last_status === 'successful' ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' :
              dest.last_status === 'error' ? 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300' :
              'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400 dark:text-slate-500'
            }`}>{dest.last_status === 'successful' ? '● Latest: successful' : dest.last_status === 'error' ? '✗ Latest: error' : dest.last_status}</span>
          )}
        </div>

        {!dest.missing && dest.last_upload && (
          <div className="mb-3 text-xs text-slate-500 dark:text-slate-500">
            Latest upload: <span className="font-mono">{dest.last_upload}</span>
            {dest.last_status === 'error' && dest.last_error && (
              <div className="mt-1 text-[11px] text-red-700 dark:text-red-300 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded p-2 font-mono whitespace-pre-wrap">{dest.last_error}</div>
            )}
          </div>
        )}

        <div className="grid grid-cols-1 sm:grid-cols-6 gap-3 mb-3">
          <div className="sm:col-span-2">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Protocol</label>
            <div className="flex gap-2">
              {(['sftp','ftp'] as const).map(t => {
                const isSelected = destForm.type === t
                return (
                  <button key={t} type="button"
                    onClick={() => setDestForm(f => ({...f, type: t, port: t === 'sftp' ? 22 : 21}))}
                    className={`flex-1 text-xs px-3 py-2 rounded border ${isSelected ? 'border-brand-500 bg-brand-50 dark:bg-brand-900/20 text-brand-700 dark:text-brand-300 font-semibold' : 'border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800'}`}>
                    {t === 'sftp' ? '🔒 SFTP (port 22)' : '📡 FTP (port 21)'}
                  </button>
                )
              })}
            </div>
          </div>
          <div className="sm:col-span-3">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Host</label>
            <input type="text" value={destForm.host} placeholder="backup.firma.com"
              onChange={e => setDestForm(f => ({...f, host: e.target.value}))}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
          </div>
          <div className="sm:col-span-1">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Port</label>
            <input type="number" value={destForm.port}
              onChange={e => setDestForm(f => ({...f, port: Number(e.target.value)||0}))}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Username</label>
            <input type="text" value={destForm.username} autoComplete="off"
              onChange={e => setDestForm(f => ({...f, username: e.target.value}))}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Password {!dest.missing && <span className="text-[10px] text-slate-400 dark:text-slate-500">(leave blank to keep the current password)</span>}</label>
            <input type="password" value={destForm.password} autoComplete="new-password"
              onChange={e => setDestForm(f => ({...f, password: e.target.value}))}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Remote directory</label>
            <input type="text" value={destForm.remote_dir}
              onChange={e => setDestForm(f => ({...f, remote_dir: e.target.value}))}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
          </div>
        </div>

        <div className="flex items-center justify-between flex-wrap gap-3">
          <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
            <input type="checkbox" checked={destForm.active}
              onChange={e => setDestForm(f => ({...f, active: e.target.checked}))}
              className="cursor-pointer"/>
            Active (send every backup to this destination)
          </label>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            {destTest && (
              <span className={`text-xs px-2 py-1 rounded font-medium ${destTest.ok ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300'}`}>
                {destTest.ok ? '✓ Connection successful' : '✗ ' + (destTest.error?.slice(0, 80) || 'Error')}
              </span>
            )}
            <button type="button" onClick={testDestination} disabled={destinationSaving || !destForm.host || !destForm.username}
              className="text-xs px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 disabled:opacity-50">
              {destinationSaving ? 'Testing...' : 'Test Connection'}
            </button>
            <button type="button" onClick={saveDest} disabled={destinationSaving || !destForm.host || !destForm.username}
              className="text-xs px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 rounded font-medium">
              Save
            </button>
            {!dest.missing && (
              <button type="button" onClick={destDelete} disabled={destinationSaving}
                className="text-xs px-3 py-1.5 border border-red-300 dark:border-red-700 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 rounded">
                Delete Destination
              </button>
            )}
          </div>
        </div>
      </div>

      <div className="flex flex-col gap-2 mb-4 sm:flex-row sm:items-center">
        <button onClick={create} disabled={processing} className="px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">
          {processing ? 'Backing up...' : '+ Back Up Now'}
        </button>
        <button onClick={load} className="px-3 py-2 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md">↻ Refresh</button>
        <span className="text-sm text-slate-500 dark:text-slate-500 sm:ml-auto">{backups.length} backups</span>
      </div>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      <div className={responsiveTableContainerClass}>
        {loading ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading...</div> :
         backups.length === 0 ? <div className="py-16 text-center text-sm text-slate-500 dark:text-slate-500">No backups yet</div> :
        <table className={responsiveTableClass}>
          <thead className={responsiveTableHeadClass}>
            <tr>
              <th className="text-left px-4 py-2.5">File</th>
              <th className="text-left px-4 py-2.5">Type</th>
              <th className="text-left px-4 py-2.5">Size</th>
              <th className="text-left px-4 py-2.5">Created</th>
              <th className="text-right px-4 py-2.5">Actions</th>
            </tr>
          </thead>
          <tbody className={responsiveTableBodyClass}>
            {backups.map(y => (
              <tr key={y.id} className={responsiveTableRowClass}>
                <td data-label="File" className={responsiveTableCodeCellClass}>{y.file}</td>
                <td data-label="Type" className={responsiveTableCellClass}>
                  <span className={`text-xs px-1.5 py-0.5 rounded uppercase tracking-wider font-semibold ${
                    y.type === 'scheduled' ? 'bg-sky-100 text-sky-700' : 'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400 dark:text-slate-500'
                  }`}>{y.type === 'scheduled' ? 'Scheduled' : y.type}</span>
                </td>
                <td data-label="Size" className={responsiveTableCodeCellClass}>{formatSize(y.size_b)}</td>
                <td data-label="Created" className={responsiveTableCellClass}>{y.created_at}</td>
                <td className={responsiveTableActionCellClass}>
                  <button onClick={() => download(y)} className="text-sm text-brand-600 dark:text-brand-400 hover:bg-brand-50 dark:hover:bg-brand-900/30 dark:bg-brand-900/20 px-2 py-1 rounded">Download</button>
                  <button onClick={() => setRestoreBackup(y)} className="text-sm text-amber-700 dark:text-amber-300 hover:bg-amber-50 dark:hover:bg-amber-900/30 dark:bg-amber-900/20 px-2 py-1 rounded">↺ Restore</button>
                  <button onClick={() => setBackupToDelete(y)} className="text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 px-2 py-1 rounded">Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>}
      </div>

      <ConfirmDialog
        open={!!backupToDelete}
        title="Delete backup file"
        message={`Delete backup file "${backupToDelete?.file}"?`}
        dangerous confirmText="Yes, delete"
        onConfirm={remove}
        onCancel={() => setBackupToDelete(null)}
      />

      <ConfirmDialog
        open={!!restoreBackup}
        title="Restore from backup"
        message={`Restore "${restoreBackup?.file}"?\n\nWARNING: Existing public_html files will be overwritten and MySQL tables will be recreated. This action cannot be undone.`}
        dangerous confirmText="Yes, restore"
        onConfirm={restore}
        onCancel={() => setRestoreBackup(null)}
      />
    </div>
  )
}

function formatSize(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(0)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`
}