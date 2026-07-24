import { useNavigate } from 'react-router-dom'
import type { Domain } from './DomainList'
import ToolCard from './ToolCard'

const ICONS = {
  connection:  'M13.828 10.172a4 4 0 015.656 5.656l-3 3a4 4 0 01-5.656-5.656m.172-5.172a4 4 0 00-5.656 5.656l-3 3a4 4 0 005.656 5.656',
  files:  'M3 7a2 2 0 012-2h4l2 2h8a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V7z',
  db:        'M4 7c0-1.657 3.582-3 8-3s8 1.343 8 3-3.582 3-8 3-8-1.343-8-3zm0 0v10c0 1.657 3.582 3 8 3s8-1.343 8-3V7M4 12c0 1.657 3.582 3 8 3s8-1.343 8-3',
  ftp:       'M3 16V8a2 2 0 012-2h6l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2zM9 12l3-3 3 3M12 9v6',
  backup:    'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1M16 12l-4 4-4-4M12 16V4',
  copy:      'M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z',
  php:       'M12 14l9-5-9-5-9 5 9 5zm0 0l6.16-3.422a12.083 12.083 0 01.665 6.479A11.952 11.952 0 0012 20.055a11.952 11.952 0 00-6.824-2.998 12.078 12.078 0 01.665-6.479L12 14z',
  log:       'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z',
  cron:      'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z',
  navigateTo:       'M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1',
  composer:  'M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3v6M9 12h6',
  service:   'M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4',
  ssl:       'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z',
  lock:      'M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3',
  stats:'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z',
  imunify:   'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622',
  ssh:       'M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z',
  wordpress: 'M12 21a9 9 0 100-18 9 9 0 000 18zm0 0c2.5-2.5 3-6 3-9s-.5-6.5-3-9m0 18c-2.5-2.5-3-6-3-9s.5-6.5 3-9M3.6 9h16.8M3.6 15h16.8',
  laravel:   'M12 3l8 4v10l-8 4-8-4V7l8-4zm0 2.2L6 8.2v7.6l6 3 6-3V8.2l-6-3zM8 9h2v5h4v2H8V9z',
  subdomain: 'M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064',
  addonDomain: 'M4 5a2 2 0 012-2h5l2 2h5a2 2 0 012 2v3M4 5v12a2 2 0 002 2h5m9-9v7a2 2 0 01-2 2h-5m0 0l3-3m-3 3l3 3',
  dns:       'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01',
  redis:     'M13 10V3L4 14h7v7l9-11h-7z',
}

function Group({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mb-5 last:mb-0">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-500 mb-2">{title}</h3>
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2.5">{children}</div>
    </section>
  )
}

export default function DomainDashboard({ domain }: { domain: Domain }) {
  const navigate = useNavigate()
  const navigateTo = (slug: string) => () => navigate(`/subscriptions/${domain.id}/${slug}`)
  return (
    <div>
      <Group title="Applications">
        <ToolCard label="WordPress" description="One-click installation · management" icon={ICONS.wordpress} color="sky" onClick={navigateTo('wordpress')} />
        <ToolCard label="Laravel Toolkit" description="Install · deploy · queue" icon={ICONS.laravel} color="emerald" onClick={navigateTo('laravel')} />
      </Group>

      <Group title="Domain and DNS">
        <ToolCard label="DNS Management"          description="A, MX, TXT, CNAME records" icon={ICONS.dns}       color="sky"  onClick={navigateTo('dns')} />
        <ToolCard label="Subdomains"          description="Subdomains"   icon={ICONS.subdomain} color="teal" onClick={navigateTo('subdomains')} />
        <ToolCard label="Addon Domains" description="Addon and parked domains · redirects" icon={ICONS.addonDomain} color="indigo" onClick={navigateTo('addon-domains')} />
      </Group>

      <Group title="Files and Databases">
        <ToolCard label="Connection Information"      description="FTP, database"  icon={ICONS.connection} color="emerald" onClick={navigateTo('connection')} />
        <ToolCard label="Files"              description="File manager"  icon={ICONS.files} color="amber"   phase="F6"  onClick={navigateTo('files')} />
        <ToolCard label="Databases"         description={domain.db_name}     icon={ICONS.db}       color="violet"  phase="F5"  onClick={navigateTo('databases')} />
        <ToolCard label="FTP"                   description="FTP accounts"     icon={ICONS.ftp}      color="sky"     phase="F4"  onClick={navigateTo('ftp')} />
        <ToolCard label="Backup and Restore" description="Backup management"    icon={ICONS.backup}    color="rose"    phase="F12" onClick={navigateTo('backups')} />
        <ToolCard label="Copy Website"  description="Cloning"          icon={ICONS.copy}    color="sky"     onClick={navigateTo('copy')} />
      </Group>

      <Group title="Development Tools">
        <ToolCard label="PHP"                   description={`Version ${domain.php_version}`} icon={ICONS.php}      color="indigo" phase="F3" onClick={navigateTo('php')} />
        <ToolCard label="Logs"             description="access, error"  icon={ICONS.log}      color="slate"  phase="F10" onClick={navigateTo('logs')} />
        <ToolCard label="Scheduled Tasks"  description="Cron"            icon={ICONS.cron}     color="teal"   phase="F8"  onClick={navigateTo('cron')} />
        <ToolCard label="Git"                   description="Repository integration" icon={ICONS.navigateTo}    color="orange" phase="F9"  onClick={navigateTo('git')} />
        <ToolCard label="PHP Composer"          description="Package manager"  icon={ICONS.composer} color="amber" phase="F3"  onClick={navigateTo('composer')} />
        <ToolCard label="Performance"            description="Accelerators"   icon={ICONS.service} color="emerald" onClick={navigateTo('performance')} />
        <ToolCard label="Redis Cache"           description="Isolated object cache · accelerator" icon={ICONS.redis} color="rose" onClick={navigateTo('redis')} />
      </Group>

      <Group title="Security">
        <ToolCard
          label="SSL/TLS Certificates"
          description={domain.ssl ? `Expires: ${domain.ssl_expiry || '—'}` : 'Let’s Encrypt'}
          icon={ICONS.ssl}
          color={domain.ssl ? 'emerald' : 'rose'}
          phase="F7"
          warning={!domain.ssl ? 'Domain is not protected' : undefined}
          onClick={navigateTo('ssl')}
        />
        <ToolCard label="Password-Protected Directories" description=".htpasswd"       icon={ICONS.lock}      color="amber" phase="F7" onClick={navigateTo('password-protection')} />
        <ToolCard label="Statistics"            description="Traffic analysis"  icon={ICONS.stats} color="indigo" phase="F10" onClick={navigateTo('stats')} />
        <ToolCard label="Imunify"                  description="Antivirus"        icon={ICONS.imunify}    color="emerald" onClick={navigateTo('imunify')} />
        <ToolCard
          label="SSH Access"
          description={domain.ssh_access ? 'Enabled' : 'Disabled'}
          icon={ICONS.ssh}
          color={domain.ssh_access ? 'emerald' : 'slate'}
          onClick={navigateTo('ssh-access')}
        />
      </Group>
    </div>
  )
}