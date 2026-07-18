import { useEffect, useState } from 'react'
import Breadcrumb from '@/components/Breadcrumb'
import { api, apiError } from '@/lib/api'

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
  const [records, setRecords] = useState<TemplateRow[]>([])
  const [meta, setMeta] = useState<TemplateMeta | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  function loadTemplate() {
    setLoading(true)
    setError(null)
    api.get<{ records: TemplateRow[]; meta: TemplateMeta }>('/dns-template')
      .then(response => {
        setRecords(response.data.records || [])
        setMeta(response.data.meta)
      })
      .catch(cause => setError(apiError(cause, 'Could not load the DNS template')))
      .finally(() => setLoading(false))
  }

  useEffect(loadTemplate, [])

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
      setSuccess('DNS template saved. New domains will use the updated records and settings.')
      loadTemplate()
    } catch (cause) {
      setError(apiError(cause, 'Could not save the DNS template'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="px-6 md:px-8 py-6">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Tools and Settings', href: '/tools-settings' },
        { label: 'DNS Template' },
      ]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Server-wide DNS Template</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        These records are applied when a domain is created or when its default DNS template is restored.
      </p>

      {error && <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-4 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      <div className="mb-4 px-3.5 py-2.5 bg-brand-50 dark:bg-brand-900/20 border border-brand-200 dark:border-brand-800 rounded-lg text-xs text-brand-800 dark:text-brand-200">
        <strong>Placeholders:</strong>{' '}
        <code className="font-mono">{'{DOMAIN}'}</code> domain name ·{' '}
        <code className="font-mono">{'{IP}'}</code> server IP ·{' '}
        <code className="font-mono">{'{SELECTOR}'}</code> DKIM selector ·{' '}
        <code className="font-mono">{'{DKIM}'}</code> generated DKIM public record
      </div>

      {loading ? (
        <div className="py-14 text-center text-sm text-slate-400">Loading…</div>
      ) : !meta ? null : (
        <>
          <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-x-auto mb-5">
            <table className="w-full min-w-[940px] text-left">
              <thead className="bg-slate-50 dark:bg-slate-900/50 text-[11px] uppercase tracking-wide text-slate-500">
                <tr>
                  <th className="px-3 py-2.5 w-36">Name</th>
                  <th className="px-3 py-2.5 w-28">Type</th>
                  <th className="px-3 py-2.5">Value</th>
                  <th className="px-3 py-2.5 w-28">TTL</th>
                  <th className="px-3 py-2.5 w-28">Priority</th>
                  <th className="px-3 py-2.5 w-24">Order</th>
                  <th className="px-3 py-2.5 w-20 text-center">Enabled</th>
                  <th className="px-3 py-2.5 w-12"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {records.map((record, index) => (
                  <tr key={record.id ?? index} className="hover:bg-slate-50 dark:hover:bg-slate-800/60">
                    <td className="px-3 py-2"><input value={record.name} onChange={event => updateRecord(index, { name: event.target.value })} className={`${INPUT_CLASS} font-mono`} /></td>
                    <td className="px-3 py-2">
                      <select value={record.type} onChange={event => updateRecord(index, { type: event.target.value })} className={`${INPUT_CLASS} font-mono`}>
                        {RECORD_TYPES.map(type => <option key={type} value={type}>{type}</option>)}
                      </select>
                    </td>
                    <td className="px-3 py-2"><input value={record.value} onChange={event => updateRecord(index, { value: event.target.value })} className={`${INPUT_CLASS} font-mono`} /></td>
                    <td className="px-3 py-2"><input type="number" min={1} value={record.ttl} onChange={event => updateRecord(index, { ttl: Number(event.target.value) || 3600 })} className={`${INPUT_CLASS} font-mono`} /></td>
                    <td className="px-3 py-2">
                      {record.type === 'MX' || record.type === 'SRV'
                        ? <input type="number" min={0} value={record.priority} onChange={event => updateRecord(index, { priority: Number(event.target.value) || 0 })} className={`${INPUT_CLASS} font-mono`} />
                        : <span className="pl-2 text-slate-300 dark:text-slate-600">—</span>}
                    </td>
                    <td className="px-3 py-2"><input type="number" value={record.sort_order} onChange={event => updateRecord(index, { sort_order: Number(event.target.value) || 0 })} className={`${INPUT_CLASS} font-mono`} /></td>
                    <td className="px-3 py-2 text-center"><input type="checkbox" checked={record.enabled} onChange={event => updateRecord(index, { enabled: event.target.checked })} className="w-4 h-4 accent-brand-600" /></td>
                    <td className="px-3 py-2 text-center"><button type="button" onClick={() => setRecords(current => current.filter((_, recordIndex) => recordIndex !== index))} title="Delete record" className="p-1 text-red-500 hover:text-red-700">×</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
            <button type="button" onClick={addRecord} className="m-3 px-3 py-1.5 text-sm rounded-lg border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">Add record</button>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-5 mb-5">
            <section className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-4">SOA Settings</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                {(['soa_refresh', 'soa_retry', 'soa_expire', 'soa_minimum', 'soa_ttl'] as const).map(field => (
                  <label key={field}>
                    <span className="block text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">{field.replace('soa_', '')} (seconds)</span>
                    <input type="number" min={1} value={meta[field]} onChange={event => setMeta({ ...meta, [field]: Number(event.target.value) || 1 })} className={`${INPUT_CLASS} font-mono`} />
                  </label>
                ))}
              </div>
            </section>

            <section className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-4">DKIM Settings</h2>
              <label className="block mb-3">
                <span className="block text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">Selector</span>
                <input value={meta.dkim_selector} onChange={event => setMeta({ ...meta, dkim_selector: event.target.value })} className={`${INPUT_CLASS} font-mono`} />
              </label>
              <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={meta.dkim_enabled} onChange={event => setMeta({ ...meta, dkim_enabled: event.target.checked })} className="w-4 h-4 accent-brand-600" />
                Generate DKIM keys for new domains
              </label>
            </section>
          </div>

          <div className="flex justify-end gap-2">
            <button type="button" onClick={loadTemplate} disabled={saving} className="px-4 py-2 text-sm rounded-lg border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-50">Discard changes</button>
            <button type="button" onClick={saveTemplate} disabled={saving} className="px-4 py-2 text-sm font-medium rounded-lg bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-50">{saving ? 'Saving…' : 'Save template'}</button>
          </div>
        </>
      )}
    </div>
  )
}
