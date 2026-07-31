import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import type { AxiosProgressEvent } from 'axios'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Inventory = {
  provider: string
  username: string
  primary_domain: string
  archive_root: string
  entry_count: number
  expanded_bytes: number
  web_files: number
  web_bytes: number
  databases: string[]
  dns_zones: string[]
  mail_files: number
  mailboxes: string[]
  alias_count: number
  cron_present: boolean
  cron_jobs: { minute: string; hour: string; day: string; month: string; weekday: string; command: string; comment?: string }[]
  ssl_certs: number
  warnings: string[]
}
type Customer = { id: number; name: string; email: string; plan_id?: number | null }
type Plan = { id: number; name: string }
type ImportResult = {
  domain_id: number; domain: string; system_user: string; web_files: number
  databases: { source: string; target: string; user: string }[]
  mailboxes: { email: string; password: string }[]
  aliases: number
  cron_jobs: number
  ssl_imported: boolean
  ssl_expires?: string
  skipped: string[]
}

export default function AccountTransferPage() {
  const [file, setFile] = useState<File | null>(null)
  const [inventory, setInventory] = useState<Inventory | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [analyzing, setAnalyzing] = useState(false)
  const [progress, setProgress] = useState(0)
  const [customers, setCustomers] = useState<Customer[]>([])
  const [plans, setPlans] = useState<Plan[]>([])
  const [customerID, setCustomerID] = useState('')
  const [planID, setPlanID] = useState('')
  const [domain, setDomain] = useState('')
  const [phpVersion, setPHPVersion] = useState('8.3')
  const [importing, setImporting] = useState(false)
  const [result, setResult] = useState<ImportResult | null>(null)

  useEffect(() => {
    Promise.all([
      api.get<Customer[]>('/customers'),
      api.get<Plan[]>('/plans'),
    ]).then(([c, p]) => {
      setCustomers(c.data)
      setPlans(p.data)
    }).catch(() => { /* selection lists stay empty; an API error surfaces on import */ })
  }, [])

  async function analyze() {
    if (!file) return
    setError(null); setInventory(null); setAnalyzing(true); setProgress(0)
    const form = new FormData()
    form.append('archive', file)
    try {
      const r = await api.post<Inventory>('/admin/transfers/analyze', form, {
        timeout: 0,
        onUploadProgress: (e: AxiosProgressEvent) => {
          if (e.total) setProgress(Math.round((e.loaded / e.total) * 100))
        },
      })
      setInventory(r.data)
      setDomain(r.data.primary_domain || '')
    } catch (e) {
      setError(apiError(e, 'Could not analyze the archive'))
    } finally {
      setAnalyzing(false)
    }
  }

  async function runImport() {
    if (!file || !inventory || !customerID || !domain) return
    setError(null); setResult(null); setImporting(true); setProgress(0)
    const form = new FormData()
    form.append('archive', file)
    form.append('customer_id', customerID)
    form.append('domain', domain)
    form.append('php_version', phpVersion)
    if (planID) form.append('plan_id', planID)
    try {
      const r = await api.post<ImportResult>('/admin/transfers/import', form, {
        timeout: 0,
        onUploadProgress: e => {
          if (e.total) setProgress(Math.round((e.loaded / e.total) * 100))
        },
      })
      setResult(r.data)
    } catch (e) {
      setError(apiError(e, 'Could not import the account'))
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Hosting Services' },
        { label: 'Account Transfer' },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🚚</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Import Account</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">
        Safely analyze a cPanel full-account backup. The archive is not extracted or persisted on the server at this stage.
      </p>

      {error && <div className="mb-4 px-4 py-3 rounded-xl border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-950/30 text-sm text-red-700 dark:text-red-300">{error}</div>}

      <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-5 mb-5">
        <div className="flex flex-col lg:flex-row lg:items-end gap-4">
          <label className="flex-1">
            <span className="block text-sm font-medium text-slate-700 dark:text-slate-200 mb-2">cPanel full backup (.tar.gz / .tgz)</span>
            <input type="file" accept=".tar.gz,.tgz,application/gzip"
              onChange={e => { setFile(e.target.files?.[0] || null); setInventory(null) }}
              className="block w-full text-sm text-slate-600 dark:text-slate-300 file:mr-4 file:rounded-lg file:border-0 file:bg-brand-50 dark:file:bg-brand-950/40 file:px-4 file:py-2.5 file:text-sm file:font-medium file:text-brand-700 dark:file:text-brand-300 hover:file:bg-brand-100" />
          </label>
          <button onClick={analyze} disabled={!file || analyzing}
            className="px-5 py-2.5 rounded-lg bg-brand-600 hover:bg-brand-700 text-white text-sm font-medium disabled:opacity-50">
            {analyzing ? `Uploading ${progress}%` : 'Analyze Archive'}
          </button>
        </div>
        {file && <div className="mt-3 text-xs text-slate-400">{file.name} · {fmtByte(file.size)}</div>}
        {analyzing && <div className="mt-3 h-1.5 rounded-full bg-slate-100 dark:bg-slate-700 overflow-hidden"><div className="h-full bg-brand-600 transition-all" style={{ width: `${progress}%` }} /></div>}
      </div>

      {inventory && (
        <>
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
            <Kpi label="Primary Domain" value={inventory.primary_domain || 'Undetermined'} />
            <Kpi label="Web Files" value={inventory.web_files.toLocaleString('en-US')} alt={fmtByte(inventory.web_bytes)} />
            <Kpi label="Databases" value={String(inventory.databases.length)} />
            <Kpi label="Email Data" value={inventory.mail_files ? `${inventory.mail_files.toLocaleString('en-US')} files` : 'None'} />
          </div>

          {inventory.warnings.length > 0 && (
            <div className="mb-5 rounded-xl border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/20 px-4 py-3">
              {inventory.warnings.map(wn => <div key={wn} className="text-sm text-amber-800 dark:text-amber-300">⚠ {wn}</div>)}
            </div>
          )}

          <div className="grid lg:grid-cols-2 gap-4">
            <Detail title="Account information" rows={[
              ['Source panel', 'cPanel'],
              ['User', inventory.username || '—'],
              ['Archive root', inventory.archive_root || '—'],
              ['Total members', inventory.entry_count.toLocaleString('en-US')],
              ['Expanded size', fmtByte(inventory.expanded_bytes)],
              ['Cron', inventory.cron_present ? 'Present' : 'None'],
              ['SSL files', String(inventory.ssl_certs)],
              ['Mailboxes', String(inventory.mailboxes.length)],
              ['Forwarders', String(inventory.alias_count)],
              ['Cron jobs', String(inventory.cron_jobs.length)],
            ]} />
            <div className="space-y-4">
              <List title="Databases" values={inventory.databases} />
              <List title="DNS zones" values={inventory.dns_zones} />
              <List title="Cron jobs" values={inventory.cron_jobs.map(c => `${c.minute} ${c.hour} ${c.day} ${c.month} ${c.weekday}  ${c.command}`)} />
            </div>
          </div>

          <div className="mt-5 rounded-xl border border-sky-200 dark:border-sky-800 bg-sky-50 dark:bg-sky-950/20 px-4 py-3 text-sm text-sky-800 dark:text-sky-300">
            Analysis complete. Import creates a new domain; if it fails, the created account is rolled back automatically.
          </div>

          <div className="mt-5 rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-5">
            <h2 className="text-base font-semibold text-slate-800 dark:text-slate-100 mb-4">Target and plan</h2>
            <div className="grid md:grid-cols-2 gap-4">
              <Field label="Target customer">
                <select value={customerID} onChange={e => setCustomerID(e.target.value)} className={inputClass}>
                  <option value="">Select a customer</option>
                  {customers.map(c => <option key={c.id} value={c.id}>{c.name} — {c.email}</option>)}
                </select>
              </Field>
              <Field label="Service plan">
                <select value={planID} onChange={e => setPlanID(e.target.value)} className={inputClass}>
                  <option value="">Default plan</option>
                  {plans.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
                </select>
              </Field>
              <Field label="Primary domain">
                <input value={domain} onChange={e => setDomain(e.target.value.toLowerCase())} className={inputClass} />
              </Field>
              <Field label="PHP version">
                <select value={phpVersion} onChange={e => setPHPVersion(e.target.value)} className={inputClass}>
                  {['7.4', '8.2', '8.3', '8.4', '8.5'].map(v => <option key={v}>{v}</option>)}
                </select>
              </Field>
            </div>
            <button onClick={runImport}
              disabled={importing || !customerID || !domain}
              className="mt-5 px-5 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-700 text-white text-sm font-medium disabled:opacity-50">
              {importing ? `Importing… ${progress}%` : 'Import Account'}
            </button>
          </div>

          {result && <div className="mt-5 rounded-2xl border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-950/20 p-5">
            <h2 className="font-semibold text-emerald-800 dark:text-emerald-200">Import complete</h2>
            <p className="mt-1 text-sm text-emerald-700 dark:text-emerald-300">{result.domain} · {result.web_files} web files · {result.databases.length} database(s)</p>
            {result.databases.map(d => <p key={d.target} className="mt-1 text-xs font-mono text-emerald-700 dark:text-emerald-300">{d.source} → {d.target}</p>)}
            {result.mailboxes.length > 0 && <div className="mt-3 rounded-lg border border-amber-200 dark:border-amber-800 bg-white/60 dark:bg-slate-900/40 p-3">
              <p className="text-xs font-semibold text-amber-800 dark:text-amber-200 mb-2">New mailbox passwords — save them now</p>
              {result.mailboxes.map(m => <p key={m.email} className="text-xs font-mono text-amber-800 dark:text-amber-200">{m.email}: {m.password}</p>)}
              <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">{result.aliases} forwarder(s) transferred.</p>
            </div>}
            {result.cron_jobs > 0 && <p className="mt-1 text-xs text-emerald-700 dark:text-emerald-300">{result.cron_jobs} cron job(s) transferred.</p>}
            {result.ssl_imported && <p className="mt-1 text-xs text-emerald-700 dark:text-emerald-300">SSL certificate transferred · valid until {result.ssl_expires}</p>}
            {result.skipped?.map(s => <p key={s} className="mt-1 text-xs text-amber-700 dark:text-amber-300">⚠ {s}</p>)}
            <Link to={`/subscriptions/${result.domain_id}`} className="inline-block mt-3 text-sm font-medium text-brand-700 dark:text-brand-300">Manage domain →</Link>
          </div>}
        </>
      )}
    </div>
  )
}

const inputClass = 'w-full px-3 py-2.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-sm text-slate-800 dark:text-slate-100'
function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label><span className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1.5">{label}</span>{children}</label>
}

function Kpi({ label, value, alt }: { label: string; value: string; alt?: string }) {
  return <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-4">
    <div className="text-[11px] uppercase tracking-wide font-semibold text-slate-400">{label}</div>
    <div className="mt-1 text-lg font-semibold text-slate-800 dark:text-slate-100 truncate">{value}</div>
    {alt && <div className="text-xs text-slate-400">{alt}</div>}
  </div>
}

function Detail({ title, rows }: { title: string; rows: string[][] }) {
  return <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-4">
    <h2 className="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">{title}</h2>
    <dl className="divide-y divide-slate-100 dark:divide-slate-700/60">
      {rows.map(([k, v]) => <div key={k} className="flex justify-between gap-4 py-2 text-sm"><dt className="text-slate-500">{k}</dt><dd className="text-slate-800 dark:text-slate-200 font-mono text-xs text-right">{v}</dd></div>)}
    </dl>
  </div>
}

function List({ title, values }: { title: string; values: string[] }) {
  return <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-4">
    <h2 className="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-2">{title} <span className="text-slate-400">({values.length})</span></h2>
    {values.length ? <div className="flex flex-wrap gap-1.5">{values.map(v => <span key={v} className="px-2 py-1 rounded-md bg-slate-100 dark:bg-slate-700 text-xs font-mono text-slate-700 dark:text-slate-200">{v}</span>)}</div> : <p className="text-xs text-slate-400">None found</p>}
  </div>
}

function fmtByte(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 ** 2) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 ** 3) return `${(b / 1024 ** 2).toFixed(1)} MB`
  return `${(b / 1024 ** 3).toFixed(2)} GB`
}
