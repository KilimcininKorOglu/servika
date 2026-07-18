import { useEffect, useState } from 'react'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Version = { version: string; ini_dir: string; service: string }
type Extension = { name: string; active: boolean; ini_file: string }

const REQUIRED_EXTENSIONS = new Set([
  'core', 'date', 'standard', 'pdo', 'mysqlnd', 'phar', 'spl', 'reflection',
  'session', 'pcre', 'tokenizer', 'json', 'hash', 'random', 'libxml',
])

export default function PHPExtensionsPage() {
  const [versions, setVersions] = useState<Version[]>([])
  const [activeVersion, setActiveVersion] = useState('8.3')
  const [extensions, setExtensions] = useState<Extension[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const [peclModalOpen, setPeclModalOpen] = useState(false)

  function load() {
    setLoading(true); setError(null)
    api.get(`/php-extensions?version=${activeVersion}`)
      .then(response => {
        setExtensions(response.data.content || [])
        setVersions(response.data.versions || [])
      })
      .catch(error => setError(apiError(error)))
      .finally(() => setLoading(false))
  }
  useEffect(load, [activeVersion])

  async function toggle(extension: Extension) {
    if (REQUIRED_EXTENSIONS.has(extension.name.toLowerCase())) {
      alert('This extension is a core part of PHP and cannot be disabled.')
      return
    }
    const active = !extension.active
    try {
      await api.put('/php-extensions/toggle', {
        version: activeVersion,
        ini_file: extension.ini_file,
        active,
      })
      setSuccess(`✓ ${extension.name} ${active ? 'enabled' : 'disabled'} · PHP-FPM restarted`)
      setTimeout(() => setSuccess(null), 3000)
      load()
    } catch (error) {
      setError(apiError(error, 'Toggle failed'))
    }
  }

  async function installIonCube() {
    if (!confirm(`IonCube Loader will be installed for PHP ${activeVersion}.\n\nA tar.gz archive will be downloaded from ioncube.com, the .so file will be copied, and it will be loaded as a zend_extension.\nContinue?`)) return
    setLoading(true); setError(null)
    try {
      const response = await api.post('/php-extensions/ioncube-install', { version: activeVersion })
      const data = response.data
      setSuccess(`✓ IonCube installed · ${data.loaded ? 'LOADED' : 'The INI file was written, but the extension was not detected at runtime'}`)
      setTimeout(() => setSuccess(null), 5000)
      load()
    } catch (error) {
      setError(apiError(error, 'IonCube installation failed'))
      setLoading(false)
    }
  }

  async function removeIonCube() {
    if (!confirm(`IonCube Loader will be removed from PHP ${activeVersion}. Continue?`)) return
    setLoading(true); setError(null)
    try {
      await api.post('/php-extensions/ioncube-remove', { version: activeVersion })
      setSuccess('✓ IonCube removed')
      setTimeout(() => setSuccess(null), 3000)
      load()
    } catch (error) {
      setError(apiError(error, 'IonCube removal failed'))
      setLoading(false)
    }
  }

  async function installPecl(packageName: string) {
    if (!packageName.match(/^[a-zA-Z0-9_-]+$/)) {
      alert('Invalid package name'); return
    }
    if (!confirm(`The PECL package "${packageName}" will be compiled and installed for PHP ${activeVersion}. Continue?`)) return
    setPeclModalOpen(false); setLoading(true)
    try {
      const response = await api.post('/php-extensions/pecl-install', { version: activeVersion, package: packageName })
      setSuccess(`✓ ${packageName} installed`)
      console.log('PECL install output:', response.data.output)
      load()
    } catch (error) {
      setError(apiError(error, 'PECL installation failed'))
      setLoading(false)
    }
  }

  const filtered = filter ? extensions.filter(extension => extension.name.toLowerCase().includes(filter.toLowerCase())) : extensions
  const activeCount = extensions.filter(extension => extension.active).length
  const inactiveCount = extensions.length - activeCount

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'System Management' },
        { label: 'PHP Extensions' },
      ]} />

      <div className="flex items-center justify-between mb-1">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">PHP Extensions</h1>
        <div className="flex gap-2">
          <button onClick={() => {
              const ionCubeInstalled = extensions.some(extension => extension.name.toLowerCase().includes('ioncube'))
              if (ionCubeInstalled) removeIonCube(); else installIonCube()
            }}
            className="px-4 py-2 bg-amber-600 hover:bg-amber-700 text-white text-sm rounded-md">
            {extensions.some(extension => extension.name.toLowerCase().includes('ioncube')) ? '⊗ Remove IonCube' : '🔐 Install IonCube'}
          </button>
          <button onClick={() => setPeclModalOpen(true)}
            className="px-4 py-2 bg-slate-700 hover:bg-slate-800 text-white text-sm rounded-md">
            📦 Install from PECL
          </button>
        </div>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        Manage PHP extensions across the server. Use the toggle to enable or disable an extension, and PHP-FPM restarts automatically. <strong>Server-wide</strong>, affects all domains.
      </p>

      {/* Version tabs */}
      <div className="flex gap-2 mb-4 border-b border-slate-200 dark:border-slate-700">
        {versions.map(version => (
          <button key={version.version} onClick={() => setActiveVersion(version.version)}
            className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition ${
              activeVersion === version.version
                ? 'border-brand-500 text-brand-700 dark:text-brand-300'
                : 'border-transparent text-slate-500 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300'
            }`}>
            PHP {version.version}
          </button>
        ))}
      </div>

      {/* Toolbar with counters and search */}
      <div className="flex items-center justify-between mb-4 gap-3">
        <div className="flex items-center gap-3 text-sm">
          <span className="px-2.5 py-0.5 rounded-full bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 font-medium text-xs">
            {activeCount} enabled
          </span>
          <span className="px-2.5 py-0.5 rounded-full bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400 dark:text-slate-500 font-medium text-xs">
            {inactiveCount} inactive
          </span>
          <span className="text-slate-400 dark:text-slate-500 text-xs">Total {extensions.length}</span>
        </div>
        <input
          type="text"
          value={filter}
          onChange={event => setFilter(event.target.value)}
          placeholder="🔍 Search extensions..."
          className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm w-64 focus:border-brand-500 outline-none"
        />
      </div>

      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      {loading ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Loading…</div> : (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2">
          {filtered.map(extension => {
            const required = REQUIRED_EXTENSIONS.has(extension.name.toLowerCase())
            return (
              <div key={extension.ini_file}
                className={`flex items-center justify-between gap-2 px-3 py-2 rounded-md border ${
                  extension.active
                    ? 'bg-emerald-50 dark:bg-emerald-900/20 border-emerald-200 dark:border-emerald-800'
                    : 'bg-slate-50 dark:bg-slate-900 border-slate-200 dark:border-slate-700'
                }`}>
                <div className="min-w-0 flex-1">
                  <div className="font-mono text-sm font-semibold text-slate-900 dark:text-slate-100 truncate">{extension.name}</div>
                  {required && <div className="text-[10px] text-slate-500 dark:text-slate-500">core extension</div>}
                </div>
                <button
                  onClick={() => toggle(extension)}
                  disabled={required}
                  className={`flex-shrink-0 relative inline-flex h-5 w-9 items-center rounded-full transition ${
                    extension.active ? 'bg-emerald-500' : 'bg-slate-300'
                  } ${required ? 'opacity-40 cursor-not-allowed' : ''}`}
                  title={required ? 'Core extension, cannot be disabled' : (extension.active ? 'Disable' : 'Enable')}
                >
                  <span className={`inline-block h-3 w-3 transform rounded-full bg-white dark:bg-slate-800 shadow transition ${extension.active ? 'translate-x-5' : 'translate-x-1'}`} />
                </button>
              </div>
            )
          })}
        </div>
      )}

      {peclModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => setPeclModalOpen(false)}>
          <div className="bg-white dark:bg-slate-800 rounded-2xl w-full max-w-md p-5 shadow-xl" onClick={event => event.stopPropagation()}>
            <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-2">Install an Extension from PECL</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-3">Downloads and compiles an extension from the PECL repository. Examples: <code className="font-mono">mongodb, swoole, geoip, oauth, yaml, msgpack</code></p>
            <p className="text-xs text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded p-2 mb-3">
              ⚠ The extension will be compiled for PHP {activeVersion}. Target: <code className="font-mono">/etc/php.d/</code> or the Remi directory
            </p>
            <input id="peclPackageName" type="text" autoFocus placeholder="e.g. mongodb"
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded font-mono text-sm mb-3"
              onKeyDown={event => {
                if (event.key === 'Enter') {
                  const value = (event.target as HTMLInputElement).value.trim()
                  if (value) installPecl(value)
                }
              }} />
            <div className="flex justify-end gap-2">
              <button onClick={() => setPeclModalOpen(false)}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-sm rounded">Cancel</button>
              <button onClick={() => {
                const value = (document.getElementById('peclPackageName') as HTMLInputElement)?.value?.trim()
                if (value) installPecl(value)
              }} className="px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded">Install</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}