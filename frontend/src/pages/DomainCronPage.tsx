import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import Modal from '@/components/Modal'
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

type Task = {
  idx: number
  minute: string
  hour: string
  day: string
  month: string
  weekday: string
  command: string
  comment?: string
}

type TaskResponse = {
  idx: number
  minute: string
  hour: string
  day: string
  month: string
  week: string
  command: string
  comment?: string
}

type Domain = { id: number; domain_name: string; system_user: string }

type ListResponse = { system_user: string; total: number; tasks: TaskResponse[] }

const PRESETS: Array<{ label: string; selection: Omit<Task, 'idx' | 'command' | 'comment'> }> = [
  { label: 'Every minute', selection: { minute: '*', hour: '*', day: '*', month: '*', weekday: '*' } },
  { label: 'Every hour', selection: { minute: '0', hour: '*', day: '*', month: '*', weekday: '*' } },
  { label: 'Daily at 03:00', selection: { minute: '0', hour: '3', day: '*', month: '*', weekday: '*' } },
  { label: 'Monday at 09:00', selection: { minute: '0', hour: '9', day: '*', month: '*', weekday: '1' } },
  { label: 'Every 5 minutes', selection: { minute: '*/5', hour: '*', day: '*', month: '*', weekday: '*' } },
  { label: 'Every 15 minutes', selection: { minute: '*/15', hour: '*', day: '*', month: '*', weekday: '*' } },
  { label: 'First day of the month at 00:00', selection: { minute: '0', hour: '0', day: '1', month: '*', weekday: '*' } },
]

