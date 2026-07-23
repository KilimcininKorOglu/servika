import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'

type Status = { running: boolean; status: string }
type LogResponse = { log: string; running: boolean; status: string }

export default function ServerOptimizeCard() {
  const [log, setLog] = useState('')
  const [running, setRunning] = useState(false)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirmed, setConfirmed] = useState(false)
  const logRef = useRef<HTMLPreElement>(null)

  const loadStatus = useCallback(async () => {
    try {
      const r = await api.get<Status>('/system/optimize')
      setRunning(r.data.running)
    } catch {
      // Ignore transient failures so the card stays usable.
    }
  }, [])

  useEffect(() => { loadStatus() }, [loadStatus])

  useEffect(() => {
    if (!running) return
    let stopped = false
    const tick = async () => {
      try {
        const r = await api.get<LogResponse>('/system/optimize/log')
        if (stopped) return
        setLog(r.data.log)
        if (!r.data.running) { setRunning(false); loadStatus() }
      } catch {
        // Keep polling through transient connection failures.
      }
    }
    const id = window.setInterval(tick, 2000)
    tick()
    return () => { stopped = true; window.clearInterval(id) }
  }, [running, loadStatus])

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight })
  }, [log])

  async function start() {
    setError(null); setStarting(true); setConfirmed(false)
    try {
      await api.post('/system/optimize/start')
      setLog('Optimization started...\n')
      setRunning(true)
    } catch (e: any) {
      setError(e?.response?.data?.error || e?.message || 'Failed to start optimization')
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-6">
      <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-1">Server Optimization</h3>
      <p className="text-xs text-slate-500 dark:text-slate-500 mb-3">
        Updates system packages (dnf/yum) and applies MariaDB, nginx, and PHP performance tuning.
        The process runs in the background and may take a long time. You can close the page
        while it continues.
      </p>

      {error && <div className="mt-2 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-xs">{error}</div>}

      {running && (
        <div className="mt-2 inline-flex items-center gap-2 text-xs text-sky-700 dark:text-sky-300">
          <span className="w-3 h-3 rounded-full border-2 border-sky-500 border-t-transparent animate-spin" />
          Optimization is running. Package updates may take a while; you can close the page while it continues in the background.
        </div>
      )}

      {log && (
        <pre ref={logRef} className="mt-2 text-[11px] font-mono bg-slate-900 text-slate-300 rounded-lg p-2.5 max-h-56 overflow-auto whitespace-pre-wrap leading-relaxed">{log}</pre>
      )}

      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
        {!confirmed ? (
          <button onClick={() => setConfirmed(true)} disabled={running || starting}
            className="text-xs px-3 py-1.5 rounded-lg bg-slate-900 dark:bg-white text-white dark:text-slate-900 hover:opacity-90 transition font-medium disabled:opacity-40 disabled:cursor-not-allowed">
            Update packages and optimize
          </button>
        ) : (
          <span className="flex flex-col gap-2 text-xs sm:flex-row sm:items-center">
            <span className="text-amber-600 dark:text-amber-400">This may briefly affect services. Continue?</span>
            <button onClick={start} disabled={starting}
              className="px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white text-xs font-medium transition disabled:opacity-40">
              {starting ? 'Starting...' : 'Yes, start'}
            </button>
            <button onClick={() => setConfirmed(false)}
              className="px-3 py-1.5 rounded-lg bg-slate-200 dark:bg-slate-700 hover:bg-slate-300 dark:hover:bg-slate-600 text-xs transition">
              Cancel
            </button>
          </span>
        )}
      </div>
    </div>
  )
}
