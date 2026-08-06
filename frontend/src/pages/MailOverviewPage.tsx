// Server-wide mail overview: mailbox and alias counts per domain, so total
// mail footprint is visible without opening each domain. Also exposes the live
// Postfix queue with hold, release, requeue, delete, and flush controls.
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Link } from 'react-router'
import OverviewList, { type Column, type Badge } from '@/components/OverviewList'
import { api, apiError } from '@/lib/api'
import { useReportError } from '@/lib/errors'

type ServerSettings = {
  max_message_size_mb: number
  domain_send_limit_hour: number
  client_send_limit_hour: number
  dnsbl_zones: string
}

type Row = {
  domain_id: number
  domain_name: string
  mail_enabled: boolean
  mail_status: string
  mailbox_count: number
  alias_count: number
  suspended_mailbox_count: number
}
type QueueRecipient = { address: string; delay_reason?: string }
type QueueMessage = {
  queue_id: string; queue_name: string; arrival_time: number; message_size: number
  sender: string; recipients: QueueRecipient[]
}

function buildColumns(t: TFunction): Column<Row>[] {
  return [
    {
      title: t('columns.domain'),
      cell: (s) => (
        <Link to={`/subscriptions/${s.domain_id}/mail`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
          {s.domain_name}
        </Link>
      ),
    },
    {
      title: t('columns.mail'),
      cell: (s) => (s.mail_enabled
        ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{s.mail_status}</span>
        : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">{t('mailNone')}</span>),
    },
    { title: t('columns.mailboxes'), cell: (s) => s.mailbox_count },
    { title: t('columns.aliases'), cell: (s) => s.alias_count },
    {
      title: t('columns.suspended'),
      cell: (s) => (s.suspended_mailbox_count > 0
        ? <span className="px-2 py-0.5 rounded text-xs bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">{s.suspended_mailbox_count}</span>
        : <span className="text-xs text-slate-400">0</span>),
    },
  ]
}

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export default function MailOverviewPage() {
  const { t } = useTranslation('MailOverviewPage')
  const report = useReportError()
  const columns = buildColumns(t)
  const [queue, setQueue] = useState<QueueMessage[]>([])
  const [queueError, setQueueError] = useState<string | null>(null)
  const [queueLoading, setQueueLoading] = useState(true)
  const [queueBusy, setQueueBusy] = useState('')
  const [settings, setSettings] = useState<ServerSettings | null>(null)
  const [settingsError, setSettingsError] = useState<string | null>(null)
  const [settingsSaved, setSettingsSaved] = useState(false)
  const [settingsSaving, setSettingsSaving] = useState(false)

  // Split so the mount effect never writes state synchronously: fetchQueue
  // settles only through promise callbacks, and loadQueue() adds the spinner for
  // the refresh button and the refreshes that follow a queue action.
  const fetchQueue = useCallback(() => {
    api.get<{ messages: QueueMessage[] }>('/admin/mail/queue')
      .then(response => setQueue(response.data.messages || []))
      .catch(cause => setQueueError(apiError(cause, t('errors.readFailed'))))
      .finally(() => setQueueLoading(false))
  }, [t])

  const loadQueue = useCallback(() => {
    setQueueLoading(true)
    setQueueError(null)
    fetchQueue()
  }, [fetchQueue])

  useEffect(() => { fetchQueue() }, [fetchQueue])

  useEffect(() => {
    api.get<ServerSettings>('/admin/mail/settings')
      .then(response => setSettings(response.data))
      .catch(report('mailServerSettings'))
  }, [report])

  async function saveSettings(event: React.FormEvent) {
    event.preventDefault()
    if (!settings) return
    setSettingsSaving(true)
    setSettingsError(null)
    setSettingsSaved(false)
    try {
      // The response is the normalised form of what was sent (the zone list is
      // lower-cased and re-joined), so the screen shows what Postfix is running
      // rather than what was typed.
      const response = await api.put<ServerSettings>('/admin/mail/settings', settings)
      setSettings(response.data)
      setSettingsSaved(true)
    } catch (cause) {
      setSettingsError(apiError(cause, t('serverSettings.saveFailed')))
    } finally {
      setSettingsSaving(false)
    }
  }

  async function queueAction(action: 'flush' | 'delete' | 'hold' | 'release' | 'requeue', queueID = '') {
    if (action === 'delete' && !confirm(t('confirmDelete', { queueID }))) return
    setQueueBusy(action + queueID)
    setQueueError(null)
    try {
      await api.post('/admin/mail/queue', { action, queue_id: queueID })
      loadQueue()
    } catch (cause) {
      setQueueError(apiError(cause, t('errors.actionFailed')))
    } finally {
      setQueueBusy('')
    }
  }

  return (
    <>
      <OverviewList<Row>
        title={t('overview.title')}
        icon="✉️"
        description={t('overview.description')}
        endpoint="/overview/mail"
        columns={columns}
        searchField={(s) => s.domain_name}
        rowKey={(s) => s.domain_id}
        emptyMessage={t('overview.empty')}
        summary={(list): Badge[] => {
          const boxes = list.reduce((n, s) => n + s.mailbox_count, 0)
          const active = list.filter((s) => s.mail_enabled).length
          return [
            { label: t('summary.mailDomains'), value: active },
            { label: t('summary.totalMailboxes'), value: boxes },
          ]
        }}
      />

      <div className="w-full px-6 pb-8">
        <form onSubmit={saveSettings} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('serverSettings.title')}</h2>
          <p className="text-xs text-slate-500 mt-1 mb-4">{t('serverSettings.subtitle')}</p>
          {settings === null ? (
            <p className="text-xs text-slate-400 dark:text-slate-500">{t('serverSettings.loading')}</p>
          ) : (
            <>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <label className="text-xs text-slate-600 dark:text-slate-300">{t('serverSettings.maxMessageSize')}
                  <input type="number" min="0" max="512" value={settings.max_message_size_mb}
                    onChange={event => setSettings({ ...settings, max_message_size_mb: Number(event.target.value) })}
                    className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
                </label>
                <label className="text-xs text-slate-600 dark:text-slate-300">{t('serverSettings.domainLimit')}
                  <input type="number" min="0" value={settings.domain_send_limit_hour}
                    onChange={event => setSettings({ ...settings, domain_send_limit_hour: Number(event.target.value) })}
                    className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
                </label>
                <label className="text-xs text-slate-600 dark:text-slate-300">{t('serverSettings.clientLimit')}
                  <input type="number" min="0" value={settings.client_send_limit_hour}
                    onChange={event => setSettings({ ...settings, client_send_limit_hour: Number(event.target.value) })}
                    className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm" />
                </label>
              </div>
              <label className="block mt-3 text-xs text-slate-600 dark:text-slate-300">{t('serverSettings.dnsbl')}
                <input value={settings.dnsbl_zones} placeholder="zen.spamhaus.org bl.spamcop.net"
                  onChange={event => setSettings({ ...settings, dnsbl_zones: event.target.value })}
                  className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono" />
              </label>
              <p className="mt-1 text-[11px] text-slate-400 dark:text-slate-500">{t('serverSettings.zeroNote')}</p>
              {settingsError && <p className="mt-2 text-xs text-red-600 dark:text-red-400">{settingsError}</p>}
              {settingsSaved && <p className="mt-2 text-xs text-emerald-600 dark:text-emerald-400">{t('serverSettings.saved')}</p>}
              <button disabled={settingsSaving} className="mt-3 px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                {settingsSaving ? t('serverSettings.saving') : t('serverSettings.save')}
              </button>
            </>
          )}
        </form>
      </div>
      <div className="w-full px-6 pb-8">
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
          <div className="p-5 flex items-center justify-between gap-3 border-b border-slate-200 dark:border-slate-700">
            <div>
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('queue.title')}</h2>
              <p className="text-xs text-slate-500 mt-1">{t('queue.subtitle')}</p>
            </div>
            <div className="flex gap-2">
              <button onClick={loadQueue} disabled={queueLoading}
                className="px-3 py-1.5 text-xs border border-slate-300 dark:border-slate-600 rounded">{t('queue.refresh')}</button>
              <button onClick={() => queueAction('flush')} disabled={!!queueBusy}
                className="px-3 py-1.5 text-xs bg-slate-900 text-white dark:bg-white dark:text-slate-900 rounded disabled:opacity-50">
                {t('queue.retry')}
              </button>
            </div>
          </div>
          {queueError && <div className="m-4 p-3 text-sm text-red-700 bg-red-50 dark:bg-red-900/20 dark:text-red-300 rounded">{queueError}</div>}
          {queueLoading ? <div className="p-8 text-center text-sm text-slate-400">{t('queue.loading')}</div> :
           queue.length === 0 ? <div className="p-8 text-center text-sm text-emerald-600 dark:text-emerald-400">{t('queue.empty')}</div> :
           <div className="overflow-x-auto">
             <table className="w-full text-sm">
               <thead className="bg-slate-50 dark:bg-slate-900 text-xs text-slate-500">
                 <tr><th className="text-left p-3">{t('queue.colId')}</th><th className="text-left p-3">{t('queue.colRoute')}</th><th className="text-left p-3">{t('queue.colSizeTime')}</th><th className="text-right p-3">{t('queue.colActions')}</th></tr>
               </thead>
               <tbody className="divide-y divide-slate-100 dark:divide-slate-700">
                 {queue.map(message => <tr key={message.queue_id}>
                   <td className="p-3 font-mono">{message.queue_id}<div className="text-[10px] text-slate-400">{message.queue_name}</div></td>
                   <td className="p-3">
                     <div className="font-mono text-xs">{message.sender || '<>'}</div>
                     <div className="font-mono text-xs text-slate-500">→ {message.recipients.map(recipient => recipient.address).join(', ')}</div>
                     {message.recipients.find(recipient => recipient.delay_reason)?.delay_reason &&
                       <div className="mt-1 text-[10px] text-amber-600 max-w-xl">{message.recipients.find(recipient => recipient.delay_reason)?.delay_reason}</div>}
                   </td>
                   <td className="p-3 text-xs text-slate-500">{formatSize(message.message_size)}<div>{new Date(message.arrival_time * 1000).toLocaleString()}</div></td>
                   <td className="p-3 text-right whitespace-nowrap">
                     {message.queue_name === 'hold' ?
                       <button onClick={() => queueAction('release', message.queue_id)} className="text-xs text-emerald-600 px-2">{t('queue.release')}</button> :
                       <button onClick={() => queueAction('hold', message.queue_id)} className="text-xs text-amber-600 px-2">{t('queue.hold')}</button>}
                     <button onClick={() => queueAction('requeue', message.queue_id)} className="text-xs text-brand-600 px-2">{t('queue.requeue')}</button>
                     <button onClick={() => queueAction('delete', message.queue_id)} className="text-xs text-red-600 px-2">{t('queue.delete')}</button>
                   </td>
                 </tr>)}
               </tbody>
             </table>
           </div>}
        </div>
      </div>
    </>
  )
}
