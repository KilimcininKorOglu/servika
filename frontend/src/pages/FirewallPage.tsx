import { useEffect, useMemo, useState } from 'react'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Rule = {
  id: number; type: 'ban' | 'whitelist' | 'close'; ip: string; port: number
  protocol: string; description: string; enabled: boolean; created_at: string
}
type ListResponse = { rules: Rule[]; protected_ports: number[] }

// Presets for closing commonly exposed ports with one click.
const TEMPLATES = [
  { key: 'close_mysql', icon: '🗄️', name: 'Close External MySQL Access', ports: '3306',
    description: 'Closes database port 3306 to the internet. MySQL remains accessible from the server.' },
  { key: 'close_ftp', icon: '📁', name: 'Close FTP', ports: '21',
    description: 'Closes FTP port 21. You can safely close FTP if you use SFTP.' },
  { key: 'close_mail', icon: '📧', name: 'Close Mail Ports', ports: '25, 465, 587, 110, 143',
    description: 'Closes SMTP, POP3, and IMAP ports. This reduces spam relay risk when no mail server is present.' },
  { key: 'close_rpc', icon: '🔗', name: 'Close RPC / NFS', ports: '111, 2049',
    description: 'Closes rpcbind port 111 and NFS port 2049 when file sharing is not in use.' },
] as const

// Manual rule modes with descriptions and examples.
const MODES = {
  ban: { icon: '🚫', name: 'Block IP', activeColor: 'bg-red-600 border-red-600',
    description: 'Block a specific IP address. Enter a port to block access only to that port, or leave it blank to block access to all ports.',
    example: 'Example: Completely block 45.9.1.2 after repeated SSH attempts.' },
  whitelist: { icon: '✅', name: 'Allow IP', activeColor: 'bg-emerald-600 border-emerald-600',
    description: 'Enter a port to allow access only from these IP addresses and block everyone else. Leave the port blank to give this IP priority access to all ports.',
    example: 'Example: Enter port 8443 and your office IP so only you can access the panel.' },
  close: { icon: '🔒', name: 'Close Port', activeColor: 'bg-amber-600 border-amber-600',
    description: 'Close a port to everyone except allowlisted addresses. Critical ports for SSH, web, the panel, and DNS are protected and cannot be closed.',
    example: 'Example: Close database port 3306 to external access.' },
} as const

