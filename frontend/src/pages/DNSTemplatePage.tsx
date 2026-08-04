import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import Breadcrumb from '@/components/Breadcrumb'
import { api, apiError } from '@/lib/api'
import {
  responsiveTableActionCellClass,
  responsiveTableBodyClass,
  responsiveTableCellClass,
  responsiveTableClass,
  responsiveTableContainerClass,
  responsiveTableHeadClass,
  responsiveTableRowClass,
} from '@/lib/table'

type TemplateRow = {
  id?: number
  name: string
  type: string
  value: string
  ttl: number
  priority: number
  sort_order: number
  enabled: boolean
}

type TemplateMeta = {
  soa_refresh: number
  soa_retry: number
  soa_expire: number
  soa_minimum: number
  soa_ttl: number
  dkim_selector: string
  dkim_enabled: boolean
}

const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA', 'PTR', 'DS', 'TLSA', 'SSHFP', 'NAPTR']
const INPUT_CLASS = 'w-full px-2.5 py-1.5 bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-600 rounded-lg text-sm text-slate-800 dark:text-slate-100 focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none'

/** Renders the server-wide DNS template editor. */
export default function DNSTemplatePage() {
  const { t } = useTranslation('DNSTemplatePage')
  const [records, setRecords] = useState<TemplateRow[]>([])
  const [meta, setMeta] = useState<TemplateMeta | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  // Split so the mount effect never writes state synchronously: fetchTemplate
  // settles only through promise callbacks, and loadTemplate adds the spinner
  // for the discard button and the post-save refresh.
  const fetchTemplate = useCallback(() => {
    api.get<{ records: TemplateRow[]; meta: TemplateMeta }>('/dns-template')
      .then(response => {
        setRecords(response.data.records || [])
        setMeta(response.data.meta)
      })
      .catch(cause => setError(apiError(cause, t('errors.loadFailed'))))
      .finally(() => setLoading(false))
  }, [t])

  const loadTemplate = useCallback(() => {
    setLoading(true)
    setError(null)
    fetchTemplate()
  }, [fetchTemplate])

  useEffect(() => { fetchTemplate() }, [fetchTemplate])

  function updateRecord(index: number, patch: Partial<TemplateRow>) {
    setRecords(current => current.map((record, recordIndex) => recordIndex === index ? { ...record, ...patch } : record))
  }

  function addRecord() {
    setRecords(current => [...current, {
      name: '@',
      type: 'A',
      value: '{IP}',
      ttl: 3600,
      priority: 0,
      sort_order: (current.length + 1) * 10,
      enabled: true,
    }])
  }

  async function saveTemplate() {
    if (!meta) return
    setSaving(true)
    setError(null)
    setSuccess(null)
    try {
      await api.put('/dns-template', { records, meta })
      setSuccess(t('success'))
      loadTemplate()
    } catch (cause) {
      setError(apiError(cause, t('errors.saveFailed')))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="px-4 py-4 sm:px-6 sm:py-5">
      <Breadcrumb items={[
        { label: t('breadcrumb.home'), href: '/' },
        { label: t('breadcrumb.tools'), href: '/tools-settings' },
        { label: t('breadcrumb.current') },
      ]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('title')}</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        {t('subtitle')}
      </p>

      {error && <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-4 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      <div className="mb-4 px-3.5 py-2.5 bg-brand-50 dark:bg-brand-900/20 border border-brand-200 dark:border-brand-800 rounded-lg text-xs text-brand-800 dark:text-brand-200">
        <strong>{t('placeholders.label')}</strong>{' '}
        <code className="font-mono">{'{DOMAIN}'}</code>{t('placeholders.domain')}
        <code className="font-mono">{'{IP}'}</code>{t('placeholders.ip')}
        <code className="font-mono">{'{SELECTOR}'}</code>{t('placeholders.selector')}
        <code className="font-mono">{'{DKIM}'}</code>{t('placeholders.dkim')}
      </div>

      {loading ? (
        <div className="py-14 text-center text-sm text-slate-400">{t('loading')}</div>
      ) : !meta ? null : (
        <>
          <div className={`${responsiveTableContainerClass} mb-5`}>
            <table className={responsiveTableClass}>
              <thead className={responsiveTableHeadClass}>
                <tr>
                  <th className="px-3 py-2.5 w-36">{t('table.name')}</th>
                  <th className="px-3 py-2.5 w-28">{t('table.type')}</th>
                  <th className="px-3 py-2.5">{t('table.value')}</th>
                  <th className="px-3 py-2.5 w-28">{t('table.ttl')}</th>
                  <th className="px-3 py-2.5 w-28">{t('table.priority')}</th>
                  <th className="px-3 py-2.5 w-24">{t('table.order')}</th>
                  <th className="px-3 py-2.5 w-20 text-center">{t('table.enabled')}</th>
                  <th className="px-3 py-2.5 w-12"></th>
                </tr>
              </thead>
              <tbody className={responsiveTableBodyClass}>
                {records.map((record, index) => (
                  <tr key={record.id ?? index} className={responsiveTableRowClass}>
                    <td data-label={t('table.name')} className={responsiveTableCellClass}><input value={record.name} onChange={event => updateRecord(index, { name: event.target.value })} className={`${INPUT_CLASS} font-mono`} /></td>
                    <td data-label={t('table.type')} className={responsiveTableCellClass}>
                      <select value={record.type} onChange={event => updateRecord(index, { type: event.target.value })} className={`${INPUT_CLASS} font-mono`}>
                        {RECORD_TYPES.map(type => <option key={type} value={type}>{type}</option>)}
                      </select>
                    </td>
                    <td data-label={t('table.value')} className={responsiveTableCellClass}><input value={record.value} onChange={event => updateRecord(index, { value: event.target.value })} className={`${INPUT_CLASS} font-mono`} /></td>
                    <td data-label={t('table.ttl')} className={responsiveTableCellClass}><input type="number" min={1} value={record.ttl} onChange={event => updateRecord(index, { ttl: Number(event.target.value) || 3600 })} className={`${INPUT_CLASS} font-mono`} /></td>
                    <td data-label={t('table.priority')} className={responsiveTableCellClass}>
                      {record.type === 'MX' || record.type === 'SRV'
                        ? <input type="number" min={0} value={record.priority} onChange={event => updateRecord(index, { priority: Number(event.target.value) || 0 })} className={`${INPUT_CLASS} font-mono`} />
                        : <span className="pl-2 text-slate-300 dark:text-slate-600">{t('table.priorityNone')}</span>}
                    </td>
                    <td data-label={t('table.order')} className={responsiveTableCellClass}><input type="number" value={record.sort_order} onChange={event => updateRecord(index, { sort_order: Number(event.target.value) || 0 })} className={`${INPUT_CLASS} font-mono`} /></td>
                    <td data-label={t('table.enabled')} className={responsiveTableCellClass}><input type="checkbox" checked={record.enabled} onChange={event => updateRecord(index, { enabled: event.target.checked })} className="w-4 h-4 accent-brand-600" /></td>
                    <td className={responsiveTableActionCellClass}><button type="button" onClick={() => setRecords(current => current.filter((_, recordIndex) => recordIndex !== index))} title={t('table.deleteTitle')} className="p-1 text-red-500 hover:text-red-700">×</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
            <button type="button" onClick={addRecord} className="m-3 px-3 py-1.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('addRecord')}</button>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-5 mb-5">
            <section className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-4">{t('soa.title')}</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                {(['soa_refresh', 'soa_retry', 'soa_expire', 'soa_minimum', 'soa_ttl'] as const).map(field => (
                  <label key={field}>
                    <span className="block text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">{t('soa.fieldSeconds', { field: field.replace('soa_', '') })}</span>
                    <input type="number" min={1} value={meta[field]} onChange={event => setMeta({ ...meta, [field]: Number(event.target.value) || 1 })} className={`${INPUT_CLASS} font-mono`} />
                  </label>
                ))}
              </div>
            </section>

            <section className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-4">{t('dkim.title')}</h2>
              <label className="block mb-3">
                <span className="block text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">{t('dkim.selector')}</span>
                <input value={meta.dkim_selector} onChange={event => setMeta({ ...meta, dkim_selector: event.target.value })} className={`${INPUT_CLASS} font-mono`} />
              </label>
              <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={meta.dkim_enabled} onChange={event => setMeta({ ...meta, dkim_enabled: event.target.checked })} className="w-4 h-4 accent-brand-600" />
                {t('dkim.generate')}
              </label>
            </section>
          </div>

          <div className="flex justify-end gap-2">
            <button type="button" onClick={loadTemplate} disabled={saving} className="px-4 py-2 text-sm rounded-lg border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-50">{t('discard')}</button>
            <button type="button" onClick={saveTemplate} disabled={saving} className="px-4 py-2 text-sm font-medium rounded-lg bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-50">{saving ? t('saving') : t('save')}</button>
          </div>
        </>
      )}
    </div>
  )
}
