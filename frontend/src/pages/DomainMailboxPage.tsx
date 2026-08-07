import { useCallback, useEffect, useState } from 'react'
import { useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { api, apiError } from '@/lib/api'
import { useDialog } from '@/lib/dialog'
import { useCopyOrOffer } from '@/lib/useCopyOrOffer'
import Breadcrumb from '@/components/Breadcrumb'

type Domain = { id: number; domain_name: string }
type Mailbox = {
  id: number; local_part: string; email: string; status: string
  quota_bytes: number; used_bytes: number; usage_checked_at?: string | null
}
type Connection = {
  hostname?: string; imap_port?: number; submission_port?: number
  security?: string; username: string; reason?: string
}
type Autoresponder = {
  mailbox_id: number; email: string; enabled: boolean
  subject: string; body: string; interval_days: number
}
type Forwarding = { enabled: boolean; destinations: string[]; keep_copy: boolean }
type ImportFormats = {
  maildir_tar_supported: boolean; mbox_supported: boolean
  pst_supported: boolean; max_bytes: number
}

type Tab = 'general' | 'autoresponder' | 'forwarding' | 'transfer'

// A quota of zero is not a limit of zero; it is the absence of one, which the
// userdb query turns into a NULL quota_rule. Drawing it as a full bar would tell
// a customer their unlimited mailbox is out of space.
function quotaPercent(used: number, quota: number): number | null {
  if (quota <= 0) return null
  return Math.min(100, Math.round((used / quota) * 100))
}

function formatBytes(value: number): string {
  if (value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const power = Math.min(units.length - 1, Math.floor(Math.log(value) / Math.log(1024)))
  const scaled = value / Math.pow(1024, power)
  return `${power === 0 ? scaled : scaled.toFixed(1)} ${units[power]}`
}

export default function DomainMailboxPage() {
  const { t } = useTranslation('DomainMailboxPage')
  const { notify, confirm } = useDialog()
  const copy = useCopyOrOffer()
  const { id, mid } = useParams()

  const [domain, setDomain] = useState<Domain | null>(null)
  const [mailbox, setMailbox] = useState<Mailbox | null>(null)
  const [connection, setConnection] = useState<Connection | null>(null)
  const [formats, setFormats] = useState<ImportFormats | null>(null)
  const [tab, setTab] = useState<Tab>('general')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  const [isRecalculating, setIsRecalculating] = useState(false)
  const [autoresponder, setAutoresponder] = useState<Autoresponder | null>(null)
  const [isSavingAutoresponder, setIsSavingAutoresponder] = useState(false)
  const [forwarding, setForwarding] = useState<Forwarding>({ enabled: false, destinations: [], keep_copy: true })
  const [destinationText, setDestinationText] = useState('')
  const [isSavingForwarding, setIsSavingForwarding] = useState(false)
  const [uploadFile, setUploadFile] = useState<File | null>(null)
  const [isImporting, setIsImporting] = useState(false)

  // Split so the mount effect never writes state synchronously: fetchMailbox
  // settles only through promise callbacks, and load() adds the spinner for the
  // refreshes that follow a write.
  const fetchMailbox = useCallback(() => {
    if (!id || !mid) return
    Promise.all([
      api.get<Domain>(`/domains/${id}`),
      api.get<Mailbox[]>(`/domains/${id}/mail`),
      api.get<Connection>(`/domains/${id}/mail/${mid}/connection`),
      api.get<Autoresponder>(`/domains/${id}/mail/${mid}/autoresponder`),
      api.get<Forwarding>(`/domains/${id}/mail/${mid}/forwarding`),
      api.get<ImportFormats>(`/domains/${id}/mail/${mid}/import`),
    ])
      .then(([domainResponse, mailboxesResponse, connectionResponse, autoresponderResponse, forwardingResponse, formatsResponse]) => {
        setDomain(domainResponse.data)
        const found = (mailboxesResponse.data || []).find(box => String(box.id) === mid) || null
        setMailbox(found)
        setConnection(connectionResponse.data)
        setAutoresponder(autoresponderResponse.data)
        setForwarding(forwardingResponse.data)
        setDestinationText((forwardingResponse.data.destinations || []).join('\n'))
        setFormats(formatsResponse.data)
        setError(null)
      })
      .catch(cause => setError(apiError(cause, t('errors.loadFailed'))))
      .finally(() => setLoading(false))
  }, [id, mid, t])

  const load = useCallback(() => { setLoading(true); fetchMailbox() }, [fetchMailbox])

  useEffect(() => { fetchMailbox() }, [fetchMailbox])

  async function recalculateQuota() {
    setIsRecalculating(true)
    try {
      const response = await api.post<{ used_bytes: number; quota_bytes: number; dovecot_recount: boolean }>(
        `/domains/${id}/mail/${mid}/quota-recalc`)
      setMailbox(current => current ? { ...current, used_bytes: response.data.used_bytes, quota_bytes: response.data.quota_bytes } : current)
      // The panel's own number is right either way; saying which half did not
      // land is the difference between a customer who waits and one who knows.
      setSuccess(response.data.dovecot_recount ? t('quota.recalculated') : t('quota.recalculatedPartly'))
    } catch (cause) {
      await notify({ message: apiError(cause, t('errors.recalculateFailed')), tone: 'error' })
    } finally {
      setIsRecalculating(false)
    }
  }

  async function saveAutoresponder(event: React.FormEvent) {
    event.preventDefault()
    if (!autoresponder) return
    setIsSavingAutoresponder(true)
    try {
      await api.put(`/domains/${id}/mail/${mid}/autoresponder`, autoresponder)
      setSuccess(t('autoresponder.saved'))
    } catch (cause) {
      await notify({ message: apiError(cause, t('errors.saveAutoresponderFailed')), tone: 'error' })
    } finally {
      setIsSavingAutoresponder(false)
    }
  }

  async function saveForwarding(event: React.FormEvent) {
    event.preventDefault()
    const destinations = destinationText.split(/[\n,]/).map(value => value.trim()).filter(Boolean)
    if (forwarding.enabled && destinations.length === 0) {
      await notify({ message: t('forwarding.needsADestination'), tone: 'error' })
      return
    }
    if (forwarding.enabled && !forwarding.keep_copy) {
      // Mail that arrives is forwarded and then dropped, so nothing is left here
      // to go back to. That is worth one question before it is switched on.
      if (!(await confirm({ message: t('forwarding.confirmNoCopy'), dangerous: true }))) return
    }
    setIsSavingForwarding(true)
    try {
      const response = await api.put<Forwarding & { applied: boolean; reason?: string }>(
        `/domains/${id}/mail/${mid}/forwarding`,
        { enabled: forwarding.enabled, destinations, keep_copy: forwarding.keep_copy })
      setForwarding({ enabled: response.data.enabled, destinations: response.data.destinations || [], keep_copy: response.data.keep_copy })
      setDestinationText((response.data.destinations || []).join('\n'))
      setSuccess(response.data.applied ? t('forwarding.saved') : t('forwarding.savedNotApplied'))
    } catch (cause) {
      await notify({ message: apiError(cause, t('errors.saveForwardingFailed')), tone: 'error' })
    } finally {
      setIsSavingForwarding(false)
    }
  }

  async function runImport(event: React.FormEvent) {
    event.preventDefault()
    if (!uploadFile) return
    setIsImporting(true)
    try {
      const body = new FormData()
      body.append('file', uploadFile)
      const response = await api.post<{ messages: number; folders: number }>(
        `/domains/${id}/mail/${mid}/import`, body)
      setSuccess(t('transfer.imported', { messages: response.data.messages, folders: response.data.folders }))
      setUploadFile(null)
      load()
    } catch (cause) {
      await notify({ message: apiError(cause, t('errors.importFailed')), tone: 'error' })
    } finally {
      setIsImporting(false)
    }
  }

  // A download carries the session cookie, so it goes through the same origin
  // with credentials rather than through a link the browser sends anonymously.
  async function exportMailbox() {
    try {
      const response = await fetch(`/api/v1/domains/${id}/mail/${mid}/export`, { credentials: 'include' })
      if (!response.ok) throw new Error(String(response.status))
      const blob = await response.blob()
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `${mailbox?.local_part || 'mailbox'}-maildir.tar.gz`
      anchor.click()
      URL.revokeObjectURL(url)
    } catch (cause) {
      await notify({ message: apiError(cause, t('errors.exportFailed')), tone: 'error' })
    }
  }

  const percent = mailbox ? quotaPercent(mailbox.used_bytes, mailbox.quota_bytes) : null
  const tabs: { key: Tab; label: string }[] = [
    { key: 'general', label: t('tabs.general') },
    { key: 'autoresponder', label: t('tabs.autoresponder') },
    { key: 'forwarding', label: t('tabs.forwarding') },
    { key: 'transfer', label: t('tabs.transfer') },
  ]

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <Breadcrumb items={[
        { label: t('breadcrumb.home'), href: '/' },
        { label: t('breadcrumb.domains'), href: '/domains' },
        { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: t('breadcrumb.email'), href: `/subscriptions/${id}/mail` },
        { label: mailbox?.email || '...' },
      ]} />

      <h1 className="mt-4 text-xl font-semibold text-slate-900 dark:text-slate-100">{mailbox?.email || t('title')}</h1>
      <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{t('subtitle')}</p>

      {error && <div className="mt-4 rounded-lg bg-red-50 dark:bg-red-900/20 px-4 py-3 text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mt-4 rounded-lg bg-emerald-50 dark:bg-emerald-900/20 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {loading ? (
        <p className="mt-6 text-sm text-slate-500 dark:text-slate-400">{t('loading')}</p>
      ) : !mailbox ? (
        <p className="mt-6 text-sm text-slate-500 dark:text-slate-400">{t('notFound')}</p>
      ) : (
        <>
          <div className="mt-5 inline-flex gap-1 rounded-xl bg-slate-100 p-1 dark:bg-slate-800">
            {tabs.map(entry => (
              <button key={entry.key} type="button" onClick={() => { setTab(entry.key); setSuccess(null) }}
                className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${tab === entry.key ? 'bg-white text-slate-900 shadow-sm dark:bg-slate-700 dark:text-slate-100' : 'text-slate-500 hover:text-slate-800 dark:text-slate-400 dark:hover:text-slate-200'}`}>
                {entry.label}
              </button>
            ))}
          </div>

          {tab === 'general' && (
            <div className="mt-5 grid grid-cols-1 lg:grid-cols-2 gap-5 items-start">
              <div className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800">
                <h3 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">{t('quota.title')}</h3>
                <p className="text-sm text-slate-700 dark:text-slate-300">
                  {percent === null
                    ? t('quota.unlimited', { used: formatBytes(mailbox.used_bytes) })
                    : t('quota.used', { used: formatBytes(mailbox.used_bytes), quota: formatBytes(mailbox.quota_bytes) })}
                </p>
                {percent !== null && (
                  <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-slate-100 dark:bg-slate-700">
                    <div className={`h-full ${percent >= 90 ? 'bg-red-500' : 'bg-brand-500'}`} style={{ width: `${percent}%` }} />
                  </div>
                )}
                <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">
                  {mailbox.usage_checked_at ? t('quota.checkedAt', { when: new Date(mailbox.usage_checked_at).toLocaleString() }) : t('quota.neverChecked')}
                </p>
                <button type="button" onClick={recalculateQuota} disabled={isRecalculating}
                  className="mt-3 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-50 hover:bg-slate-800 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100">
                  {isRecalculating ? t('quota.recalculating') : t('quota.recalculate')}
                </button>
              </div>

              <div className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800">
                <h3 className="mb-1 text-sm font-semibold text-slate-900 dark:text-slate-100">{t('connection.title')}</h3>
                <p className="mb-3 text-xs text-slate-500 dark:text-slate-400">{t('connection.description')}</p>
                {connection?.reason === 'no_mail_hostname' ? (
                  <p className="text-sm text-amber-700 dark:text-amber-300">{t('connection.hostnamePending')}</p>
                ) : (
                  <dl className="space-y-2 text-sm">
                    {[
                      { label: t('connection.server'), value: connection?.hostname || '' },
                      { label: t('connection.username'), value: connection?.username || '' },
                      { label: t('connection.imap'), value: `${connection?.imap_port} (${connection?.security})` },
                      { label: t('connection.submission'), value: `${connection?.submission_port} (${connection?.security})` },
                    ].map(row => (
                      <div key={row.label} className="flex items-center justify-between gap-3">
                        <dt className="text-slate-500 dark:text-slate-400">{row.label}</dt>
                        <dd className="flex items-center gap-2">
                          <span className="font-mono text-slate-800 dark:text-slate-200">{row.value}</span>
                          <button type="button" onClick={() => copy(row.value)}
                            className="text-xs text-brand-600 hover:underline dark:text-brand-400">{t('connection.copy')}</button>
                        </dd>
                      </div>
                    ))}
                  </dl>
                )}
              </div>
            </div>
          )}

          {tab === 'autoresponder' && autoresponder && (
            <form onSubmit={saveAutoresponder} className="mt-5 max-w-xl space-y-3 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800">
              <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={autoresponder.enabled}
                  onChange={event => setAutoresponder({ ...autoresponder, enabled: event.target.checked })} />
                {t('autoresponder.enabled')}
              </label>
              <input value={autoresponder.subject} onChange={event => setAutoresponder({ ...autoresponder, subject: event.target.value })}
                placeholder={t('autoresponder.subject')}
                className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm outline-none focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 dark:border-slate-600 dark:bg-slate-900" />
              <textarea value={autoresponder.body} onChange={event => setAutoresponder({ ...autoresponder, body: event.target.value })}
                rows={6} placeholder={t('autoresponder.body')}
                className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm outline-none focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 dark:border-slate-600 dark:bg-slate-900" />
              <label className="block text-sm text-slate-700 dark:text-slate-300">
                {t('autoresponder.intervalDays')}
                <input type="number" min={1} value={autoresponder.interval_days}
                  onChange={event => setAutoresponder({ ...autoresponder, interval_days: Number(event.target.value) })}
                  className="ml-2 w-20 rounded-lg border border-slate-300 px-2 py-1 text-sm dark:border-slate-600 dark:bg-slate-900" />
              </label>
              <p className="text-xs text-slate-500 dark:text-slate-400">{t('autoresponder.intervalHint')}</p>
              <button disabled={isSavingAutoresponder}
                className="rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-50 hover:bg-slate-800 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100">
                {isSavingAutoresponder ? t('saving') : t('save')}
              </button>
            </form>
          )}

          {tab === 'forwarding' && (
            <form onSubmit={saveForwarding} className="mt-5 max-w-xl space-y-3 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800">
              <p className="text-xs text-slate-500 dark:text-slate-400">{t('forwarding.description')}</p>
              <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={forwarding.enabled}
                  onChange={event => setForwarding({ ...forwarding, enabled: event.target.checked })} />
                {t('forwarding.enabled')}
              </label>
              <textarea value={destinationText} onChange={event => setDestinationText(event.target.value)}
                rows={4} placeholder={t('forwarding.destinationsPlaceholder')} disabled={!forwarding.enabled}
                className="w-full rounded-lg border border-slate-300 px-3 py-2 font-mono text-sm outline-none focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 disabled:opacity-50 dark:border-slate-600 dark:bg-slate-900" />
              <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={forwarding.keep_copy} disabled={!forwarding.enabled}
                  onChange={event => setForwarding({ ...forwarding, keep_copy: event.target.checked })} />
                {t('forwarding.keepCopy')}
              </label>
              <button disabled={isSavingForwarding}
                className="rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-50 hover:bg-slate-800 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100">
                {isSavingForwarding ? t('saving') : t('save')}
              </button>
            </form>
          )}

          {tab === 'transfer' && (
            <div className="mt-5 grid grid-cols-1 lg:grid-cols-2 gap-5 items-start">
              <div className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800">
                <h3 className="mb-1 text-sm font-semibold text-slate-900 dark:text-slate-100">{t('transfer.exportTitle')}</h3>
                <p className="mb-3 text-xs text-slate-500 dark:text-slate-400">{t('transfer.exportDescription')}</p>
                <button type="button" onClick={exportMailbox}
                  className="rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-800 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100">
                  {t('transfer.export')}
                </button>
              </div>

              <form onSubmit={runImport} className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700 dark:bg-slate-800">
                <h3 className="mb-1 text-sm font-semibold text-slate-900 dark:text-slate-100">{t('transfer.importTitle')}</h3>
                <p className="mb-3 text-xs text-slate-500 dark:text-slate-400">
                  {formats?.pst_supported ? t('transfer.importFormatsWithPst') : t('transfer.importFormats')}
                </p>
                <input type="file" onChange={event => setUploadFile(event.target.files?.[0] || null)}
                  className="w-full text-sm text-slate-700 dark:text-slate-300" />
                <button disabled={isImporting || !uploadFile}
                  className="mt-3 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-50 hover:bg-slate-800 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100">
                  {isImporting ? t('transfer.importing') : t('transfer.import')}
                </button>
              </form>
            </div>
          )}
        </>
      )}
    </div>
  )
}
