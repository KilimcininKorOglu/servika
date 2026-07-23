import { useEffect, useState } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ResourceCard from '@/components/ResourceCard'
import DomainResourceCard from '@/components/DomainResourceCard'
import DomainDashboard from "@/components/DomainDashboard"
import ToolCard from '@/components/ToolCard'
import type { Domain } from '@/components/DomainList'

type Tab = 'dashboard' | 'hosting' | 'getting_started'

const ICONS = {
  connection:  'M13.828 10.172a4 4 0 015.656 5.656l-3 3a4 4 0 01-5.656-5.656m.172-5.172a4 4 0 00-5.656 5.656l-3 3a4 4 0 005.656 5.656',
  files:  'M3 7a2 2 0 012-2h4l2 2h8a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V7z',
  db:        'M4 7c0-1.657 3.582-3 8-3s8 1.343 8 3-3.582 3-8 3-8-1.343-8-3zm0 0v10c0 1.657 3.582 3 8 3s8-1.343 8-3V7M4 12c0 1.657 3.582 3 8 3s8-1.343 8-3',
  ftp:       'M3 16V8a2 2 0 012-2h6l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2zM9 12l3-3 3 3M12 9v6',
  backup:     'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1M16 12l-4 4-4-4M12 16V4',
  copy:     'M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z',
  php:       'M12 14l9-5-9-5-9 5 9 5zm0 0l6.16-3.422a12.083 12.083 0 01.665 6.479A11.952 11.952 0 0012 20.055a11.952 11.952 0 00-6.824-2.998 12.078 12.078 0 01.665-6.479L12 14z',
  log:       'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z',
  cron:      'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z',
  git:       'M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1',
  composer:  'M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3v6M9 12h6',
  service:    'M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4',
  ssl:       'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z',
  lock:     'M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17',
  stats:'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z',
  imunify:   'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z',
  waf:       'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
  mail:     'M3 8l9 6 9-6m-9 6V4m0 0v16',
  dns:       'M21 12a9 9 0 11-18 0 9 9 0 0118 0zM3 12h18M12 3a14 14 0 010 18M12 3a14 14 0 000 18',
  apache:    'M13 10V3L4 14h7v7l9-11h-7z',
}

