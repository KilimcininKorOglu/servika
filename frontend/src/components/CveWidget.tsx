import { useEffect, useRef, useState } from 'react'
import { api, apiError } from '@/lib/api'

// Dashboard CVE (security vulnerability) widget.
// Backend: GET /system/cve (cached dnf updateinfo summary), POST /system/cve/update
// (background dnf update --security, survives tab close), GET /system/cve/log (status).
// Card shell visually matches the <Card> on HomePage (separate component, redefined).

type CveEntry = { id: string; severity: string; package: string }
type CveSummary = {
  critical: number
  important: number
  moderate: number
  low: number
  total_cves: number
  total_advisories: number
  last_scan: string
  top_cves: CveEntry[] | null
  update_running: boolean
  reboot_required: boolean
  kernelcare: {
    installed: boolean
    active: boolean
    registered: boolean
    effective_kernel: string
    patched_cves: string[] | null
    running: boolean
  }
}

const SHIELD = 'M12 3 4.5 6v5.5c0 4.2 3.2 7.1 7.5 8.5 4.3-1.4 7.5-4.3 7.5-8.5V6L12 3Z'
const CHECK = 'M9 12.5l2 2 4.5-4.5'
const ALERT = 'M12 9v3.5m0 3h.01'

// Severity label → color/text (semantic — distinct from brand orange).
const SEV: Record<string, { label: string; dot: string; text: string }> = {
  critical:  { label: 'Critical', dot: 'bg-red-500', text: 'text-red-600 dark:text-red-400' },
  important: { label: 'Important', dot: 'bg-amber-500', text: 'text-amber-600 dark:text-amber-400' },
  moderate:  { label: 'Moderate', dot: 'bg-sky-500', text: 'text-sky-600 dark:text-sky-400' },
  low:       { label: 'Low', dot: 'bg-slate-400', text: 'text-slate-500 dark:text-slate-400' },
}

