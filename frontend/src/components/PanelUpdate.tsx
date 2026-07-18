import { useCallback, useEffect, useRef, useState } from 'react'
import { api, apiError } from '@/lib/api'

type UpdateStatus = {
  tool_available: boolean
  running: boolean
  status: string
}

type UpdateLog = {
  log: string
  running: boolean
  status: string
}

/** Displays update status, starts an update, and follows its persistent log. */
export default function PanelUpdate() {
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [log, setLog] = useState('')
  const [running, setRunning] = useState(false)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirmation, setConfirmation] = useState(false)
  const logRef = useRef<HTMLPreElement>(null)

  const loadStatus = useCallback(async () => {
    try {
      const response = await api.get<UpdateStatus>('/system/update')
      setStatus(response.data)
      setRunning(response.data.running)
    } catch {
      // The API can be temporarily unavailable while the panel restarts.
    }
  }, [])

  useEffect(() => {
    loadStatus()
  }, [loadStatus])

  useEffect(() => {
    if (!running) return
    let stopped = false
    const poll = async () => {
      try {
        const response = await api.get<UpdateLog>('/system/update/log')
        if (stopped) return
        setLog(response.data.log)
        if (!response.data.running) {
          setRunning(false)
          loadStatus()
        }
      } catch {
        // Polling continues across the expected panel restart.
      }
    }
    const interval = window.setInterval(poll, 2000)
    poll()
    return () => {
      stopped = true
      window.clearInterval(interval)
    }
  }, [running, loadStatus])

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight })
  }, [log])

  async function start() {
    setError(null)
    setStarting(true)
    setConfirmation(false)
    try {
      await api.post('/system/update/start')
      setLog('Update started...\n')
      setRunning(true)
    } catch (caughtError) {
      setError(apiError(caughtError, 'Could not start the update'))
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="mb-6 p-4 border rounded-2xl bg-emerald-50 dark:bg-emerald-900/15 border-emerald-200 dark:border-emerald-800/50">
      <div className="flex items-start gap-3">
        <div className="w-10 h-10 rounded-lg flex items-center justify-center text-xl flex-shrink-0 bg-emerald-100 dark:bg-emerald-900/40">⬆️</div>
        <div className="flex-1 min-w-0">
          <div className="flex items-baseline gap-2">
            <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">Panel Update</span>
            <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300">Server</span>
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">
            Updates the panel to the latest version from GitHub. Environment settings, databases, and sites are preserved, and an unhealthy release is rolled back automatically.
            {status && !status.tool_available && ' The update tool is missing and will be downloaded automatically on first use.'}
          </div>

          {error && <div className="mt-2 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-xs">{error}</div>}

          {running && (
            <div className="mt-2 inline-flex items-center gap-2 text-xs text-emerald-700 dark:text-emerald-300">
              <span className="w-3 h-3 rounded-full border-2 border-emerald-500 border-t-transparent animate-spin" />
              The update is running. The panel may restart briefly; keep this page open.
            </div>
          )}

          {log && (
            <pre ref={logRef} className="mt-2 text-[11px] font-mono bg-slate-900 text-slate-300 rounded-lg p-2.5 max-h-56 overflow-auto whitespace-pre-wrap leading-relaxed">{log}</pre>
          )}

          <div className="mt-3 flex items-center gap-2">
            {!confirmation ? (
              <button onClick={() => setConfirmation(true)} disabled={running || starting}
                className="text-xs px-3 py-1.5 rounded-lg bg-slate-900 dark:bg-white text-white dark:text-slate-900 hover:opacity-90 transition font-medium disabled:opacity-40 disabled:cursor-not-allowed">
                Check for updates and install
              </button>
            ) : (
              <>
                <span className="text-xs text-slate-600 dark:text-slate-300">The panel will be updated and its service restarted. Confirm?</span>
                <button onClick={start} disabled={starting}
                  className="text-xs px-3 py-1.5 rounded-lg bg-emerald-600 text-white hover:bg-emerald-700 transition font-medium disabled:opacity-40">
                  {starting ? 'Starting...' : 'Yes, update'}
                </button>
                <button onClick={() => setConfirmation(false)} className="text-xs px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition">
                  Cancel
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