export default function SubscriptionDetailPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('dashboard')
  const [diskMB, setDiskMB] = useState<number | null>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const [processing, setProcessing] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)
  const [noticeError, setNoticeError] = useState(false)

  function loadDomain() {
    if (!id) return
    api.get<Domain>(`/domains/${id}`)
      .then(r => setDomain(r.data))
      .catch(e => setError(apiError(e, 'Could not load the subscription')))
  }

  useEffect(() => {
    if (!id) return
    loadDomain()
    api.get<{ disk_mb: { usage: number } }>(`/domains/${id}/resources`)
      .then(r => setDiskMB(r.data.disk_mb.usage))
      .catch(() => {})
  }, [id])

  async function toggleSuspension() {
    if (!id || !domain) return
    const suspend = !domain.suspended
    if (suspend && !window.confirm(`Suspend "${domain.domain_name}"? The website will return HTTP 503 until it is resumed.`)) return

    setMenuOpen(false)
    setProcessing(true)
    setError(null)
    setNotice(null)
    setNoticeError(false)
    try {
      await api.post(`/domains/${id}/${suspend ? 'suspend' : 'resume'}`)
      setNotice(suspend ? 'The domain has been suspended.' : 'The domain has been resumed.')
      setDomain(current => current ? { ...current, suspended: suspend, status: suspend ? 'passive' : 'active' } : current)
    } catch (cause) {
      setNoticeError(true)
      setNotice(apiError(cause, 'Could not update the domain suspension state'))
    } finally {
      setProcessing(false)
    }
  }

  if (error) return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'Domains', href: '/domains' }, { label: 'Error' }]} />
      <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md p-4 text-sm text-red-700 dark:text-red-300">{error}</div>
    </div>
  )

  if (!domain) return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'Domains', href: '/domains' }]} />
      <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div>
    </div>
  )

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Domains', href: '/domains' },
        { label: domain.domain_name },
      ]} />

      <div className="flex items-center gap-3 mb-1">
        <h1 className="text-2xl font-semibold text-brand-700 dark:text-brand-300">{domain.domain_name}</h1>
        <button
          onClick={() => navigate('/subscriptions')}
          className="text-slate-400 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300"
          title="Switch subscription"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        <span className={`text-[10px] px-2 py-0.5 rounded uppercase font-semibold tracking-wider flex items-center gap-1 ${
          domain.status === 'active' ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-slate-200 text-slate-600 dark:text-slate-400 dark:text-slate-500'
        }`}>
          <span className={`w-1.5 h-1.5 rounded-full ${domain.status === 'active' ? 'bg-emerald-500' : 'bg-slate-400'}`}></span>
          {domain.status}
        </span>
        {domain.suspended && (
          <span className="text-[10px] px-2 py-0.5 rounded uppercase font-semibold tracking-wider bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300">
            Suspended
          </span>
        )}
        <div className="relative">
          <button
            type="button"
            onClick={() => setMenuOpen(open => !open)}
            disabled={processing}
            className="ml-1 p-1 text-slate-400 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 rounded disabled:opacity-50"
            title="More actions"
            aria-haspopup="menu"
            aria-expanded={menuOpen}
          >
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
              <circle cx="12" cy="5" r="1.5" /><circle cx="12" cy="12" r="1.5" /><circle cx="12" cy="19" r="1.5" />
            </svg>
          </button>
          {menuOpen && (
            <div role="menu" className="absolute left-0 top-full z-20 mt-1 min-w-48 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-1 shadow-lg">
              <button
                type="button"
                role="menuitem"
                onClick={toggleSuspension}
                className={`w-full rounded-md px-3 py-2 text-left text-sm ${domain.suspended ? 'text-emerald-700 dark:text-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-900/20' : 'text-red-700 dark:text-red-300 hover:bg-red-50 dark:hover:bg-red-900/20'}`}
              >
                {domain.suspended ? 'Resume domain' : 'Suspend domain'}
              </button>
            </div>
          )}
        </div>
      </div>

      {notice && (
        <div className={`mb-4 rounded-lg border px-3 py-2 text-sm ${noticeError ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300' : 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-900/20 dark:text-emerald-300'}`}>
          {notice}
        </div>
      )}

      <div className="flex items-center gap-5 border-b border-slate-200 dark:border-slate-700 mb-5">
        <TabBtn enabled={tab === 'dashboard'} onClick={() => setTab('dashboard')}>Dashboard</TabBtn>
        <TabBtn enabled={tab === 'hosting'}   onClick={() => setTab('hosting')}>Hosting and DNS</TabBtn>
        <TabBtn enabled={tab === 'getting_started'} onClick={() => setTab('getting_started')}>Getting Started</TabBtn>
      </div>

      <div className="grid grid-cols-12 gap-5">
        <aside className="col-span-12 lg:col-span-3 space-y-4">
          <WebSitePreview domainName={domain.domain_name} ssl={domain.ssl} />

          <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Statistics</h3>
              <button onClick={() => {
                if (!id) return;
                api.get<{ disk_mb: { usage: number } }>(`/domains/${id}/resources`)
                  .then(r => setDiskMB(r.data.disk_mb.usage))
                  .catch(() => {});
                loadDomain();
              }} className="text-slate-400 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300" title="Refresh">
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
              </button>
            </div>
            <div className="space-y-2.5 text-sm">
              <Stat label="Disk space" value={diskMB != null ? `${diskMB} MB` : '…'} />
              <Stat label="Monthly traffic" value={`${Math.round(domain.traffic_kb / 1024)} MB`} />
              <Stat label="Created" value={domain.created_at} />
              <Stat label="PHP version" value={domain.php_version} />
            </div>
          </div>
        </aside>

        <section className="col-span-12 lg:col-span-6">
          {tab === 'dashboard' && <DomainDashboard domain={domain} />}
          {tab === 'hosting'   && <HostingTab domain={domain} />}
          {tab === 'getting_started' && <GettingStartedTab />}

          <div className="mt-5 pt-3 border-t border-slate-100 dark:border-slate-800 flex items-center justify-between text-xs text-slate-500 dark:text-slate-500 flex-wrap gap-2">
            <div className="flex items-center gap-4">
              <span>Website: <span className="font-mono text-slate-700 dark:text-slate-300">httpdocs</span></span>
              <span>IP: <span className="font-mono text-slate-700 dark:text-slate-300">{domain.ipv4}</span></span>
              <span>System user: <span className="font-mono text-slate-700 dark:text-slate-300">{domain.system_user}</span></span>
            </div>
            <button className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300">Add description</button>
          </div>
        </section>

        <aside className="col-span-12 lg:col-span-3">
          <DomainResourceCard domainId={domain.id} />
        </aside>
      </div>
    </div>
  )
}

function WebSitePreview({ domainName, ssl }: { domainName: string; ssl: boolean }) {
  const url = `${ssl ? 'https' : 'http'}://${domainName}`
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
      <div className="relative aspect-[4/3] bg-gradient-to-br from-slate-800 to-slate-900 overflow-hidden">
        {ssl ? (
          <div className="absolute inset-0 overflow-hidden pointer-events-none">
            <iframe
              src={url}
              title={`${domainName} preview`}
              loading="lazy"
              sandbox="allow-scripts allow-same-origin"
              tabIndex={-1}
              aria-hidden
              className="origin-top-left"
              style={{ width: '400%', height: '400%', transform: 'scale(0.25)', border: 0, background: '#fff' }}
            />
          </div>
        ) : (
          <div className="absolute inset-0 flex flex-col items-center justify-center text-center px-4">
            <svg className="w-9 h-9 text-white/40 mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.542 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" /></svg>
            <div className="text-[11px] text-white/60">Preview is available only for HTTPS sites</div>
            <div className="text-[10px] text-white/40 mt-0.5">It appears automatically when SSL is enabled</div>
          </div>
        )}
        <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent p-3 flex items-center justify-between gap-2">
          <div className="min-w-0">
            <div className="text-[9px] uppercase tracking-wider text-white/60">Website</div>
            <div className="text-xs font-semibold text-white truncate">{domainName}</div>
          </div>
          <a href={url} target="_blank" rel="noreferrer"
            className="shrink-0 inline-flex items-center gap-1 text-[11px] bg-white/90 hover:bg-white text-slate-900 px-2 py-1 rounded-md font-medium transition">
            <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
            Open
          </a>
        </div>
      </div>
    </div>
  )
}

