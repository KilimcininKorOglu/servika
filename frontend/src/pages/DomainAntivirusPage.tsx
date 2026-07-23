import { useEffect, useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
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

type Finding = { file: string; signature: string; engine: string; quarantined: number }
type Scan = { id: number; status: string; engine: string; scanned: number; infected: number; started_at: string; finished_at: string }
type Status = { clamav: boolean; signature_date: string; username: string; last_scan: Scan | null; findings: Finding[] }

export default function DomainAntivirusPage() {
  const { id } = useParams()
  const [d, setD] = useState<Status | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)
  const [signatureLoading, setSignatureLoading] = useState(false)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  function load() {
    if (!id) return
    api.get<Status>(`/domains/${id}/antivirus`).then(r => {
      setD(r.data)
      if (r.data.last_scan?.status === 'running') startPoll(r.data.last_scan.id)
    }).catch(e => setError(apiError(e))).finally(() => setLoading(false))
  }
  useEffect(() => { load(); return () => { if (pollRef.current) clearInterval(pollRef.current) } }, [id])

  function startPoll(sid: number) {
    setScanning(true)
    if (pollRef.current) clearInterval(pollRef.current)
    pollRef.current = setInterval(async () => {
      try {
        const { data } = await api.get<Scan & { findings: Finding[] }>(`/domains/${id}/antivirus/scan/${sid}`)
        if (data.status !== 'running') {
          if (pollRef.current) clearInterval(pollRef.current)
          setScanning(false)
          load()
        }
      } catch { if (pollRef.current) clearInterval(pollRef.current); setScanning(false) }
    }, 2500)
  }

  async function scan() {
    setError(null); setScanning(true)
    try {
      const { data } = await api.post(`/domains/${id}/antivirus/scan`, {})
      startPoll(data.scan_id)
    } catch (e) { setError(apiError(e, 'Could not start scan')); setScanning(false) }
  }

  async function quarantineFinding(b: Finding) {
    if (!confirm(`Quarantine this file?\n${b.file}\n\n(The file will be moved under ~/.quarantined and become inaccessible.)`)) return
    setError(null)
    try { await api.post(`/domains/${id}/antivirus/quarantine`, { file: b.file }); load() }
    catch (e) { setError(apiError(e, 'Could not quarantine file')) }
  }

  async function updateSignature() {
    setSignatureLoading(true); setError(null)
    try { await api.post(`/domains/${id}/antivirus/update-signature`, {}); load() }
    catch (e) { setError(apiError(e, 'Could not update signatures')) }
    finally { setSignatureLoading(false) }
  }

  if (loading) return <div className="px-4 py-4 text-slate-400 sm:px-6 sm:py-5">Loading...</div>
  if (!d) return <div className="px-4 py-4 sm:px-6 sm:py-5"><div className="text-sm text-red-600">{error || 'Not found'}</div></div>

  const activeFindings = d.findings.filter(finding => !finding.quarantined)

  return (
    <div className="px-4 py-4 sm:px-6 sm:py-5">
      <div className="max-w-4xl mx-auto">
        <Breadcrumb items={[
          { label: 'Home', href: '/' },
          { label: 'Domains', href: '/domains' },
          { label: 'Antivirus' },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Antivirus Malware Scan</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          <span className="font-mono">public_html</span> is scanned using ClamAV signatures and built-in webshell heuristics.
        </p>

        {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}

        {/* Status and actions */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="text-sm space-y-0.5">
              <div className="flex items-center gap-2">
                <span className={`w-2 h-2 rounded-full ${d.clamav ? 'bg-emerald-500' : 'bg-amber-500'}`} />
                <span className="text-slate-700 dark:text-slate-200">Engine: <span className="font-medium">{d.clamav ? 'ClamAV + Heuristics' : 'Heuristics Only'}</span></span>
              </div>
              {d.clamav && <div className="text-xs text-slate-400 ml-4">Signature database: {d.signature_date || '-'}</div>}
              {d.last_scan && <div className="text-xs text-slate-400 ml-4">
                Latest scan: {d.last_scan.finished_at || d.last_scan.started_at}. {d.last_scan.scanned} files. {d.last_scan.infected} findings
              </div>}
            </div>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              {d.clamav && <button onClick={updateSignature} disabled={signatureLoading || scanning}
                className="px-3 py-2 text-sm border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-50">
                {signatureLoading ? 'Updating...' : 'Update Signatures'}</button>}
              <button onClick={scan} disabled={scanning}
                className="px-4 py-2 text-sm font-medium bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg disabled:opacity-50">
                {scanning ? 'Scanning...' : 'Scan Now'}</button>
            </div>
          </div>
          {scanning && (
            <div className="mt-3 flex items-center gap-2 text-sm text-brand-600 dark:text-brand-400">
              <span className="inline-block w-4 h-4 border-2 border-brand-500 border-t-transparent rounded-full animate-spin" />
              Scan in progress. Large sites may take several minutes.
            </div>
          )}
        </div>

        {/* Findings */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">
            Findings {d.last_scan && <span className="text-xs font-normal text-slate-400">from the latest scan</span>}
          </h3>
          {!d.last_scan ? (
            <div className="text-center py-8 text-sm text-slate-500 dark:text-slate-400">No scans yet. Select “Scan Now” to begin.</div>
          ) : activeFindings.length === 0 && d.findings.length === 0 ? (
            <div className="text-center py-8">
              <div className="text-3xl mb-2">✅</div>
              <p className="text-sm text-emerald-600 dark:text-emerald-400 font-medium">Clean. No malware found.</p>
            </div>
          ) : (
            <div className={responsiveTableContainerClass}>
              <table className={responsiveTableClass}>
                <thead className={responsiveTableHeadClass}>
                  <tr>
                    <th className="py-2 pr-3 text-left">File</th>
                    <th className="py-2 pr-3 text-left">Signature</th>
                    <th className="py-2 pr-3 text-left">Engine</th>
                    <th className="py-2 pr-3 text-left">Status</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody className={responsiveTableBodyClass}>
                  {d.findings.map((b, i) => (
                    <tr key={i} className={responsiveTableRowClass}>
                      <td data-label="File" className={`${responsiveTableCodeCellClass} break-all`}>{b.file}</td>
                      <td data-label="Signature" className={responsiveTableCellClass}>{b.signature}</td>
                      <td data-label="Engine" className={responsiveTableCellClass}><span className="text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 text-slate-500">{b.engine}</span></td>
                      <td data-label="Status" className={responsiveTableCellClass}>
                        {b.quarantined ? <span className="text-xs text-amber-600 dark:text-amber-400">🔒 Quarantined</span>
                          : <span className="text-xs text-red-600 dark:text-red-400">⚠ Active</span>}
                      </td>
                      <td className={responsiveTableActionCellClass}>
                        {!b.quarantined && <button onClick={() => quarantineFinding(b)} className="text-xs text-red-600 dark:text-red-400 hover:underline whitespace-nowrap">Quarantine</button>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to Subscription</Link></div>
      </div>
    </div>
  )
}
