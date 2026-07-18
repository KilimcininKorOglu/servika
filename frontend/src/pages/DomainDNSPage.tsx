import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

type RecordItem = {
  id: number
  domain_id: number
  name: string
  type: string
  value: string
  ttl: number
  priority: number
  active: boolean
  created_at: string
}

type Domain = { id: number; domain_name: string; ipv4: string }
type SOA = { primary_ns: string; hostmaster: string; refresh: number; retry: number; expire: number; minimum: number; ttl: number }
type DNSSECStatus = { active: boolean; signed: boolean; ds: string[]; status: string }

const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA', 'PTR', 'DS', 'TLSA', 'SSHFP', 'NAPTR']

// Format hint displayed in the Value field for each record type.
const VALUE_HINT: Record<string, string> = {
  A:     'IPv4 address, for example 203.0.113.10',
  AAAA:  'IPv6 address, for example 2a01:4f8:1c1c::1',
  CNAME: 'Target domain name, for example target.example.com',
  MX:    'Mail server, for example mail.example.com (priority is set separately)',
  TXT:   'Free-form text, for example v=spf1 mx ~all',
  NS:    'Name server, for example ns1.example.com',
  SRV:   'weight port target, for example 5 5060 sip.example.com (priority is set separately)',
  CAA:   'flags tag "value", for example 0 issue "letsencrypt.org"',
  PTR:   'Target domain name, for example host.example.com',
  DS:    'keytag algorithm digest-type digest, for example 12345 13 2 49FD46E6C4B45C55D4AC…',
  TLSA:  'usage selector matching-type data, for example 3 1 1 0B9FA5A59EED715C26C1020C…',
  SSHFP: 'algorithm type fingerprint, for example 4 2 123456789ABCDEF…',
  NAPTR: 'order preference "flags" "service" "regexp" replacement',
}

