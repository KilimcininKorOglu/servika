import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Domain = { id: number; domain_name: string; system_user: string; ipv4: string; ssl: boolean; ssl_expiry?: string }
type SSLStatus = {
  active: boolean
  source: string
  expires_at?: string
  cert_path?: string
  key_path?: string
}

export default function DomainSSLPage() {
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [status, setStatus] = useState<SSLStatus | null>(null)
  const [isProcessing, setIsProcessing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  function load() {
    if (!id) return
    api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    api.get<SSLStatus>(`/domains/${id}/ssl`).then(r => setStatus(r.data)).catch(e => setError(apiError(e)))
  }
  useEffect(load, [id])

  async function issue(type: 'self-signed' | 'letsencrypt') {
    if (type === 'letsencrypt' && !confirm('The domain must point to this server with a DNS A record before a Let\'s Encrypt certificate can be issued. Continue?')) return
    setIsProcessing(true); setError(null); setSuccess(null)
    try {
      const { data } = await api.post(`/domains/${id}/ssl/issue`, { type })
      setSuccess(`Certificate installed (${type}). Expires: ${data.expires_at}. The site now uses HTTPS.`)
      load()
    } catch (e) {
      setError(apiError(e, 'SSL installation failed'))
    } finally {
      setIsProcessing(false)
    }
  }

  async function disable() {
    if (!confirm('Remove SSL? The site will switch back to HTTP.')) return
    setIsProcessing(true); setError(null); setSuccess(null)
    try {
      await api.delete(`/domains/${id}/ssl`)
      setSuccess('SSL removed. The site now uses HTTP.')
      load()
    } catch (e) {
      setError(apiError(e, 'Failed to remove SSL'))
    } finally {
      setIsProcessing(false)
    }
  }

  return (
    <div className="px-6 py-5 max-w-[1100px]">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Domains', href: '/domains' },
        { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: 'SSL/TLS Certificates' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">SSL/TLS Certificates</h1>
      {domain && (
        <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">
          <Link to={`/subscriptions/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">{domain.domain_name}</Link>
          {' · '}
          IP: <span className="font-mono">{domain.ipv4}</span>
        </p>
      )}

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {/* Status card */}
      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-6 mb-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Current Status</h2>
          {status && (
            status.active ? (
              <span className="text-xs px-2 py-1 bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 rounded uppercase font-semibold tracking-wider flex items-center gap-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
                Protected
              </span>
            ) : (
              <span className="text-xs px-2 py-1 bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 rounded uppercase font-semibold tracking-wider flex items-center gap-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-amber-400"></span>
                Unprotected
              </span>
            )
          )}
        </div>
        {!status ? (
          <div className="text-sm text-slate-400 dark:text-slate-500">Loading…</div>
        ) : status.active ? (
          <div className="space-y-2 text-sm">
            <DetailRow label="Source" value={status.source === 'letsencrypt' ? "Let's Encrypt" : 'Self-signed'} />
            {status.expires_at && <DetailRow label="Expiry" value={new Date(status.expires_at).toLocaleDateString('en-US', { dateStyle: 'long' })} />}
            <DetailRow label="Certificate path" value={status.cert_path || '—'} mono />
            <DetailRow label="Key path" value={status.key_path || '—'} mono />
            <button
              onClick={disable}
              disabled={isProcessing}
              className="mt-4 px-4 py-2 border border-red-300 dark:border-red-700 text-red-700 dark:text-red-300 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 disabled:opacity-50 rounded-md text-sm font-medium transition"
            >
              Remove SSL (switch to HTTP)
            </button>
          </div>
        ) : (
          <div className="text-sm text-slate-600 dark:text-slate-400 dark:text-slate-500">
            There is no active SSL certificate for this domain. Install one below.
          </div>
        )}
      </div>

      {/* Action cards */}
      {status && !status.active && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
          <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-6">
            <div className="flex items-center gap-2 mb-2">
              <div className="w-9 h-9 rounded-lg bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 flex items-center justify-center">
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.7}><path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
              </div>
              <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100">Self-Signed Certificate</h3>
            </div>
            <p className="text-sm text-slate-500 dark:text-slate-500 mb-4">
              Creates a self-signed certificate on the server. Browsers display a warning, but the connection is encrypted.
              Suitable for test and development environments.
            </p>
            <ul className="text-xs text-slate-500 dark:text-slate-500 mb-4 space-y-1">
              <li>✓ No DNS dependency</li>
              <li>✓ Installs immediately</li>
              <li>✗ Browser displays a security warning</li>
            </ul>
            <button
              onClick={() => issue('self-signed')}
              disabled={isProcessing}
              className="w-full px-4 py-2.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md transition"
            >
              {isProcessing ? 'Installing…' : 'Install Self-Signed Certificate'}
            </button>
          </div>

          <div className="bg-white dark:bg-slate-800 border border-emerald-200 dark:border-emerald-800 rounded-2xl p-6">
            <div className="flex items-center gap-2 mb-2">
              <div className="w-9 h-9 rounded-lg bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 flex items-center justify-center">
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.7}><path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>
              </div>
              <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100">Let's Encrypt (Free)</h3>
            </div>
            <p className="text-sm text-slate-500 dark:text-slate-500 mb-4">
              An official Let's Encrypt certificate trusted by all browsers. It renews automatically every 90 days.
            </p>
            <ul className="text-xs text-slate-500 dark:text-slate-500 mb-4 space-y-1">
              <li>✓ Trusted by browsers</li>
              <li>✓ Automatic renewal (cron)</li>
              <li>⚠ The domain must point to this server via DNS</li>
            </ul>
            <button
              onClick={() => issue('letsencrypt')}
              disabled={isProcessing}
              className="w-full px-4 py-2.5 bg-emerald-600 hover:bg-emerald-700 disabled:bg-emerald-300 text-white text-sm font-medium rounded-md transition"
            >
              {isProcessing ? 'Installing…' : 'Get Let\'s Encrypt Certificate'}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

function DetailRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-slate-500 dark:text-slate-500">{label}</span>
      <span className={`text-slate-800 dark:text-slate-200 text-right break-all ${mono ? 'font-mono text-xs' : ''}`}>{value}</span>
    </div>
  )
}