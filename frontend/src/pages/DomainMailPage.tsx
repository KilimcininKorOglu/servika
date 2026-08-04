import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Domain = { id: number; domain_name: string }
type Mailbox = { id: number; local_part: string; email: string; status: string; created_at: string }
type MailStatus = { enabled: boolean; dkim_selector?: string }
type Alias = { id: number; source: string; destination: string; catch_all: boolean; status: string; created_at: string }
type SpamSettings = { enabled: boolean; greylist_score: number; add_header_score: number; reject_score: number }
type SpamResponse = { settings: SpamSettings; rspamd: boolean }
type Autoresponder = { mailbox_id: number; email: string; enabled: boolean; subject: string; body: string; interval_days: number }
type MailFilter = {
  id: number; mailbox_id: number; email: string; name: string; match_field: 'from' | 'to' | 'subject'
  match_value: string; action_type: 'move' | 'redirect' | 'discard'; action_value: string; priority: number; enabled: boolean
}
type SendLimits = { mailbox_id: number; email: string; hour_limit: number; day_limit: number; sent_hour: number; sent_day: number; spam_suspended_at?: string }

export default function DomainMailPage() {
  const { t } = useTranslation('DomainMailPage')
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
  const [spam, setSpam] = useState<SpamSettings>({ enabled: true, greylist_score: 4, add_header_score: 6, reject_score: 15 })
  const [rspamd, setRspamd] = useState(false)
  const [isSavingSpam, setIsSavingSpam] = useState(false)
  const [autoresponder, setAutoresponder] = useState<Autoresponder>({ mailbox_id: 0, email: '', enabled: true, subject: 'Automatic reply', body: '', interval_days: 7 })
  const [isSavingAutoresponder, setIsSavingAutoresponder] = useState(false)
  const [filters, setFilters] = useState<MailFilter[]>([])
  const [filter, setFilter] = useState<Omit<MailFilter, 'id' | 'email'>>({
    mailbox_id: 0, name: '', match_field: 'subject', match_value: '', action_type: 'move', action_value: 'Junk', priority: 100, enabled: true,
  })
  const [isSavingFilter, setIsSavingFilter] = useState(false)
  const [limits, setLimits] = useState<SendLimits>({ mailbox_id: 0, email: '', hour_limit: 100, day_limit: 500, sent_hour: 0, sent_day: 0 })
  const [isSavingLimits, setIsSavingLimits] = useState(false)

  function loadMail() {
    if (!id) return
    setLoading(true)
    Promise.all([
      api.get<MailStatus>(`/domains/${id}/mail/status`),
      api.get<Mailbox[]>(`/domains/${id}/mail`),
      api.get<Alias[]>(`/domains/${id}/mail/aliases`),
      api.get<SpamResponse>(`/domains/${id}/mail/spam`).catch(() => ({ data: { settings: spam, rspamd: false } as SpamResponse })),
      api.get<MailFilter[]>(`/domains/${id}/mail/filters`).catch(() => ({ data: [] as MailFilter[] })),
    ])
      .then(([statusResponse, mailboxesResponse, aliasesResponse, spamResponse, filtersResponse]) => {
        setStatus(statusResponse.data)
        setMailboxes(mailboxesResponse.data || [])
        setAliases(aliasesResponse.data || [])
        setSpam(spamResponse.data.settings)
        setRspamd(spamResponse.data.rspamd)
        setFilters(filtersResponse.data || [])
        const boxes = mailboxesResponse.data || []
        if (!filter.mailbox_id && boxes.length) setFilter(current => ({ ...current, mailbox_id: boxes[0].id }))
        if (!autoresponder.mailbox_id && boxes.length) loadAutoresponder(boxes[0].id)
        if (!limits.mailbox_id && boxes.length) loadSendLimits(boxes[0].id)
      })
      .catch(cause => setError(apiError(cause)))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`)
      .then(response => setDomain(response.data))
      .catch(cause => setError(apiError(cause, t('errors.loadDomainFailed'))))
    loadMail()
  }, [id])

  async function enableMail() {
    setIsSaving(true)
    setError(null)
    setSuccess(null)
    try {
      await api.post(`/domains/${id}/mail/enable`)
      setSuccess(t('messages.enabled'))
      loadMail()
    } catch (cause) {
      setError(apiError(cause, t('errors.enableFailed')))
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
      setError(apiError(cause, t('errors.createMailboxFailed')))
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
      setSuccess(t('messages.forwarderAdded'))
      loadMail()
    } catch (cause) {
      setError(apiError(cause, t('errors.addForwarderFailed')))
    } finally {
      setIsSavingAlias(false)
    }
  }

  async function removeMailbox(mailbox: Mailbox) {
    if (!confirm(t('confirm.deleteMailbox', { email: mailbox.email }))) return
    setError(null)
    setSuccess(null)
    try {
      await api.delete(`/domains/${id}/mail/${mailbox.id}`)
      loadMail()
    } catch (cause) {
      setError(apiError(cause, t('errors.deleteMailboxFailed')))
    }
  }

  async function removeAlias(alias: Alias) {
    if (!confirm(t('confirm.deleteForwarder', { source: alias.source }))) return
    setError(null)
    setSuccess(null)
    try {
      await api.delete(`/domains/${id}/mail/aliases/${alias.id}`)
      loadMail()
    } catch (cause) {
      setError(apiError(cause, t('errors.deleteForwarderFailed')))
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
      setError(apiError(cause, t('errors.resetPasswordFailed')))
    }
  }

  async function toggleAliasStatus(alias: Alias) {
    setError(null)
    setSuccess(null)
    try {
      await api.post(`/domains/${id}/mail/aliases/${alias.id}/status`, { status: alias.status === 'active' ? 'suspended' : 'active' })
      loadMail()
    } catch (cause) {
      setError(apiError(cause, t('errors.updateForwarderFailed')))
    }
  }

  async function toggleMailboxStatus(mailbox: Mailbox) {
    setError(null)
    setSuccess(null)
    try {
      await api.post(`/domains/${id}/mail/${mailbox.id}/status`, { status: mailbox.status === 'active' ? 'suspended' : 'active' })
      loadMail()
    } catch (cause) {
      setError(apiError(cause, t('errors.updateMailboxFailed')))
    }
  }

  async function saveSpam(event: React.FormEvent) {
    event.preventDefault()
    setIsSavingSpam(true)
    setError(null)
    setSuccess(null)
    try {
      const response = await api.put<{ settings: SpamSettings }>(`/domains/${id}/mail/spam`, spam)
      setSpam(response.data.settings)
      setRspamd(true)
      setSuccess(t('messages.spamApplied'))
    } catch (cause) {
      setError(apiError(cause, t('errors.applySpamFailed')))
    } finally {
      setIsSavingSpam(false)
    }
  }

  async function loadAutoresponder(mailboxID: number) {
    if (!mailboxID) return
    try {
      const response = await api.get<Autoresponder>(`/domains/${id}/mail/${mailboxID}/autoresponder`)
      setAutoresponder(response.data)
    } catch (cause) {
      setError(apiError(cause, t('errors.readAutoresponderFailed')))
    }
  }

  async function saveAutoresponder(event: React.FormEvent) {
    event.preventDefault()
    setIsSavingAutoresponder(true)
    setError(null)
    setSuccess(null)
    try {
      await api.put(`/domains/${id}/mail/${autoresponder.mailbox_id}/autoresponder`, autoresponder)
      setSuccess(t('messages.autoresponderSaved'))
    } catch (cause) {
      setError(apiError(cause, t('errors.saveAutoresponderFailed')))
    } finally {
      setIsSavingAutoresponder(false)
    }
  }

  async function deleteAutoresponder() {
    setIsSavingAutoresponder(true)
    setError(null)
    try {
      await api.delete(`/domains/${id}/mail/${autoresponder.mailbox_id}/autoresponder`)
      setAutoresponder(current => ({ ...current, enabled: false, body: '' }))
      setSuccess(t('messages.autoresponderRemoved'))
    } catch (cause) {
      setError(apiError(cause))
    } finally {
      setIsSavingAutoresponder(false)
    }
  }

  async function addFilter(event: React.FormEvent) {
    event.preventDefault()
    setIsSavingFilter(true)
    setError(null)
    setSuccess(null)
    try {
      await api.post(`/domains/${id}/mail/filters`, filter)
      setFilter(current => ({ ...current, name: '', match_value: '' }))
      setSuccess(t('messages.filterAdded'))
      loadMail()
    } catch (cause) {
      setError(apiError(cause, t('errors.addFilterFailed')))
    } finally {
      setIsSavingFilter(false)
    }
  }

  async function deleteFilter(item: MailFilter) {
    if (!confirm(t('confirm.deleteFilter', { name: item.name }))) return
    setError(null)
    try {
      await api.delete(`/domains/${id}/mail/filters/${item.id}`)
      loadMail()
    } catch (cause) {
      setError(apiError(cause))
    }
  }

  async function loadSendLimits(mailboxID: number) {
    if (!mailboxID) return
    try {
      const response = await api.get<SendLimits>(`/domains/${id}/mail/${mailboxID}/send-limits`)
      setLimits(response.data)
    } catch (cause) {
      setError(apiError(cause, t('errors.readSendLimitsFailed')))
    }
  }

  async function saveSendLimits(event: React.FormEvent) {
    event.preventDefault()
    setIsSavingLimits(true)
    setError(null)
    setSuccess(null)
    try {
      await api.put(`/domains/${id}/mail/${limits.mailbox_id}/send-limits`, limits)
      setSuccess(t('messages.sendLimitsSaved'))
      loadSendLimits(limits.mailbox_id)
    } catch (cause) {
      setError(apiError(cause, t('errors.saveSendLimitsFailed')))
    } finally {
      setIsSavingLimits(false)
    }
  }

  return (
    <div className="px-6 py-5">
      <div>
        <Breadcrumb items={[
          { label: t('breadcrumb.home'), href: '/' },
          { label: t('breadcrumb.domains'), href: '/domains' },
          { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
          { label: t('breadcrumb.email') },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          {t('subtitle')}
        </p>

        {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
        {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

        {generatedPassword && (
          <div className="mb-3 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg p-4">
            <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium mb-1">{t('generatedPassword.title', { email: generatedPassword.email })}</p>
            <p className="text-xs text-emerald-700 dark:text-emerald-300 mb-2">{t('generatedPassword.saveNote')}</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-2 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{generatedPassword.password}</code>
              <button type="button" onClick={() => navigator.clipboard.writeText(generatedPassword.password)} className="px-3 py-2 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">{t('generatedPassword.copy')}</button>
            </div>
          </div>
        )}

        {loading ? (
          <div className="text-sm text-slate-400">{t('loading')}</div>
        ) : !status?.enabled ? (
          <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-6 text-center">
            <div className="text-3xl mb-2">📧</div>
            <p className="text-sm text-slate-600 dark:text-slate-300 mb-1">{t('enable.notEnabled')}</p>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">{t('enable.info')}</p>
            <button type="button" onClick={enableMail} disabled={isSaving}
              className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
              {isSaving ? t('enable.enabling') : t('enable.button')}
            </button>
          </div>
        ) : (
          <>
            <form onSubmit={addMailbox} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('mailboxAdd.title')}</h3>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <input value={localPart} onChange={event => setLocalPart(event.target.value)} required placeholder={t('mailboxAdd.localPlaceholder')}
                  className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.domain_name}</span>
                <input value={password} onChange={event => setPassword(event.target.value)} type="password" placeholder={t('mailboxAdd.passwordPlaceholder')}
                  className="sm:w-60 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <button disabled={isSaving || !localPart} className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                  {isSaving ? t('mailboxAdd.adding') : t('mailboxAdd.add')}
                </button>
              </div>
            </form>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('mailboxes.title')}</h3>
              {mailboxes.length === 0 ? (
                <div className="text-center py-8">
                  <p className="text-sm text-slate-500 dark:text-slate-400">{t('mailboxes.empty')}</p>
                </div>
              ) : (
                <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                  {mailboxes.map(mailbox => (
                    <li key={mailbox.id} className="flex items-center justify-between py-2.5">
                      <div>
                        <span className="text-sm font-mono text-slate-800 dark:text-slate-200">{mailbox.email}</span>
                        {mailbox.status !== 'active' && (
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">{t('mailboxes.suspended')}</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button type="button" onClick={() => toggleMailboxStatus(mailbox)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">
                          {mailbox.status === 'active' ? t('mailboxes.suspend') : t('mailboxes.activate')}
                        </button>
                        <button type="button" onClick={() => resetPassword(mailbox)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">{t('mailboxes.resetPassword')}</button>
                        <button type="button" onClick={() => removeMailbox(mailbox)} className="text-xs text-red-600 dark:text-red-400 hover:underline">{t('mailboxes.delete')}</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('forwarders.title')}</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
                {t('forwarders.description')}
              </p>
              <form onSubmit={addAlias} className="mb-4 space-y-2">
                <div className="flex items-center gap-2">
                  {aliasCatchAll ? (
                    <span className="flex-1 px-3 py-2 border border-dashed border-slate-300 dark:border-slate-600 rounded-lg text-sm text-slate-500 dark:text-slate-400 font-mono">*@{domain?.domain_name}</span>
                  ) : (
                    <>
                      <input value={aliasLocalPart} onChange={event => setAliasLocalPart(event.target.value)} required={!aliasCatchAll} placeholder={t('forwarders.sourcePlaceholder')}
                        className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                      <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.domain_name}</span>
                    </>
                  )}
                </div>
                <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300">
                  <input type="checkbox" checked={aliasCatchAll} onChange={event => setAliasCatchAll(event.target.checked)} />
                  {t('forwarders.catchAll')}
                </label>
                <div className="flex items-center gap-2">
                  <input value={aliasDestination} onChange={event => setAliasDestination(event.target.value)} required placeholder={t('forwarders.destinationPlaceholder')}
                    className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                  <button disabled={isSavingAlias || !aliasDestination || (!aliasCatchAll && !aliasLocalPart)}
                    className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                    {isSavingAlias ? t('forwarders.adding') : t('forwarders.add')}
                  </button>
                </div>
              </form>

              {aliases.length === 0 ? (
                <div className="text-center py-6">
                  <p className="text-sm text-slate-500 dark:text-slate-400">{t('forwarders.empty')}</p>
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
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">{t('forwarders.suspended')}</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button type="button" onClick={() => toggleAliasStatus(alias)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">
                          {alias.status === 'active' ? t('forwarders.suspend') : t('forwarders.activate')}
                        </button>
                        <button type="button" onClick={() => removeAlias(alias)} className="text-xs text-red-600 dark:text-red-400 hover:underline">{t('forwarders.delete')}</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <form onSubmit={saveSpam} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('spam.title')}</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
                {t('spam.description')}
                {!rspamd && <span className="text-amber-600 dark:text-amber-400">{t('spam.notInstalled')}</span>}
              </p>
              <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-200 mb-3">
                <input type="checkbox" checked={spam.enabled} onChange={event => setSpam({ ...spam, enabled: event.target.checked })} />
                {t('spam.enable')}
              </label>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                {([['greylist_score', t('spam.greylist')], ['add_header_score', t('spam.addHeader')], ['reject_score', t('spam.reject')]] as const).map(([key, label]) => (
                  <label key={key} className="text-xs text-slate-600 dark:text-slate-300">
                    {label}
                    <input type="number" step="0.5" min="0" max="50" value={spam[key]}
                      onChange={event => setSpam({ ...spam, [key]: Number(event.target.value) })}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm outline-none" />
                  </label>
                ))}
              </div>
              <button disabled={isSavingSpam} className="mt-3 px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                {isSavingSpam ? t('spam.applying') : t('spam.apply')}
              </button>
            </form>

            {mailboxes.length > 0 && (
            <form onSubmit={saveAutoresponder} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('autoresponder.title')}</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">{t('autoresponder.description')}</p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
                <label className="text-xs text-slate-600 dark:text-slate-300">{t('autoresponder.mailbox')}
                  <select value={autoresponder.mailbox_id} onChange={event => loadAutoresponder(Number(event.target.value))}
                    className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm">
                    {mailboxes.map(mailbox => <option key={mailbox.id} value={mailbox.id}>{mailbox.email}</option>)}
                  </select>
                </label>
                <label className="text-xs text-slate-600 dark:text-slate-300">{t('autoresponder.interval')}
                  <input type="number" min="1" max="30" value={autoresponder.interval_days}
                    onChange={event => setAutoresponder({ ...autoresponder, interval_days: Number(event.target.value) })}
                    className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
                </label>
              </div>
              <input value={autoresponder.subject} onChange={event => setAutoresponder({ ...autoresponder, subject: event.target.value })}
                placeholder={t('autoresponder.subjectPlaceholder')} className="w-full mb-2 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
              <textarea value={autoresponder.body} onChange={event => setAutoresponder({ ...autoresponder, body: event.target.value })}
                placeholder={t('autoresponder.bodyPlaceholder')} rows={3} className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
              <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-200 my-3">
                <input type="checkbox" checked={autoresponder.enabled} onChange={event => setAutoresponder({ ...autoresponder, enabled: event.target.checked })} />
                {t('autoresponder.enable')}
              </label>
              <div className="flex gap-2">
                <button disabled={isSavingAutoresponder} className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                  {isSavingAutoresponder ? t('autoresponder.saving') : t('autoresponder.save')}
                </button>
                <button type="button" onClick={deleteAutoresponder} disabled={isSavingAutoresponder} className="px-3 py-2 text-sm text-red-600 dark:text-red-400 hover:underline">{t('autoresponder.remove')}</button>
              </div>
            </form>
            )}

            {mailboxes.length > 0 && (
            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('filters.title')}</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">{t('filters.description')}</p>
              <form onSubmit={addFilter} className="grid grid-cols-1 sm:grid-cols-2 gap-2 mb-4">
                <select value={filter.mailbox_id} onChange={event => setFilter({ ...filter, mailbox_id: Number(event.target.value) })}
                  className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm">
                  {mailboxes.map(mailbox => <option key={mailbox.id} value={mailbox.id}>{mailbox.email}</option>)}
                </select>
                <input value={filter.name} onChange={event => setFilter({ ...filter, name: event.target.value })} required placeholder={t('filters.namePlaceholder')}
                  className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
                <select value={filter.match_field} onChange={event => setFilter({ ...filter, match_field: event.target.value as MailFilter['match_field'] })}
                  className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm">
                  <option value="subject">{t('filters.subjectContains')}</option><option value="from">{t('filters.fromContains')}</option><option value="to">{t('filters.toContains')}</option>
                </select>
                <input value={filter.match_value} onChange={event => setFilter({ ...filter, match_value: event.target.value })} required placeholder={t('filters.matchedTextPlaceholder')}
                  className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
                <select value={filter.action_type} onChange={event => setFilter({ ...filter, action_type: event.target.value as MailFilter['action_type'] })}
                  className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm">
                  <option value="move">{t('filters.moveToFolder')}</option><option value="redirect">{t('filters.redirectTo')}</option><option value="discard">{t('filters.discard')}</option>
                </select>
                {filter.action_type !== 'discard' &&
                  <input value={filter.action_value} onChange={event => setFilter({ ...filter, action_value: event.target.value })} required
                    placeholder={filter.action_type === 'move' ? t('filters.folderPlaceholder') : t('filters.targetPlaceholder')}
                    className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono" />}
                <button disabled={isSavingFilter} className="sm:col-span-2 px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                  {isSavingFilter ? t('filters.adding') : t('filters.add')}
                </button>
              </form>
              {filters.length === 0 ? <p className="text-sm text-slate-500 dark:text-slate-400 text-center py-4">{t('filters.empty')}</p> : (
                <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                  {filters.map(item => (
                    <li key={item.id} className="flex items-center justify-between py-2.5 text-sm">
                      <div>
                        <span className="font-mono text-xs text-slate-500">{item.email}</span>{' '}
                        <span className="text-slate-800 dark:text-slate-200">{item.name}</span>
                        <div className="text-xs text-slate-500">{item.match_field} ∋ “{item.match_value}” → {item.action_type} {item.action_value}</div>
                      </div>
                      <button type="button" onClick={() => deleteFilter(item)} className="text-xs text-red-600 dark:text-red-400 hover:underline">{t('filters.delete')}</button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            )}

            {mailboxes.length > 0 && (
            <form onSubmit={saveSendLimits} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('sendLimits.title')}</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
                {t('sendLimits.description')}
              </p>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <label className="text-xs text-slate-600 dark:text-slate-300">{t('sendLimits.mailbox')}
                  <select value={limits.mailbox_id} onChange={event => loadSendLimits(Number(event.target.value))}
                    className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm">
                    {mailboxes.map(mailbox => <option key={mailbox.id} value={mailbox.id}>{mailbox.email}</option>)}
                  </select>
                </label>
                <label className="text-xs text-slate-600 dark:text-slate-300">{t('sendLimits.hourly', { count: limits.sent_hour })}
                  <input type="number" min="0" max="100000" value={limits.hour_limit}
                    onChange={event => setLimits({ ...limits, hour_limit: Number(event.target.value) })}
                    className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
                </label>
                <label className="text-xs text-slate-600 dark:text-slate-300">{t('sendLimits.daily', { count: limits.sent_day })}
                  <input type="number" min="0" max="100000" value={limits.day_limit}
                    onChange={event => setLimits({ ...limits, day_limit: Number(event.target.value) })}
                    className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
                </label>
              </div>
              {limits.spam_suspended_at &&
                <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">{t('sendLimits.suspendedNote', { time: limits.spam_suspended_at })}</p>}
              <button disabled={isSavingLimits} className="mt-3 px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                {isSavingLimits ? t('sendLimits.saving') : t('sendLimits.save')}
              </button>
            </form>
            )}
          </>
        )}

        <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">{t('back')}</Link></div>
      </div>
    </div>
  )
}
