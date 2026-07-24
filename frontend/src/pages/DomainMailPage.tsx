import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Domain = { id: number; domain_name: string }
type Mailbox = { id: number; local_part: string; email: string; status: string; created_at: string }
type MailStatus = { enabled: boolean; dkim_selector?: string }
type Alias = { id: number; source: string; destination: string; catch_all: boolean; status: string; created_at: string }

export default function DomainMailPage() {
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [status, setStatus] = useState<MailStatus | null>(null)
  const [mailboxes, setMailboxes] = useState<Mailbox[]>([])
  const [aliases, setAliases] = useState<Alias[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [localPart, setLocalPart] = useState('')
  const [password, setPassword] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const [generatedPassword, setGeneratedPassword] = useState<{ email: string; password: string } | null>(null)
  const [aliasLocalPart, setAliasLocalPart] = useState('')
  const [aliasDestination, setAliasDestination] = useState('')
  const [aliasCatchAll, setAliasCatchAll] = useState(false)
  const [isSavingAlias, setIsSavingAlias] = useState(false)

  function loadMail() {
    if (!id) return
    setLoading(true)
    Promise.all([
      api.get<MailStatus>(`/domains/${id}/mail/status`),
      api.get<Mailbox[]>(`/domains/${id}/mail`),
      api.get<Alias[]>(`/domains/${id}/mail/aliases`),
    ])
      .then(([statusResponse, mailboxesResponse, aliasesResponse]) => {
        setStatus(statusResponse.data)
        setMailboxes(mailboxesResponse.data || [])
        setAliases(aliasesResponse.data || [])
      })
      .catch(cause => setError(apiError(cause)))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`)
      .then(response => setDomain(response.data))
      .catch(cause => setError(apiError(cause, 'Could not load the domain')))
    loadMail()
  }, [id])

  async function enableMail() {
    setIsSaving(true)
    setError(null)
    setSuccess(null)
    try {
      await api.post(`/domains/${id}/mail/enable`)
      setSuccess('Email hosting has been enabled for this domain. MX, SPF, DKIM, and DMARC records were added to DNS when possible.')
      loadMail()
    } catch (cause) {
      setError(apiError(cause, 'Could not enable email hosting'))
    } finally {
      setIsSaving(false)
    }
  }

  async function addMailbox(event: React.FormEvent) {
    event.preventDefault()
    setError(null)
    setSuccess(null)
    setGeneratedPassword(null)
    setIsSaving(true)
    try {
      const response = await api.post<{ email: string; password: string }>(`/domains/${id}/mail`, { local_part: localPart, password })
      setGeneratedPassword({ email: response.data.email, password: response.data.password })
      setLocalPart('')
      setPassword('')
      loadMail()
    } catch (cause) {
      setError(apiError(cause, 'Could not create the mailbox'))
    } finally {
      setIsSaving(false)
    }
  }

  async function addAlias(event: React.FormEvent) {
    event.preventDefault()
    setError(null)
    setSuccess(null)
    setIsSavingAlias(true)
    try {
      await api.post(`/domains/${id}/mail/aliases`, {
        local_part: aliasCatchAll ? '' : aliasLocalPart,
        destination: aliasDestination,
      })
      setAliasLocalPart('')
      setAliasDestination('')
      setAliasCatchAll(false)
      setSuccess('Mail forwarder has been added.')
      loadMail()
    } catch (cause) {
      setError(apiError(cause, 'Could not add the mail forwarder'))
    } finally {
      setIsSavingAlias(false)
    }
  }

  async function removeMailbox(mailbox: Mailbox) {
    if (!confirm(`Delete the mailbox "${mailbox.email}"? The Maildir will remain on disk, and only the account row will be removed.`)) return
    setError(null)
    setSuccess(null)
    try {
      await api.delete(`/domains/${id}/mail/${mailbox.id}`)
      loadMail()
    } catch (cause) {
      setError(apiError(cause, 'Could not delete the mailbox'))
    }
  }

  async function removeAlias(alias: Alias) {
    if (!confirm(`Delete the forwarder "${alias.source}"?`)) return
    setError(null)
    setSuccess(null)
    try {
      await api.delete(`/domains/${id}/mail/aliases/${alias.id}`)
      loadMail()
    } catch (cause) {
      setError(apiError(cause, 'Could not delete the mail forwarder'))
    }
  }

  async function resetPassword(mailbox: Mailbox) {
    setError(null)
    setSuccess(null)
    setGeneratedPassword(null)
    try {
      const response = await api.put<{ password: string }>(`/domains/${id}/mail/${mailbox.id}/password`, {})
      setGeneratedPassword({ email: mailbox.email, password: response.data.password })
    } catch (cause) {
      setError(apiError(cause, 'Could not reset the password'))
    }
  }

  async function toggleAliasStatus(alias: Alias) {
    setError(null)
    setSuccess(null)
    try {
      await api.post(`/domains/${id}/mail/aliases/${alias.id}/status`, { status: alias.status === 'active' ? 'suspended' : 'active' })
      loadMail()
    } catch (cause) {
      setError(apiError(cause, 'Could not update the mail forwarder'))
    }
  }

  return (
    <div className="px-6 py-5">
      <div className="max-w-3xl mx-auto">
        <Breadcrumb items={[
          { label: 'Home', href: '/' },
          { label: 'Domains', href: '/domains' },
          { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
          { label: 'Email' },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Email Accounts</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          Manage native Postfix and Dovecot mailboxes. Use SMTP port 587 with STARTTLS for authenticated sending.
        </p>

        {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
        {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

        {generatedPassword && (
          <div className="mb-3 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg p-4">
            <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium mb-1">Password for {generatedPassword.email}</p>
            <p className="text-xs text-emerald-700 dark:text-emerald-300 mb-2">Save it now. It will not be shown again.</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-2 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{generatedPassword.password}</code>
              <button type="button" onClick={() => navigator.clipboard.writeText(generatedPassword.password)} className="px-3 py-2 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">Copy</button>
            </div>
          </div>
        )}

        {loading ? (
          <div className="text-sm text-slate-400">Loading…</div>
        ) : !status?.enabled ? (
          <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-6 text-center">
            <div className="text-3xl mb-2">📧</div>
            <p className="text-sm text-slate-600 dark:text-slate-300 mb-1">Email hosting is not enabled for this domain yet.</p>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">Enabling it adds MX, SPF, DKIM, and DMARC DNS records when possible.</p>
            <button type="button" onClick={enableMail} disabled={isSaving}
              className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
              {isSaving ? 'Enabling…' : 'Enable Email'}
            </button>
          </div>
        ) : (
          <>
            <form onSubmit={addMailbox} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Add mailbox</h3>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <input value={localPart} onChange={event => setLocalPart(event.target.value)} required placeholder="info"
                  className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.domain_name}</span>
                <input value={password} onChange={event => setPassword(event.target.value)} type="password" placeholder="password, generated if empty"
                  className="sm:w-60 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <button disabled={isSaving || !localPart} className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                  {isSaving ? 'Adding…' : 'Add'}
                </button>
              </div>
            </form>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Mailboxes</h3>
              {mailboxes.length === 0 ? (
                <div className="text-center py-8">
                  <p className="text-sm text-slate-500 dark:text-slate-400">No mailboxes yet.</p>
                </div>
              ) : (
                <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                  {mailboxes.map(mailbox => (
                    <li key={mailbox.id} className="flex items-center justify-between py-2.5">
                      <div>
                        <span className="text-sm font-mono text-slate-800 dark:text-slate-200">{mailbox.email}</span>
                        {mailbox.status !== 'active' && (
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">suspended</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button type="button" onClick={() => resetPassword(mailbox)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">Reset password</button>
                        <button type="button" onClick={() => removeMailbox(mailbox)} className="text-xs text-red-600 dark:text-red-400 hover:underline">Delete</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">Forwarders and Catch-All</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
                Forward incoming mail to one or more destination addresses without creating a mailbox. Catch-all forwards mail for undefined addresses on this domain.
              </p>
              <form onSubmit={addAlias} className="mb-4 space-y-2">
                <div className="flex items-center gap-2">
                  {aliasCatchAll ? (
                    <span className="flex-1 px-3 py-2 border border-dashed border-slate-300 dark:border-slate-600 rounded-lg text-sm text-slate-500 dark:text-slate-400 font-mono">*@{domain?.domain_name}</span>
                  ) : (
                    <>
                      <input value={aliasLocalPart} onChange={event => setAliasLocalPart(event.target.value)} required={!aliasCatchAll} placeholder="support"
                        className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                      <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.domain_name}</span>
                    </>
                  )}
                </div>
                <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300">
                  <input type="checkbox" checked={aliasCatchAll} onChange={event => setAliasCatchAll(event.target.checked)} />
                  Forward all undefined addresses on this domain
                </label>
                <div className="flex items-center gap-2">
                  <input value={aliasDestination} onChange={event => setAliasDestination(event.target.value)} required placeholder="target1@example.com, target2@example.com"
                    className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                  <button disabled={isSavingAlias || !aliasDestination || (!aliasCatchAll && !aliasLocalPart)}
                    className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                    {isSavingAlias ? 'Adding…' : 'Add'}
                  </button>
                </div>
              </form>

              {aliases.length === 0 ? (
                <div className="text-center py-6">
                  <p className="text-sm text-slate-500 dark:text-slate-400">No forwarders yet.</p>
                </div>
              ) : (
                <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                  {aliases.map(alias => (
                    <li key={alias.id} className="flex items-center justify-between py-2.5">
                      <div>
                        <span className="text-sm font-mono text-slate-800 dark:text-slate-200">
                          {alias.catch_all ? `*@${domain?.domain_name}` : alias.source}
                        </span>
                        <span className="mx-1.5 text-slate-400">→</span>
                        <span className="text-sm font-mono text-slate-600 dark:text-slate-400">{alias.destination}</span>
                        {alias.status !== 'active' && (
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">suspended</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button type="button" onClick={() => toggleAliasStatus(alias)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">
                          {alias.status === 'active' ? 'Suspend' : 'Activate'}
                        </button>
                        <button type="button" onClick={() => removeAlias(alias)} className="text-xs text-red-600 dark:text-red-400 hover:underline">Delete</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </>
        )}

        <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to subscription</Link></div>
      </div>
    </div>
  )
}