export default function DomainDNSPage() {
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [records, setRecords] = useState<RecordItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [editingRecord, setEdit] = useState<RecordItem | null>(null)
  const [recordToDelete, setRecordToDelete] = useState<RecordItem | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [bulkDeleteConfirmationOpen, setBulkDeleteConfirmationOpen] = useState(false)
  const [soa, setSOA] = useState<SOA | null>(null)
  const [soaOpen, setSOAOpen] = useState(false)
  const [dnssec, setDNSSEC] = useState<DNSSECStatus | null>(null)
  const [dnssecProcessing, setDNSSECProcessing] = useState(false)
  const [dnssecDisableConfirmationOpen, setDNSSECDisableConfirmationOpen] = useState(false)
  const [dsCopied, setDSCopied] = useState(false)

  function load() {
    if (!id) return
    setLoading(true); setError(null)
    api.get<RecordItem[]>(`/domains/${id}/dns`)
      .then(r => { setRecords(r.data); setSelected(new Set()) })
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }

  function toggleSelection(rid: number) {
    setSelected(prev => {
      const n = new Set(prev)
      if (n.has(rid)) n.delete(rid); else n.add(rid)
      return n
    })
  }
  function selectAll() {
    setSelected(prev => prev.size === records.length ? new Set() : new Set(records.map(k => k.id)))
  }

  async function bulkDelete() {
    if (!id || selected.size === 0) return
    setError(null); setSuccess(null); setBulkDeleteConfirmationOpen(false)
    try {
      const { data } = await api.post(`/domains/${id}/dns/bulk-delete`, { ids: [...selected] })
      setSuccess(`${data.deleted} records deleted`)
      load()
    } catch (e) { setError(apiError(e, 'Bulk deletion failed')) }
  }
  async function bulkStatus(active: boolean) {
    if (!id || selected.size === 0) return
    setError(null); setSuccess(null)
    try {
      const { data } = await api.post(`/domains/${id}/dns/bulk-status`, { ids: [...selected], active: active })
      setSuccess(`${data.updated} records updated`)
      load()
    } catch (e) { setError(apiError(e, 'Bulk update failed')) }
  }
  useEffect(() => {
    if (id) {
      api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
      api.get<SOA>(`/domains/${id}/dns/soa`).then(r => setSOA(r.data)).catch(() => {})
      api.get<DNSSECStatus>(`/domains/${id}/dns/dnssec`).then(r => setDNSSEC(r.data)).catch(() => {})
    }
    load()
  }, [id])

  async function changeDNSSEC(active: boolean) {
    if (!id) return
    setError(null); setSuccess(null); setDNSSECDisableConfirmationOpen(false); setDNSSECProcessing(true)
    try {
      const { data } = await api.post<DNSSECStatus>(`/domains/${id}/dns/dnssec`, { active })
      setDNSSEC(data)
      setSuccess(active
        ? 'DNSSEC enabled. Add the DS record below at your domain registrar.'
        : 'DNSSEC disabled.')
    } catch (e) {
      setError(apiError(e, 'Could not update DNSSEC'))
    } finally {
      setDNSSECProcessing(false)
    }
  }

  async function refreshDNSSEC() {
    if (!id) return
    try {
      const { data } = await api.get<DNSSECStatus>(`/domains/${id}/dns/dnssec`)
      setDNSSEC(data)
    } catch (e) {
      setError(apiError(e, 'Could not refresh DNSSEC status'))
    }
  }

  async function saveSOA() {
    if (!id || !soa) return
    setError(null); setSuccess(null)
    try {
      const { data } = await api.put<SOA>(`/domains/${id}/dns/soa`, soa)
      setSOA(data)
      setSuccess('SOA settings saved')
    } catch (e) { setError(apiError(e, 'Could not save SOA settings')) }
  }

  async function applyTemplate() {
    if (!id) return
    setError(null); setSuccess(null)
    try {
      const { data } = await api.post(`/domains/${id}/dns/template`)
      setSuccess(`${data.added} default records added`)
      load()
    } catch (e) {
      setError(apiError(e, 'Could not apply template'))
    }
  }

  async function remove() {
    if (!recordToDelete || !id) return
    try {
      await api.delete(`/domains/${id}/dns/${recordToDelete.id}`)
      setRecordToDelete(null); load()
    } catch (e) {
      alert(apiError(e, 'Deletion failed'))
    }
  }

  return (
    <div className="px-6 py-5 max-w-[1300px]">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Domains', href: '/domains' },
        { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: 'DNS Settings' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">DNS Settings</h1>
      {domain && (
        <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
          <Link to={`/subscriptions/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">{domain.domain_name}</Link>
          {' · '}IP: <span className="font-mono">{domain.ipv4}</span>
        </p>
      )}

      <div className="bg-sky-50 dark:bg-sky-900/20 border border-sky-200 dark:border-sky-800 rounded-md px-3 py-2 text-xs text-sky-800 dark:text-sky-200 mb-4">
        <strong>Authoritative DNS:</strong> BIND publishes these records. Point the domain to <span className="font-mono">ns1.{domain?.domain_name || 'your-domain'}</span> and <span className="font-mono">ns2.{domain?.domain_name || 'your-domain'}</span>.
      </div>

      {soa && (
        <div className="border border-slate-200 dark:border-slate-800 rounded-xl mb-4 overflow-hidden">
          <button onClick={() => setSOAOpen(value => !value)} className="w-full flex items-center justify-between px-4 py-2.5 text-sm font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-800/50 transition">
            <span>SOA Settings <span className="text-xs text-slate-400 font-normal">(authority, refresh, retry, expire, and TTL)</span></span>
            <span className="text-slate-400 text-xs">{soaOpen ? '▲ Hide' : '▼ Edit'}</span>
          </button>
          {soaOpen && (
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3 p-4 border-t border-slate-100 dark:border-slate-800">
              <label className="col-span-2">
                <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Primary NS</span>
                <input value={soa.primary_ns} onChange={e => setSOA({ ...soa, primary_ns: e.target.value })}
                  className="mt-1 w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono outline-none focus:border-brand-500" />
              </label>
              <label className="col-span-2">
                <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">Hostmaster Email</span>
                <input value={soa.hostmaster} onChange={e => setSOA({ ...soa, hostmaster: e.target.value })}
                  className="mt-1 w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono outline-none focus:border-brand-500" />
              </label>
              {(['refresh', 'retry', 'expire', 'minimum', 'ttl'] as const).map(field => (
                <label key={field}>
                  <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{field}</span>
                  <input type="number" min={1} value={soa[field]} onChange={e => setSOA({ ...soa, [field]: parseInt(e.target.value) || 0 })}
                    className="mt-1 w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono outline-none focus:border-brand-500" />
                </label>
              ))}
              <div className="flex items-end">
                <button onClick={saveSOA} className="px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded-md">Save SOA</button>
              </div>
            </div>
          )}
        </div>
      )}

      {dnssec && (
        <div className="border border-slate-200 dark:border-slate-800 rounded-xl mb-4 overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 gap-3 flex-wrap">
            <div>
              <div className="text-sm font-medium text-slate-700 dark:text-slate-200 flex items-center gap-2">
                DNSSEC
                {dnssec.active ? (
                  dnssec.signed
                    ? <span className="text-xs px-1.5 py-0.5 rounded bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 font-medium">Signed</span>
                    : <span className="text-xs px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 font-medium">Signing…</span>
                ) : (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 font-medium">Disabled</span>
                )}
              </div>
              <p className="text-xs text-slate-400 dark:text-slate-500 mt-0.5">Signs this zone with BIND inline signing. Add the generated DS record at your domain registrar.</p>
            </div>
            <div className="flex items-center gap-2">
              {dnssec.active && (
                <button onClick={refreshDNSSEC} className="px-2.5 py-1.5 text-xs bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 rounded-md transition">Refresh status</button>
              )}
              {dnssec.active ? (
                <button disabled={dnssecProcessing} onClick={() => setDNSSECDisableConfirmationOpen(true)} className="px-3 py-1.5 text-sm bg-white dark:bg-slate-800 border border-red-200 dark:border-red-800 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 rounded-md transition disabled:opacity-50">Disable</button>
              ) : (
                <button disabled={dnssecProcessing} onClick={() => changeDNSSEC(true)} className="px-3 py-1.5 text-sm bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 font-medium rounded-md transition disabled:opacity-50">{dnssecProcessing ? 'Enabling…' : 'Enable'}</button>
              )}
            </div>
          </div>
          {dnssec.active && (
            <div className="px-4 pb-4 border-t border-slate-100 dark:border-slate-800 pt-3">
              {dnssec.ds.length > 0 ? (
                <div>
                  <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">DS record for your registrar</div>
                  {dnssec.ds.map((record, index) => (
                    <div key={index} className="flex items-center gap-2 mb-1">
                      <code className="flex-1 text-xs font-mono bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded px-2 py-1 break-all text-slate-800 dark:text-slate-200">{record}</code>
                      <button onClick={() => { void navigator.clipboard?.writeText(record); setDSCopied(true); setTimeout(() => setDSCopied(false), 1500) }}
                        className="px-2 py-1 text-xs bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 rounded transition whitespace-nowrap">{dsCopied ? 'Copied' : 'Copy'}</button>
                    </div>
                  ))}
                  <p className="text-[11px] text-slate-400 dark:text-slate-500 mt-1">Add this DS record in your registrar's DNSSEC settings. Propagation may take up to the record TTL.</p>
                </div>
              ) : (
                <p className="text-xs text-amber-600 dark:text-amber-400">Signing is still in progress. Refresh the status after a few seconds.</p>
              )}
              {dnssec.status && (
                <pre className="mt-2 text-[10px] font-mono text-slate-500 dark:text-slate-400 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded p-2 overflow-x-auto max-h-44">{dnssec.status}</pre>
              )}
            </div>
          )}
        </div>
      )}

      <div className="flex items-center gap-2 mb-4">
        <button
          onClick={() => setEdit({} as RecordItem)}
          className="inline-flex items-center gap-1.5 px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md shadow-sm transition"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          New Record
        </button>
        <button
          onClick={applyTemplate}
          className="px-3 py-2 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md transition"
          title="Adds the default A/MX/TXT/NS records (idempotent)"
        >
          📋 Apply Default Template
        </button>
        <button onClick={load} className="px-3 py-2 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md transition">↻ Refresh</button>
        <span className="ml-auto text-sm text-slate-500 dark:text-slate-500">{records.length} records</span>
      </div>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {selected.size > 0 && (
        <div className="mb-3 px-3 py-2 bg-brand-50 dark:bg-brand-900/20 border border-brand-200 dark:border-brand-800 rounded-md flex items-center gap-2 flex-wrap">
          <span className="text-sm font-medium text-brand-800 dark:text-brand-200">{selected.size} records selected</span>
          <div className="ml-auto flex items-center gap-2 flex-wrap">
            <button onClick={() => bulkStatus(true)} className="px-3 py-1.5 text-sm bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-md text-emerald-700 dark:text-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-900/30 transition">Enable</button>
            <button onClick={() => bulkStatus(false)} className="px-3 py-1.5 text-sm bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-700 transition">Disable</button>
            <button onClick={() => setBulkDeleteConfirmationOpen(true)} className="px-3 py-1.5 text-sm bg-red-600 hover:bg-red-700 text-white rounded-md transition">Delete Selected ({selected.size})</button>
            <button onClick={() => setSelected(new Set())} className="px-2 py-1.5 text-sm text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition">Clear Selection</button>
          </div>
        </div>
      )}

      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
        {loading ? (
          <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div>
        ) : records.length === 0 ? (
          <div className="py-12 text-center">
            <p className="text-sm text-slate-500 dark:text-slate-500 mb-3">No DNS records yet.</p>
            <button onClick={applyTemplate} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md">
              Apply Default Template
            </button>
          </div>
        ) : (
          <table className="w-full">
            <thead className="bg-slate-50 dark:bg-slate-900 text-xs uppercase tracking-wider text-slate-500 dark:text-slate-500 border-b border-slate-200 dark:border-slate-700">
              <tr>
                <th className="px-4 py-2.5 w-10">
                  <input type="checkbox" aria-label="Select all" checked={records.length > 0 && selected.size === records.length}
                    ref={el => { if (el) el.indeterminate = selected.size > 0 && selected.size < records.length }}
                    onChange={selectAll} className="rounded border-slate-300 dark:border-slate-600 cursor-pointer" />
                </th>
                <th className="text-left px-4 py-2.5">Name</th>
                <th className="text-left px-4 py-2.5">Type</th>
                <th className="text-left px-4 py-2.5">Value</th>
                <th className="text-left px-4 py-2.5">TTL</th>
                <th className="text-left px-4 py-2.5">Priority</th>
                <th className="text-left px-4 py-2.5">Status</th>
                <th className="text-right px-4 py-2.5">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {records.map(k => (
                <tr key={k.id} className={selected.has(k.id) ? 'bg-brand-50/60 dark:bg-brand-900/10' : 'hover:bg-slate-50 dark:hover:bg-slate-800/60'}>
                  <td className="px-4 py-2.5">
                    <input type="checkbox" aria-label={`Select ${k.name} ${k.type}`} checked={selected.has(k.id)} onChange={() => toggleSelection(k.id)}
                      className="rounded border-slate-300 dark:border-slate-600 cursor-pointer" />
                  </td>
                  <td className="px-4 py-2.5 text-sm font-mono">{k.name}</td>
                  <td className="px-4 py-2.5">
                    <span className="text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 rounded font-mono font-semibold">{k.type}</span>
                  </td>
                  <td className="px-4 py-2.5 text-sm font-mono text-slate-800 dark:text-slate-200 break-all">{k.value}</td>
                  <td className="px-4 py-2.5 text-sm font-mono text-slate-600 dark:text-slate-400 dark:text-slate-500">{k.ttl}</td>
                  <td className="px-4 py-2.5 text-sm font-mono text-slate-600 dark:text-slate-400 dark:text-slate-500">{k.type === 'MX' || k.type === 'SRV' ? k.priority : '—'}</td>
                  <td className="px-4 py-2.5">
                    {k.active ? (
                      <span className="text-xs text-emerald-700 dark:text-emerald-300 inline-flex items-center gap-1"><span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>Active</span>
                    ) : (
                      <span className="text-xs text-slate-500 dark:text-slate-500">Disabled</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-right space-x-1">
                    <button onClick={() => setEdit(k)} className="text-sm text-slate-600 dark:text-slate-400 dark:text-slate-500 hover:text-slate-900 dark:hover:text-slate-100 dark:text-slate-100 px-2 py-1 rounded hover:bg-slate-100 dark:bg-slate-800 dark:hover:bg-slate-800">Edit</button>
                    <button onClick={() => setRecordToDelete(k)} className="text-sm text-red-600 dark:text-red-400 hover:text-red-700 dark:text-red-300 px-2 py-1 rounded hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20">Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {editingRecord && (
        <RecordModal
          current={editingRecord}
          domainId={Number(id)}
          ipv4={domain?.ipv4 || ''}
          onClose={() => setEdit(null)}
          onSaved={() => { setEdit(null); load() }}
        />
      )}

      <ConfirmDialog
        open={!!recordToDelete}
        title="Delete DNS record"
        message={`Delete "${recordToDelete?.name} ${recordToDelete?.type} ${recordToDelete?.value.slice(0, 40)}"?`}
        dangerous
        confirmText="Yes, delete"
        onConfirm={remove}
        onCancel={() => setRecordToDelete(null)}
      />

      <ConfirmDialog
        open={bulkDeleteConfirmationOpen}
        title="Delete selected DNS records"
        message={`${selected.size} DNS records will be permanently deleted. This action cannot be undone. Continue?`}
        dangerous
        confirmText={`Yes, delete ${selected.size} records`}
        onConfirm={bulkDelete}
        onCancel={() => setBulkDeleteConfirmationOpen(false)}
      />

      <ConfirmDialog
        open={dnssecDisableConfirmationOpen}
        title="Disable DNSSEC"
        message="Remove the DS record at your domain registrar and wait for its TTL before disabling DNSSEC. Otherwise the domain may stop resolving with SERVFAIL. Continue only after the DS record has been removed."
        dangerous
        confirmText="DS removed, disable DNSSEC"
        onConfirm={() => changeDNSSEC(false)}
        onCancel={() => setDNSSECDisableConfirmationOpen(false)}
      />
    </div>
  )
}

function RecordModal({ current, domainId, ipv4, onClose, onSaved }: {
  current: RecordItem; domainId: number; ipv4: string; onClose: () => void; onSaved: () => void
}) {
  const isNew = !current.id
  const [form, setForm] = useState<RecordItem>({
    id: current.id || 0,
    domain_id: domainId,
    name: current.name || '@',
    type: current.type || 'A',
    value: current.value || ipv4,
    ttl: current.ttl || 3600,
    priority: current.priority || 0,
    active: current.active !== false,
    created_at: '',
  })
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setProcessing(true); setError(null)
    try {
      if (isNew) await api.post(`/domains/${domainId}/dns`, form)
      else      await api.put(`/domains/${domainId}/dns/${form.id}`, form)
      onSaved()
    } catch (e) {
      setError(apiError(e, 'Could not save record'))
    } finally {
      setProcessing(false)
    }
  }

  return (
    <Modal open={true} title={isNew ? 'New DNS Record' : 'Edit DNS Record'} onClose={onClose} width="md">
      <form onSubmit={submit} className="space-y-3">
        <div className="grid grid-cols-3 gap-3">
          <div className="col-span-2">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Name (subdomain)</label>
            <input type="text" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} required
              className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            <p className="text-[10px] text-slate-500 dark:text-slate-500 mt-0.5">“@” is the root domain. Other examples include “www” and “mail”.</p>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Type</label>
            <select value={form.type} onChange={e => {
              const type = e.target.value
              setForm({ ...form, type, priority: type === 'MX' || type === 'SRV' ? 10 : 0 })
            }}
              className="w-full px-2 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono bg-white dark:bg-slate-800">
              {RECORD_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
            </select>
          </div>
        </div>

        <div>
          <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Value</label>
          <input type="text" value={form.value} onChange={e => setForm({ ...form, value: e.target.value })} required
            className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          {VALUE_HINT[form.type] && <p className="text-[10px] text-slate-500 dark:text-slate-500 mt-0.5">{VALUE_HINT[form.type]}</p>}
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">TTL (sec)</label>
            <input type="number" min={60} value={form.ttl} onChange={e => setForm({ ...form, ttl: parseInt(e.target.value) || 3600 })}
              className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono" />
          </div>
          {(form.type === 'MX' || form.type === 'SRV') && (
            <div>
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Priority</label>
              <input type="number" min={0} value={form.priority} onChange={e => setForm({ ...form, priority: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono" />
            </div>
          )}
        </div>

        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
          <input type="checkbox" checked={form.active} onChange={e => setForm({ ...form, active: e.target.checked })} className="rounded" />
          Active
        </label>

        {error && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-700 dark:text-red-300">{error}</div>}

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onClose} className="px-4 py-2 border border-slate-200 dark:border-slate-700 rounded-md text-sm">Cancel</button>
          <button type="submit" disabled={processing || !form.value.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm rounded-md">{processing ? 'Saving…' : (isNew ? 'Create' : 'Update')}</button>
        </div>
      </form>
    </Modal>
  )
}