export default function DomainCronPage() {
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [modal, setModal] = useState(false)

  function load() {
    if (!id) return
    setLoading(true); setError(null)
    api.get<ListResponse>(`/domains/${id}/cron`)
      .then(r => setTasks(r.data.tasks.map(task => ({
        idx: task.idx,
        minute: task.minute,
        hour: task.hour,
        day: task.day,
        month: task.month,
        weekday: task.week,
        command: task.command,
        comment: task.comment,
      }))))
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (id) api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    load()
  }, [id])

  async function remove(task: Task) {
    if (!confirm(`Delete the task "${task.command.slice(0, 60)}..."?`)) return
    try {
      await api.delete(`/domains/${id}/cron/${task.idx}`)
      load()
    } catch (e) {
      alert(apiError(e, 'Deletion failed'))
    }
  }

  return (
    <div className="px-4 py-4 sm:px-6 sm:py-5 max-w-[1300px]">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Domains', href: '/domains' },
        { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: 'Scheduled Tasks' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Scheduled Tasks</h1>
      {domain && (
        <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">
          <Link to={`/subscriptions/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">{domain.domain_name}</Link>
          {', '}
          <span className="font-mono text-slate-600 dark:text-slate-400 dark:text-slate-500">/var/spool/cron/{domain.system_user}</span>
        </p>
      )}

      <div className="grid grid-cols-2 gap-2 mb-4 sm:flex sm:items-center">
        <button
          onClick={() => setModal(true)}
          className="inline-flex items-center gap-1.5 px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md shadow-sm transition"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          Add Task
        </button>
        <button onClick={load} className="px-3 py-2 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md transition">↻ Refresh</button>
        <span className="col-span-2 text-sm text-slate-500 dark:text-slate-500 sm:col-span-1 sm:ml-auto">{tasks.length} tasks</span>
      </div>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}

      <div className={responsiveTableContainerClass}>
        {loading ? (
          <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading...</div>
        ) : tasks.length === 0 ? (
          <div className="py-16 text-center">
            <div className="w-14 h-14 mx-auto rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center mb-3">
              <svg className="w-7 h-7 text-slate-400 dark:text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <p className="text-sm text-slate-500 dark:text-slate-500">No tasks yet. Add one above.</p>
          </div>
        ) : (
          <table className={responsiveTableClass}>
            <thead className={responsiveTableHeadClass}>
              <tr>
                <th className="text-left px-4 py-2.5">Min</th>
                <th className="text-left px-4 py-2.5">Hour</th>
                <th className="text-left px-4 py-2.5">Day</th>
                <th className="text-left px-4 py-2.5">Month</th>
                <th className="text-left px-4 py-2.5">Weekday</th>
                <th className="text-left px-4 py-2.5">Command / Description</th>
                <th className="text-right px-4 py-2.5">Action</th>
              </tr>
            </thead>
            <tbody className={responsiveTableBodyClass}>
              {tasks.map((task) => (
                <tr key={task.idx} className={responsiveTableRowClass}>
                  <td data-label="Min" className={responsiveTableCodeCellClass}>{task.minute}</td>
                  <td data-label="Hour" className={responsiveTableCodeCellClass}>{task.hour}</td>
                  <td data-label="Day" className={responsiveTableCodeCellClass}>{task.day}</td>
                  <td data-label="Month" className={responsiveTableCodeCellClass}>{task.month}</td>
                  <td data-label="Weekday" className={responsiveTableCodeCellClass}>{task.weekday}</td>
                  <td data-label="Command" className={responsiveTableCellClass}>
                    <div className="min-w-0 flex-1 text-right lg:text-left">
                      <div className="font-mono text-slate-800 dark:text-slate-200 break-all lg:truncate lg:max-w-md" title={task.command}>{task.command}</div>
                      {task.comment && <div className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">{task.comment}</div>}
                    </div>
                  </td>
                  <td className={responsiveTableActionCellClass}>
                    <button onClick={() => remove(task)} className="text-sm text-red-600 dark:text-red-400 hover:text-red-700 dark:text-red-300 px-2 py-1 rounded hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 transition">Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <CronTaskModal open={modal} onClose={() => setModal(false)} onSaved={load} domainId={Number(id)} />
    </div>
  )
}

function CronTaskModal({ open, onClose, onSaved, domainId }: {
  open: boolean; onClose: () => void; onSaved: () => void; domainId: number
}) {
  const [minute, setMinute] = useState('0')
  const [hour, setHour] = useState('3')
  const [day, setDay] = useState('*')
  const [month, setMonth] = useState('*')
  const [weekday, setWeekday] = useState('*')
  const [command, setCommand] = useState('')
  const [comment, setComment] = useState('')
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function applyPreset(preset: typeof PRESETS[number]['selection']) {
    setMinute(preset.minute)
    setHour(preset.hour)
    setDay(preset.day)
    setMonth(preset.month)
    setWeekday(preset.weekday)
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setProcessing(true); setError(null)
    try {
      await api.post(`/domains/${domainId}/cron`, { minute, hour, day, month, week: weekday, command: command.trim(), comment: comment.trim() })
      onSaved()
      setCommand(''); setComment('')
      onClose()
    } catch (e) {
      setError(apiError(e, 'Could not add item'))
    } finally {
      setProcessing(false)
    }
  }

  return (
    <Modal open={open} title="New Scheduled Task" onClose={onClose} width="lg">
      <form onSubmit={submit} className="space-y-4">
        <div>
          <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">Presets</label>
          <div className="flex flex-wrap gap-1.5">
            {PRESETS.map(p => (
              <button
                key={p.label}
                type="button"
                onClick={() => applyPreset(p.selection)}
                className="px-2 py-1 text-xs bg-slate-100 dark:bg-slate-800 hover:bg-brand-100 dark:bg-brand-900/30 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 rounded transition"
              >
                {p.label}
              </button>
            ))}
          </div>
        </div>

        <div className="grid grid-cols-5 gap-2">
          <Field label="Minute"   value={minute} onChange={setMinute} />
          <Field label="Hour"     value={hour}   onChange={setHour} />
          <Field label="Day"      value={day}    onChange={setDay} />
          <Field label="Month"       value={month}     onChange={setMonth} />
          <Field label="Weekday"    value={weekday}  onChange={setWeekday} />
        </div>
        <p className="text-xs text-slate-500 dark:text-slate-500">Cron format: <code className="font-mono">*</code> any value, <code className="font-mono">*/5</code> every five, <code className="font-mono">0,15,30</code> list, <code className="font-mono">9-17</code> range.</p>

        <div>
          <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">Command</label>
          <input
            type="text"
            value={command}
            onChange={e => setCommand(e.target.value)}
            placeholder="/usr/bin/php /home/c_user/public_html/cron.php"
            required
            className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none text-sm font-mono"
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">Description (optional)</label>
          <input
            type="text"
            value={comment}
            onChange={e => setComment(e.target.value)}
            placeholder="e.g. nightly backup script"
            className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none text-sm"
          />
        </div>

        {error && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onClose} disabled={processing} className="px-4 py-2 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 rounded-md text-sm">Cancel</button>
          <button type="submit" disabled={processing || !command.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">
            {processing ? 'Adding...' : 'Add Task'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function Field({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <div>
      <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">{label}</label>
      <input
        type="text"
        value={value}
        onChange={e => onChange(e.target.value)}
        className="w-full px-2 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none"
      />
    </div>
  )
}