export default function FirewallPage() {
  const [rules, setRules] = useState<Rule[]>([])
  const [protectedPorts, setProtectedPorts] = useState<number[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)

  const [type, setType] = useState<'ban' | 'whitelist' | 'close'>('ban')
  const [ip, setIp] = useState('')
  const [port, setPort] = useState('')
  const [protocol, setProtocol] = useState<'tcp' | 'udp'>('tcp')
  const [description, setDescription] = useState('')

  function load() {
    setLoading(true)
    api.get<ListResponse>('/firewall')
      .then(response => { setRules(response.data.rules || []); setProtectedPorts(response.data.protected_ports || []) })
      .catch(e => setError(apiError(e)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [])

  async function applyTemplate(template: typeof TEMPLATES[number]) {
    if (!confirm(`Apply the "${template.name}" template?\nPorts to close: ${template.ports}\nThese ports will no longer be accessible from the internet.`)) return
    setError(null); setSuccess(null); setBusy('template:' + template.key)
    try {
      const { data } = await api.post('/firewall/template', { template: template.key })
      setSuccess(data.added > 0 ? `"${template.name}" applied. ${data.added} rule(s) added.` : `"${template.name}" is already applied. No new rules were added.`)
      load()
    } catch (caughtError) { setError(apiError(caughtError, 'Could not apply the template')) }
    finally { setBusy(null) }
  }

  async function add(event: React.FormEvent) {
    event.preventDefault()
    setError(null); setSuccess(null); setBusy('manual')
    try {
      await api.post('/firewall', {
        type, ip: type === 'close' ? '' : ip.trim(),
        port: port.trim() ? parseInt(port, 10) : 0, protocol, description: description.trim(),
      })
      setSuccess('The rule was added and applied to the firewall.')
      setIp(''); setPort(''); setDescription('')
      load()
    } catch (caughtError) { setError(apiError(caughtError, 'Could not add the rule')) }
    finally { setBusy(null) }
  }

  async function remove(rule: Rule) {
    const summary = rule.type === 'close' ? `close port ${rule.port}` : `${rule.ip}${rule.port ? ':' + rule.port : ''} ${rule.type}`
    if (!confirm(`Delete the "${summary}" rule?`)) return
    setError(null); setSuccess(null); setBusy('remove:' + rule.id)
    try { await api.delete(`/firewall/${rule.id}`); load() }
    catch (caughtError) { setError(apiError(caughtError, 'Could not delete the rule')) }
    finally { setBusy(null) }
  }

  const ipRequired = type !== 'close'
  const mod = MODES[type]
  const protectedPortsText = useMemo(() => protectedPorts.slice().sort((a, b) => a - b).join(', '), [protectedPorts])

  // Generate the live preview sentence.
  const preview = useMemo(() => {
    if (type === 'close') return port ? `Port ${port} will be closed to everyone except allowlisted addresses.` : 'Enter the port to close.'
    const address = ip.trim() || '(enter an IP address)'
    if (type === 'ban') {
      const target = port ? `port ${port}` : 'all ports'
      return `${address} will be blocked from accessing ${target}.`
    }
    // Allowlist mode.
    if (port) return `Port ${port} will be open only to ${address}. Everyone else will be blocked.`
    return `${address} will have priority access to all ports.`
  }, [type, ip, port])

  // Warn about dynamic IP addresses when a port-specific allowlist is active.
  const restrictionWarning = type === 'whitelist' && port.trim() !== ''

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'Firewall' }]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🛡️</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Firewall</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
        Control <strong>who can access your server from the internet</strong>. Apply a preset or add a custom rule.
      </p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      <div className="mb-5 px-4 py-2.5 rounded-lg bg-sky-50 dark:bg-sky-900/20 border border-sky-200 dark:border-sky-800 text-xs text-sky-800 dark:text-sky-200">
        ℹ️ Rules affect only <strong>new connections</strong>. Your active SSH or panel session will remain connected. Protected ports <span className="font-mono">{protectedPortsText || '22, 53, 80, 443, 8080, 8443'}</span> cannot be closed.
      </div>

      {/* ---------- PRESETS ---------- */}
      <h2 className="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-2 flex items-center gap-2">⚡ Presets <span className="text-xs font-normal text-slate-400">apply with one click</span></h2>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-6">
        {TEMPLATES.map(s => (
          <div key={s.key} className="flex items-start gap-3 p-4 rounded-2xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/60">
            <div className="w-10 h-10 rounded-lg bg-slate-100 dark:bg-slate-700 flex items-center justify-center text-xl shrink-0">{s.icon}</div>
            <div className="flex-1 min-w-0">
              <div className="text-sm font-semibold text-slate-800 dark:text-slate-100">{s.name}</div>
              <div className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{s.description}</div>
              <div className="text-[11px] font-mono text-slate-400 mt-1">Port: {s.ports}</div>
            </div>
            <button onClick={() => applyTemplate(s)} disabled={!!busy}
              className="shrink-0 self-center px-3 py-1.5 text-xs font-medium bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg disabled:opacity-50">
              {busy === 'template:' + s.key ? '…' : 'Apply'}
            </button>
          </div>
        ))}
      </div>

      {/* ---------- MANUAL RULE ---------- */}
      <h2 className="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-2">✍️ Custom Rule</h2>
      <form onSubmit={add} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-6">
        {/* Step 1: choose an action. */}
        <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-2">1 · What do you want to do?</div>
        <div className="grid grid-cols-3 gap-2 mb-3">
          {(['ban', 'whitelist', 'close'] as const).map(t => (
            <button key={t} type="button" onClick={() => setType(t)}
              className={`px-3 py-3 text-sm font-medium rounded-lg border text-center transition ${
                type === t ? MODES[t].activeColor + ' text-white'
                  : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700'
              }`}>
              <div className="text-lg leading-none mb-1">{MODES[t].icon}</div>
              {MODES[t].name}
            </button>
          ))}
        </div>
        {/* Selected mode description */}
        <div className="mb-4 px-3 py-2 rounded-lg bg-slate-50 dark:bg-slate-900/40 text-xs text-slate-600 dark:text-slate-300">
          {mod.description}<br /><span className="text-slate-400">{mod.example}</span>
        </div>

        {/* Step 2: enter rule details. */}
        <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-2">2 · Details</div>
        <div className="grid grid-cols-1 sm:grid-cols-4 gap-3">
          {ipRequired && (
            <label className="block sm:col-span-2">
              <span className="text-[11px] text-slate-500 dark:text-slate-400">IP address or range</span>
              <input value={ip} onChange={e => setIp(e.target.value)} required placeholder="1.2.3.4  ·  1.2.3.0/24"
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            </label>
          )}
          <label className="block">
            <span className="text-[11px] text-slate-500 dark:text-slate-400">Port {ipRequired && <span className="text-slate-400">(blank = all)</span>}</span>
            <input value={port} onChange={e => setPort(e.target.value.replace(/[^0-9]/g, ''))} required={type === 'close'} placeholder={type === 'close' ? '3306' : 'e.g. 22'}
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          </label>
          <label className="block">
            <span className="text-[11px] text-slate-500 dark:text-slate-400">Protocol</span>
            <select value={protocol} onChange={e => setProtocol(e.target.value as 'tcp' | 'udp')}
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none">
              <option value="tcp">TCP</option><option value="udp">UDP</option>
            </select>
          </label>
          <label className="block sm:col-span-4">
            <span className="text-[11px] text-slate-500 dark:text-slate-400">Note (optional)</span>
            <input value={description} onChange={e => setDescription(e.target.value)} placeholder="e.g. IP attempting SSH brute force"
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          </label>
        </div>

        {/* Live preview */}
        <div className="mt-3 flex items-center gap-2 px-3 py-2 rounded-lg bg-slate-100 dark:bg-slate-900/60 text-xs">
          <span className="text-slate-400">Preview:</span>
          <span className="font-medium text-slate-700 dark:text-slate-200">{preview}</span>
        </div>

        {/* Dynamic IP warning for an active allowlist restriction */}
        {restrictionWarning && (
          <div className="mt-2 px-3 py-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 text-xs text-amber-800 dark:text-amber-200">
            ⚠️ <strong>Warning:</strong> This port will be open only to the IP address above. If your IP is <strong>dynamic</strong>, you will lose access when it changes.
            SSH port 22 remains open, so you can connect through SSH and delete this rule if you are locked out. Alternatively, use a static IP address.
          </div>
        )}

        <button disabled={busy === 'manual'} className="mt-3 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
          {busy === 'manual' ? 'Applying…' : 'Add and Apply Rule'}
        </button>
      </form>

      {/* ---------- ACTIVE RULES ---------- */}
      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">Active Rules {!loading && <span className="text-slate-400 font-normal">· {rules.length}</span>}</h3>
          <button onClick={load} disabled={loading} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">↻ Refresh</button>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900/50 text-[11px] uppercase tracking-wider text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-700/60">
              <tr>
                <th className="text-left font-medium px-4 py-2.5">Type</th>
                <th className="text-left font-medium px-4 py-2.5">IP / CIDR</th>
                <th className="text-left font-medium px-4 py-2.5">Port</th>
                <th className="text-left font-medium px-4 py-2.5">Proto</th>
                <th className="text-left font-medium px-4 py-2.5 w-full">Note</th>
                <th className="text-right font-medium px-4 py-2.5">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-700/60">
              {loading ? (
                <tr><td colSpan={6} className="px-4 py-10 text-center text-sm text-slate-400">Loading…</td></tr>
              ) : rules.length === 0 ? (
                <tr><td colSpan={6} className="px-4 py-10 text-center">
                  <div className="text-2xl mb-1">🛡️</div>
                  <p className="text-sm text-slate-500 dark:text-slate-400">No rules yet. The server is open to all connections.</p>
                  <p className="text-xs text-slate-400 mt-1">Apply a preset above to get started.</p>
                </td></tr>
              ) : (
                rules.map(rule => (
                  <tr key={rule.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/40">
                    <td className="px-4 py-2.5"><TypeBadge type={rule.type} /></td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-700 dark:text-slate-200">{rule.ip || <span className="text-slate-400">everyone</span>}</td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-600 dark:text-slate-300">{rule.port || <span className="text-slate-400">all</span>}</td>
                    <td className="px-4 py-2.5 font-mono text-[11px] text-slate-500 uppercase">{rule.protocol}</td>
                    <td className="px-4 py-2.5 text-xs text-slate-500 dark:text-slate-400">{rule.description || '—'}</td>
                    <td className="px-4 py-2.5 text-right">
                      <button disabled={!!busy} onClick={() => remove(rule)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">{busy === 'remove:' + rule.id ? '…' : 'Delete'}</button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

function TypeBadge({ type }: { type: Rule['type'] }) {
  const m = {
    ban: ['🚫 Blocked', 'bg-red-100 dark:bg-red-900/40 text-red-700 dark:text-red-300'],
    whitelist: ['✅ Allowed', 'bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300'],
    close: ['🔒 Closed', 'bg-amber-100 dark:bg-amber-900/40 text-amber-800 dark:text-amber-200'],
  }[type]
  return <span className={`inline-block text-xs px-2 py-0.5 rounded-full font-medium ${m[1]}`}>{m[0]}</span>
}
