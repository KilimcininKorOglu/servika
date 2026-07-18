import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Settings = {
  hdr_x_content_type: boolean
  hdr_x_xss: boolean
  hdr_referrer: boolean
  hdr_permissions: boolean
  hdr_csp_upgrade: boolean
  hdr_hsts: boolean
  hsts_max_age: number
  hsts_subdomains: boolean
  hsts_preload: boolean
  fastcgi_cache: boolean
  fastcgi_cache_minutes: number
  browser_cache: boolean
  browser_cache_days: number
  extra_directives: string
}

type Response = { domain_name: string; settings: Settings }

const BACKEND_INFO: Record<string, { name: string; icon: string; description: string; color: string }> = {
  'php-fpm': {
    name: 'nginx + PHP-FPM',
    icon: '⚡',
    description: 'Default. nginx calls PHP-FPM directly through FastCGI. Ideal for WordPress, Laravel, and dynamic PHP sites with the lowest latency.',
    color: 'emerald',
  },
  'apache': {
    name: 'nginx + Apache',
    icon: '🪶',
    description: 'nginx terminates TLS at the edge, while Apache (10080) serves the vhost behind it. Full .htaccess support for Joomla, older WordPress sites, and legacy CMSs.',
    color: 'indigo',
  },
  'static': {
    name: 'Static (no PHP)',
    icon: '📄',
    description: 'Serves files only. Intended for React, Vue, or Angular SPAs, static site generators such as Hugo and Jekyll, and CDN content. PHP requests return 404.',
    color: 'slate',
  },
}

const HEADERS = [
  { key: 'hdr_x_content_type', label: 'X-Content-Type-Options', value: 'nosniff',
    description: 'Prevents MIME sniffing as an XSS defense' },
  { key: 'hdr_x_xss', label: 'X-XSS-Protection', value: '1; mode=block',
    description: 'Legacy browser XSS protection' },
  { key: 'hdr_referrer', label: 'Referrer-Policy', value: 'strict-origin-when-cross-origin',
    description: 'Restricts cross-site Referer information' },
  { key: 'hdr_permissions', label: 'Permissions-Policy', value: 'geolocation=(), microphone=(), camera=(), interest-cohort=()',
    description: 'Disables camera, microphone, and location APIs by default' },
  { key: 'hdr_csp_upgrade', label: 'Upgrade Insecure Requests', value: 'CSP: upgrade-insecure-requests',
    description: 'Automatically upgrades HTTP links to HTTPS' },
] as const

