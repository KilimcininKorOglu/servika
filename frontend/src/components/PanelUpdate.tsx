import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
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

type VersionStatus = {
  enabled: boolean
  current: string
  latest: string
  update_available: boolean
  announcement: string
  critical: boolean
  release_date: string
  last_check?: string
  error: string
}

function formatCheckTime(value: string | undefined, notCheckedLabel: string) {
  if (!value) return notCheckedLabel
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

/** Displays update status, starts an update, and follows its persistent log. */
export default function PanelUpdate() {
  const { t } = useTranslation('PanelUpdate')
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [version, setVersion] = useState<VersionStatus | null>(null)
  const [log, setLog] = useState('')
  const [running, setRunning] = useState(false)
  const [starting, setStarting] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
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

  const loadVersion = useCallback(async () => {
    try {
      const response = await api.get<VersionStatus>('/system/version-check')
      setVersion(response.data)
    } catch {
      // Version checks are informational and must not block updates.
    }
  }, [])

  useEffect(() => {
    loadStatus()
    loadVersion()
  }, [loadStatus, loadVersion])

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
          loadVersion()
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
  }, [running, loadStatus, loadVersion])

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight })
  }, [log])

  async function refreshVersion() {
    setError(null)
    setRefreshing(true)
    try {
      const response = await api.post<VersionStatus>('/system/version-check/refresh')
      setVersion(response.data)
    } catch (caughtError) {
      setError(apiError(caughtError, t('errors.refresh')))
    } finally {
      setRefreshing(false)
    }
  }

  async function start() {
    setError(null)
    setStarting(true)
    setConfirmation(false)
    try {
      await api.post('/system/update/start')
      setLog(t('logStarted'))
      setRunning(true)
    } catch (caughtError) {
      setError(apiError(caughtError, t('errors.start')))
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="mb-6 p-4 border rounded-2xl bg-emerald-50 dark:bg-emerald-900/15 border-emerald-200 dark:border-emerald-800/50">
      <div className="flex items-start gap-3">
        <div className="w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0 bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300">
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('title')}</span>
            <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300">{t('badges.server')}</span>
            {version?.critical && <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300">{t('badges.critical')}</span>}
            {version?.update_available && !version.critical && <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300">{t('badges.updateAvailable')}</span>}
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">
            {t('description')}
            {status && !status.tool_available && t('toolMissing')}
          </div>

          {version && (
            <div className="mt-3 rounded-xl border border-slate-200 dark:border-slate-700 bg-white/70 dark:bg-slate-950/20 p-3 text-xs text-slate-600 dark:text-slate-300">
              <div className="grid gap-2 sm:grid-cols-3">
                <div><span className="block text-slate-400 dark:text-slate-500">{t('labels.current')}</span><span className="font-medium text-slate-900 dark:text-slate-100">{version.current || t('unknown')}</span></div>
                <div><span className="block text-slate-400 dark:text-slate-500">{t('labels.latest')}</span><span className="font-medium text-slate-900 dark:text-slate-100">{version.latest || t('unknown')}</span></div>
                <div><span className="block text-slate-400 dark:text-slate-500">{t('labels.lastCheck')}</span><span className="font-medium text-slate-900 dark:text-slate-100">{formatCheckTime(version.last_check, t('notCheckedYet'))}</span></div>
              </div>
              {version.announcement && (
                <div className={`mt-3 rounded-lg px-3 py-2 ${version.critical ? 'bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300' : 'bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300'}`}>
                  {version.announcement}
                  {version.release_date && <span className="ml-1 opacity-80">({version.release_date})</span>}
                </div>
              )}
              {version.error && <div className="mt-2 text-slate-500 dark:text-slate-400">{t('lastCheckFailed', { error: version.error })}</div>}
              {!version.enabled && <div className="mt-2 text-slate-500 dark:text-slate-400">{t('checksDisabled')}</div>}
            </div>
          )}

          {error && <div className="mt-2 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-xs">{error}</div>}

          {running && (
            <div className="mt-2 inline-flex items-center gap-2 text-xs text-emerald-700 dark:text-emerald-300">
              <span className="w-3 h-3 rounded-full border-2 border-emerald-500 border-t-transparent animate-spin" />
              {t('running')}
            </div>
          )}

          {log && (
            <pre ref={logRef} className="mt-2 text-[11px] font-mono bg-slate-900 text-slate-300 rounded-lg p-2.5 max-h-56 overflow-auto whitespace-pre-wrap leading-relaxed">{log}</pre>
          )}

          <div className="mt-3 flex flex-col sm:flex-row sm:items-center gap-2">
            {!confirmation ? (
              <>
                <button onClick={() => setConfirmation(true)} disabled={running || starting}
                  className="text-xs px-3 py-1.5 rounded-lg bg-slate-900 dark:bg-white text-white dark:text-slate-900 hover:opacity-90 transition font-medium disabled:opacity-40 disabled:cursor-not-allowed">
                  {t('checkAndInstall')}
                </button>
                <button onClick={refreshVersion} disabled={refreshing || running}
                  className="text-xs px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition disabled:opacity-40 disabled:cursor-not-allowed">
                  {refreshing ? t('checking') : t('refreshStatus')}
                </button>
              </>
            ) : (
              <>
                <span className="text-xs text-slate-600 dark:text-slate-300">{t('confirmPrompt')}</span>
                <button onClick={start} disabled={starting}
                  className="text-xs px-3 py-1.5 rounded-lg bg-emerald-600 text-white hover:bg-emerald-700 transition font-medium disabled:opacity-40">
                  {starting ? t('starting') : t('confirmYes')}
                </button>
                <button onClick={() => setConfirmation(false)} className="text-xs px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition">
                  {t('cancel')}
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