export default function CveWidget() {
  const [data, setData] = useState<CveSummary | null>(null)
  const [error, setError] = useState('')
  const [scanning, setScanning] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [kcRunning, setKcRunning] = useState(false)
  const [message, setMessage] = useState('')
  const pollRef = useRef<number | null>(null)
  const kcPollRef = useRef<number | null>(null)

  async function fetch(force: boolean) {
    try {
      const { data } = await api.get<CveSummary>(`/system/cve${force ? '?refresh=1' : ''}`, { timeout: 120_000 })
      setData(data)
      setError('')
      if (data.update_running) startPoll()
    } catch (e) {
      setError(apiError(e, 'Could not fetch CVE status'))
    }
  }

  useEffect(() => {
    fetch(false)
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current)
      if (kcPollRef.current) window.clearInterval(kcPollRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function startPoll() {
    setUpdating(true)
    if (pollRef.current) return
    pollRef.current = window.setInterval(async () => {
      try {
        const { data } = await api.get<{ running: boolean }>('/system/cve/log')
        if (!data.running) {
          if (pollRef.current) { window.clearInterval(pollRef.current); pollRef.current = null }
          setUpdating(false)
          setMessage('Security updates complete.')
          fetch(true)
        }
      } catch { /* transient — next tick retries */ }
    }, 5000)
  }

  async function rescan() {
    setScanning(true)
    setMessage('')
    await fetch(true)
    setScanning(false)
  }

  async function update() {
    if (!window.confirm(
      'Security updates (dnf --security) will be installed. ' +
      'A reboot may be required for kernel updates to take effect. Continue?',
    )) return
    setError('')
    setMessage('')
    try {
      await api.post('/system/cve/update')
      startPoll()
    } catch (e) {
      setError(apiError(e, 'Could not start update'))
    }
  }

  function startKcPoll() {
    setKcRunning(true)
    if (kcPollRef.current) return
    kcPollRef.current = window.setInterval(async () => {
      try {
        const { data } = await api.get<{ running: boolean }>('/system/kernelcare')
        if (!data.running) {
          if (kcPollRef.current) { window.clearInterval(kcPollRef.current); kcPollRef.current = null }
          setKcRunning(false)
          setMessage('Live kernel patch applied.')
          fetch(true)
        }
      } catch { /* transient — next tick retries */ }
    }, 5000)
  }

  async function livePatch() {
    setError('')
    setMessage('')
    try {
      await api.post('/system/kernelcare/patch')
      startKcPoll()
    } catch (e) {
      setError(apiError(e, 'Could not start live patch'))
    }
  }

  const statusColor = !data ? 'text-slate-400 dark:text-slate-500'
    : data.critical > 0 ? 'text-red-500'
    : data.important > 0 ? 'text-amber-500'
    : 'text-emerald-500'

  const clean = data !== null && data.total_cves === 0
  const top = data?.top_cves ?? []

  return (
    <div className="rounded-2xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900/60">
      {/* header */}
      <div className="mb-4 flex items-start justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="relative inline-flex">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round"
              className={`h-5 w-5 ${statusColor}`}>
              <path d={SHIELD} />
              <path d={clean ? CHECK : ALERT} />
            </svg>
          </span>
          <div>
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Security Advisories</h3>
            <p className="mt-0.5 text-[11px] text-slate-400 dark:text-slate-500">
              {data ? `${data.total_advisories} security advisories` : 'CVE audit (AlmaLinux)'}
            </p>
          </div>
        </div>
        <button
          type="button"
          onClick={rescan}
          disabled={scanning || updating}
          className="shrink-0 rounded-lg px-2 py-1 text-[11px] font-medium text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 disabled:opacity-50 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-200">
          {scanning ? 'Scanning…' : 'Rescan'}
        </button>
      </div>

      {/* update in progress */}
      {updating && (
        <div className="mb-3 flex items-center gap-2 rounded-xl border border-brand-200 bg-brand-50 px-3 py-2.5 text-[12px] font-medium text-brand-700 dark:border-brand-900/50 dark:bg-brand-900/15 dark:text-brand-300">
          <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-brand-400 border-t-transparent" />
          Installing security updates… (runs in background)
        </div>
      )}

      {/* KernelCare live patching in progress */}
      {kcRunning && (
        <div className="mb-3 flex items-center gap-2 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2.5 text-[12px] font-medium text-emerald-700 dark:border-emerald-800/50 dark:bg-emerald-900/15 dark:text-emerald-300">
          <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-emerald-400 border-t-transparent" />
          Applying live kernel patch… (KernelCare — no reboot required)
        </div>
      )}

      {/* KernelCare active — kernel live-patched */}
      {!kcRunning && data?.kernelcare?.active && (
        <div className="mb-3 flex items-start gap-2 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2.5 text-[11px] text-emerald-700 dark:border-emerald-800/50 dark:bg-emerald-900/15 dark:text-emerald-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" className="mt-0.5 h-4 w-4 shrink-0"><path d={SHIELD} /><path d={CHECK} /></svg>
          <span>
            <strong>Kernel is live-patched (KernelCare).</strong> Kernel vulnerabilities are closed without a reboot.
            {data.kernelcare.effective_kernel ? <> Effective kernel: <span className="font-mono">{data.kernelcare.effective_kernel}</span>.</> : null}
            {data.kernelcare.patched_cves?.length ? <> {data.kernelcare.patched_cves.length} CVEs live-patched.</> : null}
          </span>
        </div>
      )}

      {/* KernelCare installed but license not registered */}
      {!kcRunning && data?.kernelcare?.installed && !data.kernelcare.registered && (
        <div className="mb-3 flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2.5 text-[11px] text-amber-700 dark:border-amber-800/50 dark:bg-amber-900/15 dark:text-amber-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="mt-0.5 h-3.5 w-3.5 shrink-0"><path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m0 3.75h.008M10.36 3.6 2.26 17.66A1.5 1.5 0 0 0 3.56 19.9h16.88a1.5 1.5 0 0 0 1.3-2.25L13.64 3.6a1.5 1.5 0 0 0-2.6 0Z" /></svg>
          <span><strong>KernelCare is installed but the license is not registered.</strong> Register with a TuxCare license key for rebootless kernel patching.</span>
        </div>
      )}

      {/* reboot required — patched kernel installed but not yet active */}
      {!updating && data?.reboot_required && (
        <div className="mb-3 flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2.5 text-[11px] text-amber-700 dark:border-amber-800/50 dark:bg-amber-900/15 dark:text-amber-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="mt-0.5 h-3.5 w-3.5 shrink-0"><path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m0 3.75h.008M10.36 3.6 2.26 17.66A1.5 1.5 0 0 0 3.56 19.9h16.88a1.5 1.5 0 0 0 1.3-2.25L13.64 3.6a1.5 1.5 0 0 0-2.6 0Z" /></svg>
          <span>
            <strong>Reboot required.</strong> A new security-patched kernel is installed but the system is still
            running the old kernel — most of the CVEs below are kernel-related and will appear <strong>open until
            the server is rebooted</strong>. Schedule a reboot during a maintenance window.
          </span>
        </div>
      )}

      {/* body */}
      {error ? (
        <div className="flex items-start gap-2 rounded-xl border border-red-200 bg-red-50 px-3 py-2.5 text-[11px] text-red-700 dark:border-red-900/50 dark:bg-red-900/15 dark:text-red-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="mt-0.5 h-3.5 w-3.5 shrink-0"><path strokeLinecap="round" d="M12 9v3.75m0 3.75h.008M10.36 3.6 2.26 17.66A1.5 1.5 0 0 0 3.56 19.9h16.88a1.5 1.5 0 0 0 1.3-2.25L13.64 3.6a1.5 1.5 0 0 0-2.6 0Z" /></svg>
          <span>{error}</span>
        </div>
      ) : data === null ? (
        <div className="flex items-center justify-center gap-2 py-6 text-xs text-slate-400">
          <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-slate-300 border-t-transparent dark:border-slate-600 dark:border-t-transparent" />
          Scanning server…
        </div>
      ) : clean ? (
        <div className="flex flex-col items-center gap-1.5 py-5 text-center">
          <span className="flex h-10 w-10 items-center justify-center rounded-full bg-emerald-50 dark:bg-emerald-900/25">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="h-5 w-5 text-emerald-500"><path d="M20 6 9 17l-5-5" /></svg>
          </span>
          <p className="text-sm font-semibold text-slate-700 dark:text-slate-200">System up to date</p>
          <p className="text-[11px] text-slate-400 dark:text-slate-500">No known vulnerabilities</p>
        </div>
      ) : (
        <>
          {/* severity summary */}
          <div className="mb-3 grid grid-cols-3 gap-2.5">
            {(['critical', 'important', 'moderate'] as const).map((k) => (
              <div key={k} className="rounded-xl border border-slate-100 bg-slate-50 p-3 text-center dark:border-slate-800 dark:bg-slate-950/40">
                <div className={`text-2xl font-bold tabular-nums ${SEV[k].text}`}>{data[k]}</div>
                <div className="mt-0.5 flex items-center justify-center gap-1 text-[11px] text-slate-400 dark:text-slate-500">
                  <span className={`h-1.5 w-1.5 rounded-full ${SEV[k].dot}`} />{SEV[k].label}
                </div>
              </div>
            ))}
          </div>
          <p className="mb-3 text-[11px] text-slate-400 dark:text-slate-500">
            Total <strong className="text-slate-600 dark:text-slate-300">{data.total_cves}</strong> unique CVEs
            {data.last_scan ? <> · last scan {data.last_scan}</> : null}
          </p>

          {/* top CVEs */}
          {top.length > 0 && (
            <div className="mb-3 space-y-0.5">
              {top.slice(0, 4).map((c) => (
                <div key={c.id} className="-mx-2 flex items-center justify-between gap-2 rounded-lg px-2 py-1.5">
                  <span className="flex min-w-0 items-center gap-2">
                    <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${SEV[c.severity]?.dot ?? 'bg-slate-400'}`} />
                    <span className="font-mono text-[12px] text-slate-700 dark:text-slate-200">{c.id}</span>
                  </span>
                  <span className="min-w-0 truncate text-right text-[10px] text-slate-400 dark:text-slate-500" title={c.package}>{c.package}</span>
                </div>
              ))}
            </div>
          )}

          {/* action */}
          {!updating && (
            <button
              type="button"
              onClick={update}
              className="flex w-full items-center justify-center gap-2 rounded-xl bg-brand-600 px-3 py-2.5 text-[13px] font-semibold text-white shadow-sm transition-colors hover:bg-brand-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4"><path d={SHIELD} /><path d={CHECK} /></svg>
              Install security updates
            </button>
          )}
          {/* KernelCare — rebootless live kernel patch action */}
          {!kcRunning && data.kernelcare?.installed && data.kernelcare.registered && (
            <button
              type="button"
              onClick={livePatch}
              className="mt-2 flex w-full items-center justify-center gap-2 rounded-xl border border-emerald-300 bg-emerald-50 px-3 py-2.5 text-[13px] font-semibold text-emerald-700 transition-colors hover:bg-emerald-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-500 dark:border-emerald-800/60 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/30">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4"><path d={SHIELD} /><path d={CHECK} /></svg>
              Update live kernel patches (no reboot)
            </button>
          )}
          {message && <p className="mt-2 text-center text-[11px] font-medium text-emerald-600 dark:text-emerald-400">{message}</p>}
        </>
      )}
    </div>
  )
}
