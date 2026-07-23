import { useState } from 'react'
import { api, apiError } from '@/lib/api'

export default function ServerRebootButton() {
  const [confirmation, setConfirmation] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const confirmed = confirmation === 'REBOOT'

  async function reboot() {
    setMessage('')
    setError('')
    setLoading(true)
    try {
      await api.post('/system/reboot')
      setMessage('Reboot requested. The panel will be unavailable while the server restarts.')
      setConfirmation('')
    } catch (caughtError) {
      setError(apiError(caughtError, 'Could not request server reboot'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <section className="bg-white dark:bg-slate-800 border border-red-200 dark:border-red-900/60 rounded-2xl p-6 shadow-sm">
      <div className="flex items-start gap-3 mb-5">
        <div className="w-10 h-10 rounded-2xl bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 flex items-center justify-center shrink-0">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18.36 6.64a9 9 0 1 1-12.73 0"/><path d="M12 2v10"/></svg>
        </div>
        <div>
          <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Server Reboot</h2>
          <p className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">Restart the whole server only after active jobs and customer operations are safe to interrupt.</p>
        </div>
      </div>

      <div className="space-y-4">
        <label className="block">
          <span className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">Type REBOOT to confirm</span>
          <input value={confirmation} onChange={event => setConfirmation(event.target.value)} placeholder="REBOOT" className="w-full sm:max-w-xs px-3 py-2 text-sm bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-600 rounded-lg text-slate-800 dark:text-slate-100 focus:border-red-500 focus:ring-2 focus:ring-red-500/20 outline-none" />
        </label>

        {message && <div className="text-sm px-3 py-2 rounded-lg border bg-emerald-50 dark:bg-emerald-900/20 border-emerald-200 dark:border-emerald-800 text-emerald-700 dark:text-emerald-300">{message}</div>}
        {error && <div className="text-sm px-3 py-2 rounded-lg border bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800 text-red-700 dark:text-red-300">{error}</div>}

        <button type="button" onClick={reboot} disabled={!confirmed || loading} className="px-4 py-2 text-sm font-medium rounded-lg bg-red-600 hover:bg-red-700 text-white disabled:opacity-50 disabled:cursor-not-allowed">
          {loading ? 'Requesting...' : 'Reboot Server'}
        </button>
      </div>
    </section>
  )
}
