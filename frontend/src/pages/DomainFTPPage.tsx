import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Domain = { id: number; domain_name: string; system_user: string; ftp_host: string; ftp_user: string }

export default function DomainFTPPage() {
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [newPassword, setNewPassword] = useState<string | null>(null)
  const [processing, setProcessing] = useState(false)
  const [customPassword, setCustomPassword] = useState('')

  useEffect(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(e => setError(apiError(e)))
  }, [id])

  async function resetPassword(random: boolean) {
    if (!random && !customPassword) return
    setProcessing(true); setNewPassword(null); setError(null)
    try {
      const body = random ? {} : { password: customPassword }
      const { data } = await api.put(`/domains/${id}/ftp/password`, body)
      setNewPassword(data.password)
      setCustomPassword('')
    } catch (e) {
      setError(apiError(e, 'Password reset failed'))
    } finally {
      setProcessing(false)
    }
  }

  return (
    <div className="px-6 py-5 max-w-[900px]">
      <Breadcrumb items={[
        { label: 'Home', href: '/' }, { label: 'Domains', href: '/domains' },
        { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: 'FTP Account' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">FTP Account</h1>
      {domain && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5"><Link to={`/subscriptions/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">{domain.domain_name}</Link></p>}

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}

      {domain && (
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-6">
          <div className="grid grid-cols-2 gap-y-3 mb-6 text-sm">
            <span className="text-slate-500 dark:text-slate-500">Host</span><span className="font-mono text-slate-800 dark:text-slate-200">{domain.ftp_host}</span>
            <span className="text-slate-500 dark:text-slate-500">Port</span><span className="font-mono text-slate-800 dark:text-slate-200">21 (FTP) / 22 (SFTP)</span>
            <span className="text-slate-500 dark:text-slate-500">Username</span><span className="font-mono text-slate-800 dark:text-slate-200">{domain.ftp_user}</span>
            <span className="text-slate-500 dark:text-slate-500">Home directory</span><span className="font-mono text-slate-800 dark:text-slate-200 text-xs">/home/{domain.system_user}</span>
          </div>

          <div className="border-t border-slate-200 dark:border-slate-700 pt-5">
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Reset Password</h3>
            <div className="flex items-center gap-2 mb-4">
              <input
                type="text"
                value={customPassword}
                onChange={e => setCustomPassword(e.target.value)}
                placeholder="Enter a custom password or leave blank"
                className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none"
              />
              <button onClick={() => resetPassword(false)} disabled={processing || !customPassword} className="px-3 py-2 bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 disabled:opacity-50 text-sm rounded-md">Set This Password</button>
              <button onClick={() => resetPassword(true)} disabled={processing} className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">Generate Random Password</button>
            </div>

            {newPassword && (
              <div className="bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md p-4">
                <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium mb-1">✓ New password set</p>
                <p className="text-xs text-emerald-700 dark:text-emerald-300 mb-2">Save this in a secure place. You cannot view it again later.</p>
                <div className="flex items-center gap-2">
                  <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-2 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{newPassword}</code>
                  <button onClick={() => { navigator.clipboard.writeText(newPassword); }} className="px-3 py-2 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">Copy</button>
                </div>
              </div>
            )}
          </div>

          <div className="border-t border-slate-200 dark:border-slate-700 pt-5 mt-5 text-xs text-slate-500 dark:text-slate-500">
            <p><strong>Note:</strong> FTP currently uses <code className="font-mono">cleartext</code> authentication against the local database. Use SFTP on port 22 for encrypted transport.</p>
          </div>
        </div>
      )}
    </div>
  )
}