export default function DomainWebServerPage() {
  const { id } = useParams()
  const [response, setResponse] = useState<Response | null>(null)
  const [settings, setSettings] = useState<Settings | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [processing, setProcessing] = useState(false)

  const [backend, setBackend] = useState<string>('php-fpm')
  const [backendChanging, setBackendChanging] = useState(false)

  function load() {
    if (!id) return
    setLoading(true); setError(null)
    Promise.all([
      api.get<Response>(`/domains/${id}/nginx-settings`),
      api.get<{backend: string}>(`/domains/${id}/web-backend`),
    ]).then(([settingsResponse, backendResponse]) => {
      setResponse(settingsResponse.data); setSettings(settingsResponse.data.settings)
      setBackend(backendResponse.data.backend)
    }).catch(error => setError(apiError(error)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [id])

  async function saveBackend(newBackend: string) {
    if (newBackend === backend || backendChanging) return
    setBackendChanging(true); setError(null); setSuccess(null)
    try {
      await api.put(`/domains/${id}/web-backend`, { backend: newBackend })
      setBackend(newBackend)
      setSuccess(`✓ Web server changed to "${BACKEND_INFO[newBackend]?.name || newBackend}"`)
      setTimeout(() => setSuccess(null), 4000)
    } catch (error) {
      setError(apiError(error, 'Could not change backend'))
    } finally {
      setBackendChanging(false)
    }
  }

  async function save() {
    if (!settings) return
    setProcessing(true); setError(null); setSuccess(null)
    try {
      await api.put(`/domains/${id}/nginx-settings`, { settings })
      setSuccess('✓ Settings applied and nginx reloaded')
      load()
    } catch (error) {
      setError(apiError(error, 'Could not save settings'))
    } finally {
      setProcessing(false)
    }
  }

  function updateSetting<K extends keyof Settings>(key: K, value: Settings[K]) {
    if (!settings) return
    setSettings({ ...settings, [key]: value })
  }

  return (
    <div className="px-6 py-5 max-w-[1100px]">
      <Breadcrumb items={[
        { label: 'Home', href: '/' }, { label: 'Domains', href: '/domains' },
        { label: response?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: 'Apache and nginx Settings' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Apache and nginx Settings</h1>
      {response && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        <Link to={`/subscriptions/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">{response.domain_name}</Link>
        {' · '}Security headers and custom directives. Saving re-renders the nginx vhost.
      </p>}

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {/* Web server stack selector */}
      <div className="mb-6 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <div className="flex items-center justify-between mb-3">
          <div>
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Web Server Stack</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">
              nginx remains the TLS terminator at the edge. The selection below routes the domain to the chosen backend engine.
            </p>
          </div>
          {backendChanging && <span className="text-xs text-slate-400 dark:text-slate-500">Applying…</span>}
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          {(['php-fpm','apache','static'] as const).map(k => {
            const b = BACKEND_INFO[k]
            const enabled = backend === k
            const colorClasses: Record<string, string> = {
              emerald: enabled ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/20 ring-2 ring-emerald-500/20' : 'border-slate-200 dark:border-slate-700 hover:border-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-900/30 dark:bg-emerald-900/20',
              indigo:  enabled ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-900/20 ring-2 ring-indigo-500/20'    : 'border-slate-200 dark:border-slate-700 hover:border-indigo-300 hover:bg-indigo-50 dark:bg-indigo-900/20',
              slate:   enabled ? 'border-slate-500 bg-slate-100 dark:bg-slate-800 ring-2 ring-slate-400/20'      : 'border-slate-200 dark:border-slate-700 hover:border-slate-400 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800',
            }
            return (
              <button key={k} type="button"
                onClick={() => saveBackend(k)}
                disabled={backendChanging || enabled}
                className={`text-left p-4 border rounded-lg transition disabled:cursor-default ${colorClasses[b.color]}`}
              >
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-lg leading-none">{b.icon}</span>
                  {enabled && <span className="text-[10px] uppercase tracking-wider font-semibold text-emerald-700 dark:text-emerald-300">● Active</span>}
                </div>
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{b.name}</div>
                <div className="text-[11px] text-slate-600 dark:text-slate-400 dark:text-slate-500 mt-1.5 leading-snug">{b.description}</div>
              </button>
            )
          })}
        </div>
      </div>

      <div className="mb-5 px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-md text-xs text-amber-800 dark:text-amber-200">
        <strong>HSTS</strong> is only relevant for HTTPS-enabled sites. It is not sent to the browser when the site uses HTTP only.
        Changes automatically trigger <code className="font-mono">nginx -t</code> and <code className="font-mono">reload</code> with zero downtime.
      </div>

      {loading || !settings ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div> : (
        <>
          {/* General security headers */}
          <Card title="Security Headers (HTTP + HTTPS)">
            <div className="space-y-3">
              {HEADERS.map(h => (
                <RowToggle
                  key={h.key}
                  label={h.label}
                  value={h.value}
                  description={h.description}
                  enabled={settings[h.key] as boolean}
                  onToggle={() => updateSetting(h.key as keyof Settings, !settings[h.key] as never)}
                />
              ))}
            </div>
          </Card>

          {/* HSTS-specific settings */}
          <Card title="HTTP Strict Transport Security (HTTPS only)">
            <RowToggle
              label="Strict-Transport-Security"
              value={`max-age=${settings.hsts_max_age}${settings.hsts_subdomains ? '; includeSubDomains' : ''}${settings.hsts_preload ? '; preload' : ''}`}
              description="Browsers connect to the site only over HTTPS. Incorrect configuration is difficult to reverse, so enable it only when appropriate."
              enabled={settings.hdr_hsts}
              onToggle={() => updateSetting('hdr_hsts', !settings.hdr_hsts)}
            />
            {settings.hdr_hsts && (
              <div className="mt-3 pl-4 border-l-2 border-slate-200 dark:border-slate-700 space-y-2">
                <div>
                  <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">max-age (seconds)</label>
                  <select value={settings.hsts_max_age} onChange={event => updateSetting('hsts_max_age', parseInt(event.target.value))}
                    className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono">
                    <option value={300}>5 minutes (for testing)</option>
                    <option value={86400}>1 day</option>
                    <option value={604800}>1 week</option>
                    <option value={2592000}>30 days</option>
                    <option value={15768000}>6 months</option>
                    <option value={31536000}>1 year (recommended)</option>
                    <option value={63072000}>2 years (for preload)</option>
                  </select>
                </div>
                <CheckboxRow
                  label="includeSubDomains"
                  description="Apply to all subdomains after confirming that each one supports HTTPS"
                  checked={settings.hsts_subdomains}
                  onChange={v => updateSetting('hsts_subdomains', v)}
                />
                <CheckboxRow
                  label="preload"
                  description="Include the site in browsers by default (registration at hstspreload.org is required)"
                  checked={settings.hsts_preload}
                  onChange={v => updateSetting('hsts_preload', v)}
                />
              </div>
            )}
          </Card>

          {/* Performance cache */}
          <Card title="Performance Cache">
            <RowToggle
              label="nginx FastCGI Cache"
              value={`x-cache-status header · ${settings.fastcgi_cache_minutes}-minute cache duration`}
              description="Caches WordPress and PHP pages on disk. POST requests, cookies, login pages, and previews are skipped automatically. Resolves the WP Site Health page-cache warning."
              enabled={settings.fastcgi_cache}
              onToggle={() => updateSetting('fastcgi_cache', !settings.fastcgi_cache)}
            />
            {settings.fastcgi_cache && (
              <div className="mt-3 pl-4 border-l-2 border-slate-200 dark:border-slate-700">
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Cache duration (minutes)</label>
                <select value={settings.fastcgi_cache_minutes} onChange={event => updateSetting('fastcgi_cache_minutes', parseInt(event.target.value))}
                  className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono">
                  <option value={5}>5 minutes</option>
                  <option value={15}>15 minutes</option>
                  <option value={60}>1 hour (recommended)</option>
                  <option value={360}>6 hours</option>
                  <option value={1440}>1 day</option>
                </select>
              </div>
            )}

            <div className="mt-4 pt-4 border-t border-slate-100 dark:border-slate-800">
              <RowToggle
                label="Browser Cache (static files)"
                value={`Cache-Control: public, immutable · expires ${settings.browser_cache_days}d`}
                description="Static files such as CSS, JS, PNG, JPG, and WOFF are cached in the browser, making repeat visits load much faster."
                enabled={settings.browser_cache}
                onToggle={() => updateSetting('browser_cache', !settings.browser_cache)}
              />
              {settings.browser_cache && (
                <div className="mt-3 pl-4 border-l-2 border-slate-200 dark:border-slate-700">
                  <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Cache duration (days)</label>
                  <select value={settings.browser_cache_days} onChange={event => updateSetting('browser_cache_days', parseInt(event.target.value))}
                    className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono">
                    <option value={1}>1 day</option>
                    <option value={7}>1 week</option>
                    <option value={30}>30 days (recommended)</option>
                    <option value={90}>3 months</option>
                    <option value={365}>1 year</option>
                  </select>
                </div>
              )}
            </div>
          </Card>

          {/* Additional directives */}
          <Card title="Additional nginx Directives">
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-2">
              This text is appended to the end of the <code className="font-mono">server</code> block. Example: <code className="font-mono">client_max_body_size 200m;</code>
            </p>
            <textarea value={settings.extra_directives} onChange={event => updateSetting('extra_directives', event.target.value)}
              rows={6}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md text-xs font-mono"
              placeholder="# Example:&#10;client_max_body_size 200m;&#10;rewrite ^/old/(.*)$ /new/$1 permanent;" />
          </Card>

          <div className="flex gap-3 mt-6">
            <button onClick={save} disabled={processing}
              className="px-6 py-2.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">
              {processing ? 'Applying…' : '💾 Save and Apply'}
            </button>
            <button onClick={load} disabled={processing}
              className="px-4 py-2.5 border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 text-sm rounded-md">
              Reload
            </button>
          </div>
        </>
      )}
    </div>
  )
}

function Card({ title, children }: { title: string; children: any }) {
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4">
      <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-3 pb-2 border-b border-slate-100 dark:border-slate-800">{title}</h3>
      {children}
    </div>
  )
}

function RowToggle({ label, value, description, enabled, onToggle }:
  { label: string; value: string; description: string; enabled: boolean; onToggle: () => void }) {
  return (
    <div className="flex items-start gap-3 py-2 border-b border-slate-50 last:border-0">
      <button onClick={onToggle}
        className={`flex-shrink-0 mt-0.5 relative inline-flex h-6 w-11 items-center rounded-full transition ${
          enabled ? 'bg-emerald-500' : 'bg-slate-300'
        }`}>
        <span className={`inline-block h-4 w-4 transform rounded-full bg-white dark:bg-slate-800 shadow transition ${enabled ? 'translate-x-6' : 'translate-x-1'}`} />
      </button>
      <div className="flex-1 min-w-0">
        <div className="flex items-baseline justify-between gap-2">
          <div className="font-mono text-sm font-semibold text-slate-900 dark:text-slate-100">{label}</div>
          <code className="text-xs font-mono text-slate-500 dark:text-slate-500 truncate">{value}</code>
        </div>
        <div className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">{description}</div>
      </div>
    </div>
  )
}

function CheckboxRow({ label, description, checked, onChange }:
  { label: string; description: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-start gap-2 cursor-pointer">
      <input type="checkbox" checked={checked} onChange={e => onChange(e.target.checked)}
        className="mt-1 cursor-pointer" />
      <div>
        <div className="font-mono text-xs font-medium text-slate-900 dark:text-slate-100">{label}</div>
        <div className="text-xs text-slate-500 dark:text-slate-500">{description}</div>
      </div>
    </label>
  )
}