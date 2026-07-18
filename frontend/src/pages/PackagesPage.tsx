import { useEffect, useState } from 'react'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Package = {
  name: string; version?: string; description?: string;
  installed: boolean; protected: boolean
}

type Group = { name: string; packages: string[]; description: string }

type Tab = 'search' | 'installed'

const PRESET_GROUPS: Group[] = [
  { name: '🛠️ Development Tools', description: 'gcc, make, autoconf, automake, libtool, kernel-devel',
    packages: ['gcc', 'gcc-c++', 'make', 'autoconf', 'automake', 'libtool', 'kernel-devel'] },
  { name: '🐍 Python', description: 'Python 3 + pip + venv + development headers',
    packages: ['python3', 'python3-pip', 'python3-devel', 'python3-virtualenv'] },
  { name: '⚛️ Node.js + npm', description: 'Node.js LTS + npm',
    packages: ['nodejs', 'npm'] },
  { name: '🟦 Go', description: 'Go compiler',
    packages: ['golang'] },
  { name: '☕ Java', description: 'OpenJDK 21 LTS + Maven',
    packages: ['java-21-openjdk', 'java-21-openjdk-devel', 'maven'] },
  { name: '🦀 Rust', description: 'Rust + cargo',
    packages: ['rust', 'cargo'] },
  { name: '📦 Container/VM', description: 'Docker-compatible tools: podman + buildah + skopeo',
    packages: ['podman', 'buildah', 'skopeo'] },
  { name: '🔧 System Tools', description: 'CLI productivity tools',
    packages: ['htop', 'ncdu', 'jq', 'tmux', 'vim-enhanced', 'git', 'rsync', 'mtr', 'iftop', 'iotop'] },
  { name: '🖼️ Image Processing', description: 'ImageMagick + WebP + optimization',
    packages: ['ImageMagick', 'libwebp-tools', 'optipng', 'jpegoptim'] },
  { name: '🗄️ Database Clients', description: 'PostgreSQL + Redis CLI',
    packages: ['postgresql', 'redis'] },
  { name: '🔐 Security', description: 'GnuPG, OpenSSL, fail2ban',
    packages: ['gnupg2', 'openssl', 'fail2ban'] },
]

