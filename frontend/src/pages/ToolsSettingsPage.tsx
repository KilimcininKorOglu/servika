import { Link } from 'react-router-dom'
import Breadcrumb from '@/components/Breadcrumb'
import PanelUpdate from '@/components/PanelUpdate'

type Tool = {
  title: string
  description: string
  href: string
  icon: string
  color: string
  ready: boolean
  badge?: string
}

const GROUPS: { name: string; tools: Tool[] }[] = [
  {
    name: 'PHP Management',
    tools: [
      { title: 'PHP Versions', description: 'Add or remove versions 5.6 through 8.5. Each domain selects its version independently.',
        href: '/tools/php-versions', icon: '🐘', color: 'indigo', ready: true, badge: 'Dynamic' },
      { title: 'PHP Extensions', description: 'Toggle server-wide extensions, search PECL packages, and compile them.',
        href: '/system/php-modules', icon: '🧩', color: 'violet', ready: true },
    ],
  },
  {
    name: 'Server',
    tools: [
      { title: 'Package Manager · Compilers',
        description: 'Manage system packages such as GCC, Python, Node.js, Go, and Podman through DNF, with quick-install groups.',
        href: '/tools/packages', icon: '📦', color: 'orange', ready: true },
      { title: 'Service Plans', description: 'Hosting packages with disk, database, and FTP quotas.',
        href: '/service-plans', icon: '📋', color: 'sky', ready: true },
      { title: 'Services', description: 'Restart or reload Nginx, Apache, MariaDB, DNS, PHP-FPM, and other managed services.',
        href: '/tools/services', icon: '⚙️', color: 'rose', ready: true },
      { title: 'DNS Template', description: 'Edit the server-wide DNS records, SOA values, and DKIM settings applied to new domains.',
        href: '/tools/dns-template', icon: '🌍', color: 'emerald', ready: true, badge: 'Server-wide' },
    ],
  },
  {
    name: 'Hosting',
    tools: [
      { title: 'Domains', description: 'Browse, search, and quickly access all domains.',
        href: '/domains', icon: '🌐', color: 'teal', ready: true },
    ],
  },
  {
    name: 'Security and Backups',
    tools: [
      { title: 'Firewall', description: 'Block IP addresses and ports, manage the allowlist, and close ports with nftables. Critical ports are protected.',
        href: '/firewall', icon: '🛡️', color: 'rose', ready: true },
      { title: 'Backup Manager', description: 'View backup sizes for every domain, create backups with one click, and configure S3 or SFTP destinations.',
        href: '/backup-management', icon: '💾', color: 'sky', ready: true },
      { title: 'Monitoring and Logs', description: 'CPU, RAM, and disk charts with server logs from journald for the panel, nginx, SSH, and more.',
        href: '/monitoring', icon: '📊', color: 'emerald', ready: true },
    ],
  },
]

const COLOR_MAP: Record<string, { bg: string; icon: string; badge: string }> = {
  indigo:  { bg: 'bg-indigo-50 dark:bg-indigo-900/15 hover:bg-indigo-100 dark:hover:bg-indigo-900/25 border-indigo-200 dark:border-indigo-800/50', icon: 'bg-indigo-100 dark:bg-indigo-900/40', badge: 'bg-indigo-100 dark:bg-indigo-900/40 text-indigo-700 dark:text-indigo-300' },
  violet:  { bg: 'bg-violet-50 dark:bg-violet-900/15 hover:bg-violet-100 dark:hover:bg-violet-900/25 border-violet-200 dark:border-violet-800/50', icon: 'bg-violet-100 dark:bg-violet-900/40', badge: 'bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300' },
  orange:  { bg: 'bg-orange-50 dark:bg-orange-900/15 hover:bg-orange-100 dark:hover:bg-orange-900/25 border-orange-200 dark:border-orange-800/50', icon: 'bg-orange-100 dark:bg-orange-900/40', badge: 'bg-orange-100 dark:bg-orange-900/40 text-orange-700 dark:text-orange-300' },
  sky:     { bg: 'bg-sky-50 dark:bg-sky-900/15 hover:bg-sky-100 dark:hover:bg-sky-900/25 border-sky-200 dark:border-sky-800/50', icon: 'bg-sky-100 dark:bg-sky-900/40', badge: 'bg-sky-100 dark:bg-sky-900/40 text-sky-700 dark:text-sky-300' },
  emerald: { bg: 'bg-emerald-50 dark:bg-emerald-900/15 hover:bg-emerald-100 dark:hover:bg-emerald-900/25 border-emerald-200 dark:border-emerald-800/50', icon: 'bg-emerald-100 dark:bg-emerald-900/40', badge: 'bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300' },
  amber:   { bg: 'bg-amber-50 dark:bg-amber-900/15 hover:bg-amber-100 dark:hover:bg-amber-900/25 border-amber-200 dark:border-amber-800/50', icon: 'bg-amber-100 dark:bg-amber-900/40', badge: 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300' },
  teal:    { bg: 'bg-teal-50 dark:bg-teal-900/15 hover:bg-teal-100 dark:hover:bg-teal-900/25 border-teal-200 dark:border-teal-800/50', icon: 'bg-teal-100 dark:bg-teal-900/40', badge: 'bg-teal-100 dark:bg-teal-900/40 text-teal-700 dark:text-teal-300' },
  slate:   { bg: 'bg-slate-50 dark:bg-slate-800/40 hover:bg-slate-100 dark:hover:bg-slate-800 border-slate-200 dark:border-slate-700', icon: 'bg-slate-100 dark:bg-slate-700', badge: 'bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-300' },
  rose:    { bg: 'bg-rose-50 dark:bg-rose-900/15 hover:bg-rose-100 dark:hover:bg-rose-900/25 border-rose-200 dark:border-rose-800/50', icon: 'bg-rose-100 dark:bg-rose-900/40', badge: 'bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300' },
}

export default function ToolsSettingsPage() {
  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Tools and Settings' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Tools and Settings</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">
        Server-wide management tools for system packages, PHP versions, security, and maintenance.
      </p>

      <PanelUpdate />

      {GROUPS.map(group => (
        <div key={group.name} className="mb-7">
          <h2 className="text-sm font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-500 mb-3">{group.name}</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
            {group.tools.map(tool => {
              const color = COLOR_MAP[tool.color]
              const Component = tool.ready ? Link : 'div'
              return (
                <Component
                  key={tool.title}
                  to={tool.href}
                  className={`relative flex items-start gap-3 p-4 border rounded-2xl transition ${color.bg} ${tool.ready ? 'cursor-pointer' : 'cursor-not-allowed opacity-70'}`}
                >
                  <div className={`w-10 h-10 rounded-lg flex items-center justify-center text-xl flex-shrink-0 ${color.icon}`}>
                    {tool.icon}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-baseline gap-2">
                      <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{tool.title}</span>
                      {tool.badge && (
                        <span className={`text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium ${color.badge}`}>
                          {tool.badge}
                        </span>
                      )}
                    </div>
                    <div className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">{tool.description}</div>
                  </div>
                  {tool.ready && (
                    <svg className="w-4 h-4 text-slate-400 dark:text-slate-500 flex-shrink-0 mt-1" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                    </svg>
                  )}
                </Component>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  )
}