function TabBtn({ enabled, onClick, children }: { enabled: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`relative pb-3 pt-1 text-sm transition ${
        enabled ? 'text-slate-900 dark:text-slate-100 font-semibold' : 'text-slate-500 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300'
      }`}
    >
      {children}
      {enabled && <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-brand-500 rounded-t"></span>}
    </button>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-slate-500 dark:text-slate-500">{label}</span>
      <span className="text-slate-800 dark:text-slate-200 font-medium font-mono">{value}</span>
    </div>
  )
}

function Group({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mb-5 last:mb-0">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-500 mb-2">{title}</h3>
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2.5">{children}</div>
    </section>
  )
}

function DashboardTabContent({ domain }: { domain: Domain }) {
  return (
    <div>
      <Group title="Files and Databases">
        <ToolCard label="Connection Information" description="FTP and database" icon={ICONS.connection} color="emerald" />
        <ToolCard label="Files" description="File manager" icon={ICONS.files} color="amber" phase="F6" />
        <ToolCard label="Databases" description={domain.db_name} icon={ICONS.db} color="violet" phase="F5" />
        <ToolCard label="FTP" description="FTP accounts" icon={ICONS.ftp} color="sky" phase="F4" />
        <ToolCard label="Backup and Restore" description="Backup management" icon={ICONS.backup} color="rose" phase="F12" />
        <ToolCard label="Copy Website" description="Cloning" icon={ICONS.copy} color="sky" />
      </Group>

      <Group title="Development Tools">
        <ToolCard label="PHP" description={`Version ${domain.php_version}`} icon={ICONS.php} color="indigo" phase="F3" />
        <ToolCard label="Logs" description="Access and error logs" icon={ICONS.log} color="slate" phase="F10" />
        <ToolCard label="Scheduled Tasks" description="Cron" icon={ICONS.cron} color="teal" phase="F8" />
        <ToolCard label="Git" description="Repository integration" icon={ICONS.git} color="orange" phase="F9" />
        <ToolCard label="PHP Composer" description="Package manager" icon={ICONS.composer} color="amber" />
        <ToolCard label="Performance" description="Accelerators" icon={ICONS.service} color="emerald" />
      </Group>

      <Group title="Security">
        <ToolCard
          label="SSL/TLS Certificates"
          description={domain.ssl ? `Expires: ${domain.ssl_expiry || '—'}` : 'Let’s Encrypt'}
          icon={ICONS.ssl}
          color={domain.ssl ? 'emerald' : 'rose'}
          phase="F7"
          warning={!domain.ssl ? 'Domain is not protected' : undefined}
        />
        <ToolCard label="Password-Protected Directories" description=".htpasswd" icon={ICONS.lock} color="amber" phase="F7" />
        <ToolCard label="Statistics" description="Traffic analysis" icon={ICONS.stats} color="indigo" phase="F10" />
        <ToolCard label="Imunify" description="Antivirus" icon={ICONS.imunify} color="emerald" />
        <ToolCard label="WAF (Web Application Firewall)" description="ModSecurity + OWASP CRS" icon={ICONS.waf} color="emerald" to={`/subscriptions/${domain.id}/waf`} />
      </Group>
    </div>
  )
}

function HostingTab({ domain }: { domain: Domain }) {
  return (
    <Group title="Hosting Services">
      <ToolCard label="Hosting Settings" description="Document root and options" icon={ICONS.service} color="indigo" to={`/subscriptions/${domain.id}/web-server`} />
      <ToolCard label="Apache and nginx" description="Security headers and additional directives" icon={ICONS.apache} color="orange" to={`/subscriptions/${domain.id}/web-server`} />
      <ToolCard label="DNS Settings" description="A, CNAME, MX" icon={ICONS.dns} color="emerald" to={`/subscriptions/${domain.id}/dns`} />
    </Group>
  )
}

function GettingStartedTab() {
  return (
    <div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-2xl p-5 text-sm text-amber-800 dark:text-amber-200">
      <strong>Getting Started</strong>. This tab will contain setup wizards and will be enabled in F2.
    </div>
  )
}