export default function PackagesPage() {
  const [tab, setTab] = useState<Tab>('search')
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Package[]>([])
  const [searched, setSearched] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [processing, setProcessing] = useState<string | null>(null)
  const [outputModal, setOutputModal] = useState<{ title: string; output: string } | null>(null)
  const [openGroup, setOpenGroup] = useState<string | null>(null)
  const [groupStatus, setGroupStatus] = useState<Record<string, boolean>>({})

  async function loadGroupStatus(group: Group) {
    try {
      const response = await api.get<Record<string, boolean>>('/packages/status', {
        params: { names: group.packages.join(',') }
      })
      setGroupStatus(previous => ({ ...previous, ...response.data }))
    } catch {
      // Ignore status failures so the group expansion state remains usable.
    }
  }

  function toggleGroup(group: Group) {
    if (openGroup === group.name) {
      setOpenGroup(null)
    } else {
      setOpenGroup(group.name)
      loadGroupStatus(group)
    }
  }

  async function togglePackage(pkg: string, currentlyInstalled: boolean) {
    const action = currentlyInstalled ? 'remove' : 'install'
    const confirmationMessage = currentlyInstalled
      ? `Package "${pkg}" will be REMOVED. Continue?`
      : `Package "${pkg}" will be installed on the server. Continue?`
    if (!confirm(confirmationMessage)) return

    setProcessing(pkg); setError(null); setSuccess(null)
    try {
      const response = await api.post(`/packages/${action}`, { package: pkg })
      setSuccess(`✓ ${pkg} ${currentlyInstalled ? 'removed' : 'installed'}`)
      setGroupStatus(previous => ({ ...previous, [pkg]: !currentlyInstalled }))
      setOutputModal({
        title: `${currentlyInstalled ? 'Removal' : 'Installation'} output: ${pkg}`,
        output: response.data.output || ''
      })
      setTimeout(() => setSuccess(null), 3500)
    } catch (e) {
      setError(apiError(e, `Failed to ${currentlyInstalled ? 'remove' : 'install'} package`))
    } finally {
      setProcessing(null)
    }
  }

  async function search() {
    if (!query.trim()) return
    setLoading(true); setError(null); setSearched(true)
    try {
      const endpoint = tab === 'search' ? '/packages' : '/packages/installed'
      const response = await api.get<{ content: Package[]; total: number }>(endpoint, { params: { q: query } })
      setResults(response.data.content || [])
    } catch (e) {
      setError(apiError(e, 'Search failed'))
    } finally {
      setLoading(false)
    }
  }

  async function installPackage(pkg: string) {
    if (!confirm(`Package "${pkg}" will be installed system-wide. Continue?`)) return
    setProcessing(pkg); setError(null); setSuccess(null)
    try {
      const response = await api.post('/packages/install', { package: pkg })
      setSuccess(`✓ ${pkg} installed`)
      setOutputModal({ title: `Installation output: ${pkg}`, output: response.data.output || '' })
      setTimeout(() => setSuccess(null), 4000)
      if (tab === 'search') search()
    } catch (e) { setError(apiError(e, 'Installation failed')) }
    finally { setProcessing(null) }
  }
  async function removePackage(pkg: string) {
    if (!confirm(`Package "${pkg}" will be REMOVED. Continue?`)) return
    setProcessing(pkg); setError(null); setSuccess(null)
    try {
      const response = await api.post('/packages/remove', { package: pkg })
      setSuccess(`✓ ${pkg} removed`)
      setOutputModal({ title: `Removal output: ${pkg}`, output: response.data.output || '' })
      setTimeout(() => setSuccess(null), 4000)
      search()
    } catch (e) { setError(apiError(e, 'Removal failed')) }
    finally { setProcessing(null) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Tools and Settings' },
        { label: 'Package Manager' },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Package Manager · Compilers</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        Server packages managed through DNF. Critical packages (kernel, bash, openssh, nginx, mariadb…) are <strong>protected</strong>.
      </p>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {/* Group cards as an accordion */}
      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5">
        <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-1">📦 Quick Installation Groups</h3>
        <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">Select a group to manage its packages individually.</p>
        <div className="space-y-2">
          {PRESET_GROUPS.map(group => {
            const isOpen = openGroup === group.name
            const installedCount = group.packages.filter(pkg => groupStatus[pkg]).length
            return (
              <div key={group.name} className="border border-slate-200 dark:border-slate-700 rounded-lg overflow-hidden">
                <button onClick={() => toggleGroup(group)}
                  className="w-full text-left px-3 py-2.5 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 flex items-center justify-between transition">
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{group.name}</div>
                    <div className="text-[11px] text-slate-500 dark:text-slate-500">{group.description}</div>
                  </div>
                  <div className="flex items-center gap-3 flex-shrink-0">
                    {isOpen && (
                      <span className="text-[11px] text-slate-500 dark:text-slate-500">
                        <span className="font-semibold text-emerald-700 dark:text-emerald-300">{installedCount}</span>
                        <span> / {group.packages.length} installed</span>
                      </span>
                    )}
                    <svg className={`w-4 h-4 text-slate-400 dark:text-slate-500 transition-transform ${isOpen ? 'rotate-180' : ''}`}
                      fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                    </svg>
                  </div>
                </button>
                {isOpen && (
                  <div className="border-t border-slate-100 dark:border-slate-800 bg-slate-50 dark:bg-slate-900/50 px-3 py-2 space-y-1">
                    {group.packages.map(p => {
                      const isInstalled = !!groupStatus[p]
                      const pending = processing === p
                      return (
                        <div key={p} className="flex items-center justify-between gap-3 px-2 py-1.5 rounded hover:bg-white dark:bg-slate-800 transition">
                          <div className="flex-1 min-w-0">
                            <code className="text-sm font-mono text-slate-900 dark:text-slate-100">{p}</code>
                            {isInstalled && <span className="ml-2 text-[10px] px-1.5 py-0.5 rounded bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 font-medium">INSTALLED</span>}
                          </div>
                          <button onClick={() => togglePackage(p, isInstalled)}
                            disabled={pending}
                            className={`relative inline-flex h-5 w-9 items-center rounded-full transition flex-shrink-0 ${
                              isInstalled ? 'bg-emerald-500' : 'bg-slate-300'
                            } ${pending ? 'opacity-50 cursor-wait' : ''}`}
                            title={pending ? 'Processing…' : (isInstalled ? 'Remove' : 'Install')}>
                            <span className={`inline-block h-3 w-3 transform rounded-full bg-white dark:bg-slate-800 shadow transition ${isInstalled ? 'translate-x-5' : 'translate-x-1'}`} />
                          </button>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>

      {/* Search */}
      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <div className="flex items-center gap-2 mb-3 border-b border-slate-100 dark:border-slate-800 pb-2">
          <button onClick={() => { setTab('search'); setResults([]); setSearched(false) }}
            className={`px-3 py-1.5 text-sm rounded ${tab === 'search' ? 'bg-brand-50 dark:bg-brand-900/20 text-brand-700 dark:text-brand-300 font-medium' : 'text-slate-600 dark:text-slate-400 dark:text-slate-500 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800'}`}>
            🔍 Search Repositories
          </button>
          <button onClick={() => { setTab('installed'); setResults([]); setSearched(false) }}
            className={`px-3 py-1.5 text-sm rounded ${tab === 'installed' ? 'bg-brand-50 dark:bg-brand-900/20 text-brand-700 dark:text-brand-300 font-medium' : 'text-slate-600 dark:text-slate-400 dark:text-slate-500 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800'}`}>
            📦 Installed Packages
          </button>
        </div>

        <div className="flex gap-2 mb-4">
          <input type="text" value={query} onChange={e => setQuery(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && search()}
            placeholder={tab === 'search' ? 'e.g. mongodb, redis, nodejs, gcc, htop' : 'installed package name or description'}
            className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 outline-none" />
          <button onClick={search} disabled={loading || !query.trim()}
            className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded">
            {loading ? 'Searching…' : 'Search'}
          </button>
        </div>

        {searched && !loading && results.length === 0 && (
          <div className="py-8 text-center text-sm text-slate-400 dark:text-slate-500">No results.</div>
        )}

        {results.length > 0 && (
          <div className="space-y-1.5">
            <div className="text-xs text-slate-500 dark:text-slate-500 mb-2">{results.length} results</div>
            {results.map(p => (
              <div key={p.name}
                className={`flex items-center gap-3 px-3 py-2 rounded border ${p.installed ? 'bg-emerald-50 dark:bg-emerald-900/20 border-emerald-200 dark:border-emerald-800' : 'bg-slate-50 dark:bg-slate-900 border-slate-200 dark:border-slate-700'}`}>
                <div className="flex-1 min-w-0">
                  <div className="flex items-baseline gap-2">
                    <span className="font-mono text-sm font-semibold text-slate-900 dark:text-slate-100">{p.name}</span>
                    {p.version && <span className="text-[10px] font-mono text-slate-500 dark:text-slate-500">{p.version}</span>}
                    {p.installed && <span className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 font-medium">INSTALLED</span>}
                    {p.protected && <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 font-medium">PROTECTED</span>}
                  </div>
                  {p.description && <div className="text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500 truncate">{p.description}</div>}
                </div>
                {p.installed ? (
                  <button onClick={() => removePackage(p.name)}
                    disabled={p.protected || processing === p.name}
                    className="text-xs px-3 py-1.5 bg-red-600 hover:bg-red-700 disabled:bg-slate-300 disabled:cursor-not-allowed text-white rounded">
                    {processing === p.name ? 'Removing…' : 'Remove'}
                  </button>
                ) : (
                  <button onClick={() => installPackage(p.name)}
                    disabled={processing === p.name}
                    className="text-xs px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 rounded">
                    {processing === p.name ? 'Installing…' : 'Install'}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {outputModal && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => setOutputModal(null)}>
          <div className="bg-white dark:bg-slate-800 rounded-2xl w-full shadow-xl flex flex-col max-h-[80vh]" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between px-4 py-3 border-b border-slate-200 dark:border-slate-700">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{outputModal.title}</h3>
              <button onClick={() => setOutputModal(null)} className="text-slate-400 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300">×</button>
            </div>
            <pre className="flex-1 overflow-auto p-3 bg-slate-900 text-slate-100 text-xs font-mono whitespace-pre-wrap">{outputModal.output}</pre>
            <div className="px-4 py-2 border-t border-slate-200 dark:border-slate-700 text-right">
              <button onClick={() => setOutputModal(null)}
                className="px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded">Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}