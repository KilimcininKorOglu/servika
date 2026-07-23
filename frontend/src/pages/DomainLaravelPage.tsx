import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
import Breadcrumb from '@/components/Breadcrumb'
import { api, apiError } from '@/lib/api'

type Status = {
  installed: boolean
  exists: boolean
  app_root: string
  system_user: string
  directory: string
  php_version: string
  node_version: string
  composer_json: boolean
  git_present: boolean
  last_commit: string
  maintenance: boolean
  schedule_enabled: boolean
  queue_enabled: boolean
  queue_timeout: number
  queue_max_jobs: number
  queue_connection: string
  last_deploy_status: string
  php_binary: string
}

type NodeVersions = { versions: string[] }
type AppCandidates = { current: string; candidates: string[] }
type OperationStatus = { running: boolean; status: string; log: string }

type Tab = 'overview' | 'install' | 'commands' | 'env' | 'deploy' | 'workers'
type InstallMode = 'remote' | 'scaffold' | 'local'
type ActionResult = { data?: { output?: string; log?: string } }

const fieldClass = 'w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm text-slate-900 dark:text-slate-100 focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none'

const tabs: Array<{ key: Tab; label: string }> = [
  { key: 'overview', label: 'Overview' },
  { key: 'install', label: 'Install' },
  { key: 'commands', label: 'Commands' },
  { key: 'env', label: '.env' },
  { key: 'deploy', label: 'Deploy' },
  { key: 'workers', label: 'Workers' },
]

export default function DomainLaravelPage() {
  const { id } = useParams()
  const [active, setActive] = useState<Tab>('overview')
  const [status, setStatus] = useState<Status | null>(null)
  const [nodeVersions, setNodeVersions] = useState<string[]>([])
  const [candidates, setCandidates] = useState<AppCandidates | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [output, setOutput] = useState('')
  const [running, setRunning] = useState<string | null>(null)
  const [installMode, setInstallMode] = useState<InstallMode>('remote')
  const [repoURL, setRepoURL] = useState('')
  const [branch, setBranch] = useState('main')
  const [appRoot, setAppRoot] = useState('public_html')
  const [artisanCommand, setArtisanCommand] = useState('about')
  const [composerCommand, setComposerCommand] = useState('install')
  const [composerPackage, setComposerPackage] = useState('')
  const [npmCommand, setNpmCommand] = useState('install')
  const [npmScript, setNpmScript] = useState('build')
  const [nodeVersion, setNodeVersion] = useState('system')
  const [envContent, setEnvContent] = useState('')
  const [envLoaded, setEnvLoaded] = useState(false)
  const [queueTimeout, setQueueTimeout] = useState(60)
  const [queueMaxJobs, setQueueMaxJobs] = useState(1000)
  const [queueConnection, setQueueConnection] = useState('database')

  const canPoll = useMemo(() => status?.last_deploy_status === 'installing' || status?.last_deploy_status === 'running', [status])

  function load() {
    if (!id) return
    setLoading(true); setError(null)
    Promise.all([
      api.get<Status>(`/domains/${id}/laravel`),
      api.get<NodeVersions>(`/domains/${id}/laravel/node`),
      api.get<AppCandidates>(`/domains/${id}/laravel/app-candidates`),
    ]).then(([statusResponse, nodeResponse, candidateResponse]) => {
      setStatus(statusResponse.data)
      setNodeVersions(nodeResponse.data.versions || [])
      setCandidates(candidateResponse.data)
      setAppRoot(statusResponse.data.app_root || candidateResponse.data.current || 'public_html')
      setNodeVersion(statusResponse.data.node_version || nodeResponse.data.versions[0] || 'system')
      setQueueTimeout(statusResponse.data.queue_timeout || 60)
      setQueueMaxJobs(statusResponse.data.queue_max_jobs || 1000)
      setQueueConnection(statusResponse.data.queue_connection || 'database')
    }).catch(error => setError(apiError(error)))
      .finally(() => setLoading(false))
  }

  useEffect(load, [id])

  useEffect(() => {
    if (!id || !canPoll) return
    const timer = window.setInterval(load, 5000)
    return () => window.clearInterval(timer)
  }, [id, canPoll])

  async function runAction(label: string, fn: () => Promise<unknown>, refresh = true) {
    setRunning(label); setError(null); setSuccess(null)
    try {
      const result = await fn()
      const data = (result as ActionResult).data
      if (data?.output) setOutput(data.output)
      if (data?.log) setOutput(data.log)
      setSuccess('Action completed')
      if (refresh) load()
    } catch (error) {
      setError(apiError(error, 'Action failed'))
    } finally {
      setRunning(null)
    }
  }

  async function startInstall() {
    await runAction('install', () => api.post(`/domains/${id}/laravel/install`, {
      mode: installMode,
      repo_url: repoURL,
      branch,
      app_root: appRoot,
    }))
  }

  async function pollInstall() {
    await runAction('install-status', () => api.get<OperationStatus>(`/domains/${id}/laravel/install/status`), false)
    load()
  }

  async function pollDeploy() {
    await runAction('deploy-status', () => api.get<OperationStatus>(`/domains/${id}/laravel/deploy/status`), false)
    load()
  }

  async function saveAppRoot(nextRoot: string) {
    setAppRoot(nextRoot)
    await runAction('app-root', () => api.put(`/domains/${id}/laravel/app-root`, { app_root: nextRoot }))
  }

  async function runArtisan() {
    await runAction('artisan', () => api.post(`/domains/${id}/laravel/artisan`, { command: artisanCommand }))
  }

  async function runComposer() {
    await runAction('composer', () => api.post(`/domains/${id}/laravel/composer`, { command: composerCommand, package: composerPackage }))
  }

  async function runNpm() {
    await runAction('npm', () => api.post(`/domains/${id}/laravel/npm`, { command: npmCommand, script: npmScript, node_version: nodeVersion }))
  }

  async function loadEnv() {
    await runAction('env-load', async () => {
      const response = await api.get<{ exists: boolean; content: string }>(`/domains/${id}/laravel/env`)
      setEnvContent(response.data.content || '')
      setEnvLoaded(true)
      return response
    }, false)
  }

  async function saveEnv() {
    await runAction('env-save', () => api.put(`/domains/${id}/laravel/env`, { content: envContent }))
  }

  async function setMaintenance(enabled: boolean) {
    await runAction('maintenance', () => api.post(`/domains/${id}/laravel/maintenance`, { enabled }))
  }

  async function startDeploy() {
    await runAction('deploy', () => api.post(`/domains/${id}/laravel/deploy`, { migrate: true, npm_build: true, node_version: nodeVersion }))
  }

  async function setSchedule(enabled: boolean) {
    await runAction('schedule', () => api.post(`/domains/${id}/laravel/schedule`, { enabled }))
  }

  async function setQueue(enabled: boolean) {
    await runAction('queue', () => api.post(`/domains/${id}/laravel/queue`, {
      enabled,
      timeout: queueTimeout,
      max_jobs: queueMaxJobs,
      connection: queueConnection,
    }))
  }

  if (loading && !status) return <div className="px-6 py-5 text-sm text-slate-400">Loading…</div>

  return (
    <div className="px-6 py-5 max-w-[1100px]">
      <Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'Domains', href: '/domains' }, { label: 'Laravel Toolkit' }]} />
      <div className="mb-5 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Laravel Toolkit</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">Install, deploy, and operate a Laravel application for this subscription.</p>
        </div>
        <Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">Back to Subscription</Link>
      </div>

      {error && <div className="mb-3 px-3 py-2 rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 rounded-lg border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/20 text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      <div className="mb-4 flex flex-wrap gap-2">
        {tabs.map(tab => (
          <button key={tab.key} onClick={() => setActive(tab.key)} className={`px-3 py-1.5 rounded-lg text-sm font-medium ${active === tab.key ? 'bg-slate-900 text-white dark:bg-white dark:text-slate-900' : 'border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300'}`}>{tab.label}</button>
        ))}
      </div>

      {active === 'overview' && status && (
        <Card title="Application Status">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 text-sm">
            <Metric label="Installed" value={status.installed ? 'Yes' : 'No'} />
            <Metric label="Application root" value={status.app_root} />
            <Metric label="Document path" value={status.directory} mono />
            <Metric label="PHP" value={`${status.php_version} (${status.php_binary})`} />
            <Metric label="Composer manifest" value={status.composer_json ? 'Found' : 'Missing'} />
            <Metric label="Git" value={status.git_present ? status.last_commit || 'Repository found' : 'Not connected'} />
            <Metric label="Maintenance" value={status.maintenance ? 'Enabled' : 'Disabled'} />
            <Metric label="Schedule" value={status.schedule_enabled ? 'Enabled' : 'Disabled'} />
            <Metric label="Queue" value={status.queue_enabled ? 'Enabled' : 'Disabled'} />
          </div>
        </Card>
      )}

      {active === 'install' && (
        <Card title="Install Laravel">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Field label="Mode"><select value={installMode} onChange={e => setInstallMode(e.target.value as InstallMode)} className={fieldClass}><option value="remote">Clone remote repository</option><option value="scaffold">Create new Laravel project</option><option value="local">Initialize empty Git repository</option></select></Field>
            <Field label="Application root"><RootSelect value={appRoot} candidates={candidates?.candidates || []} onChange={setAppRoot} onSave={saveAppRoot} /></Field>
            {installMode === 'remote' && <Field label="Repository URL"><input value={repoURL} onChange={e => setRepoURL(e.target.value)} className={fieldClass} placeholder="https://github.com/example/app.git" /></Field>}
            {installMode === 'remote' && <Field label="Branch"><input value={branch} onChange={e => setBranch(e.target.value)} className={fieldClass} placeholder="main" /></Field>}
          </div>
          <div className="mt-4 flex flex-wrap gap-2"><Button disabled={!!running} onClick={startInstall}>Start install</Button><Button variant="secondary" disabled={!!running} onClick={pollInstall}>Check install status</Button></div>
        </Card>
      )}

      {active === 'commands' && (
        <Card title="Laravel Commands">
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            <CommandBox title="Artisan" value={artisanCommand} setValue={setArtisanCommand} options={['about','migrate','migrate:status','config:cache','cache:clear','queue:restart','storage:link']} onRun={runArtisan} />
            <CommandBox title="Composer" value={composerCommand} setValue={setComposerCommand} options={['install','update','dump-autoload','validate','show','diagnose','require','remove']} onRun={runComposer}><input value={composerPackage} onChange={e => setComposerPackage(e.target.value)} className={`${fieldClass} mt-2`} placeholder="vendor/package" /></CommandBox>
            <CommandBox title="npm" value={npmCommand} setValue={setNpmCommand} options={['install','ci','run','prune','ls','outdated','audit','--version']} onRun={runNpm}><input value={npmScript} onChange={e => setNpmScript(e.target.value)} className={`${fieldClass} mt-2`} placeholder="script name" /><NodeSelect value={nodeVersion} versions={nodeVersions} onChange={setNodeVersion} /></CommandBox>
          </div>
        </Card>
      )}

      {active === 'env' && (
        <Card title="Environment File">
          {!envLoaded ? <Button disabled={!!running} onClick={loadEnv}>Load .env</Button> : <><textarea value={envContent} onChange={e => setEnvContent(e.target.value)} rows={16} className={`${fieldClass} font-mono text-xs`} /><div className="mt-3"><Button disabled={!!running} onClick={saveEnv}>Save .env</Button></div></>}
        </Card>
      )}

      {active === 'deploy' && (
        <Card title="Deploy">
          <div className="flex flex-wrap gap-2 mb-4"><NodeSelect value={nodeVersion} versions={nodeVersions} onChange={setNodeVersion} /><Button disabled={!!running} onClick={startDeploy}>Deploy with migrate and build</Button><Button variant="secondary" disabled={!!running} onClick={pollDeploy}>Check deploy status</Button><Button variant="secondary" disabled={!!running} onClick={() => setMaintenance(!status?.maintenance)}>{status?.maintenance ? 'Disable maintenance' : 'Enable maintenance'}</Button></div>
        </Card>
      )}

      {active === 'workers' && status && (
        <Card title="Schedule and Queue Worker">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-4">
            <Field label="Queue timeout"><input type="number" value={queueTimeout} onChange={e => setQueueTimeout(parseInt(e.target.value) || 60)} className={fieldClass} /></Field>
            <Field label="Max jobs"><input type="number" value={queueMaxJobs} onChange={e => setQueueMaxJobs(parseInt(e.target.value) || 1000)} className={fieldClass} /></Field>
            <Field label="Connection"><input value={queueConnection} onChange={e => setQueueConnection(e.target.value)} className={fieldClass} /></Field>
          </div>
          <div className="flex flex-wrap gap-2"><Button disabled={!!running} onClick={() => setSchedule(!status.schedule_enabled)}>{status.schedule_enabled ? 'Disable schedule' : 'Enable schedule'}</Button><Button disabled={!!running} onClick={() => setQueue(!status.queue_enabled)}>{status.queue_enabled ? 'Disable queue' : 'Enable queue'}</Button></div>
        </Card>
      )}

      {output && <pre className="mt-4 bg-slate-950 text-slate-100 rounded-2xl p-4 text-xs font-mono whitespace-pre-wrap break-words max-h-[420px] overflow-auto">{output}</pre>}
    </div>
  )
}

function Card({ title, children }: { title: string; children: ReactNode }) {
  return <section className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4"><h2 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-3">{title}</h2>{children}</section>
}

function Metric({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return <div className="rounded-xl border border-slate-100 dark:border-slate-700 p-3"><div className="text-xs text-slate-500 dark:text-slate-400 mb-1">{label}</div><div className={`text-sm text-slate-900 dark:text-slate-100 ${mono ? 'font-mono break-all' : 'font-medium'}`}>{value || '—'}</div></div>
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block text-sm"><span className="block mb-1 text-slate-600 dark:text-slate-400">{label}</span>{children}</label>
}

function Button({ children, onClick, disabled, variant = 'primary' }: { children: ReactNode; onClick: () => void; disabled?: boolean; variant?: 'primary' | 'secondary' }) {
  return <button onClick={onClick} disabled={disabled} className={`px-4 py-2 rounded-lg text-sm font-medium disabled:opacity-50 ${variant === 'primary' ? 'bg-slate-900 text-white dark:bg-white dark:text-slate-900' : 'border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300'}`}>{children}</button>
}

function RootSelect({ value, candidates, onChange, onSave }: { value: string; candidates: string[]; onChange: (value: string) => void; onSave: (value: string) => void }) {
  return <div className="flex gap-2"><input list="laravel-root-candidates" value={value} onChange={e => onChange(e.target.value)} className={fieldClass} /><datalist id="laravel-root-candidates">{candidates.map(candidate => <option key={candidate || 'public_html'} value={candidate || 'public_html'} />)}</datalist><Button variant="secondary" onClick={() => onSave(value)}>Save</Button></div>
}

function NodeSelect({ value, versions, onChange }: { value: string; versions: string[]; onChange: (value: string) => void }) {
  const list = versions.length ? versions : ['system']
  return <select value={value} onChange={e => onChange(e.target.value)} className={`${fieldClass} max-w-[180px]`}>{list.map(version => <option key={version} value={version}>{version}</option>)}</select>
}

function CommandBox({ title, value, setValue, options, onRun, children }: { title: string; value: string; setValue: (value: string) => void; options: string[]; onRun: () => void; children?: ReactNode }) {
  return <div className="rounded-xl border border-slate-200 dark:border-slate-700 p-4"><h3 className="text-sm font-semibold mb-2 text-slate-900 dark:text-slate-100">{title}</h3><select value={value} onChange={e => setValue(e.target.value)} className={fieldClass}>{options.map(option => <option key={option} value={option}>{option}</option>)}</select>{children}<div className="mt-3"><Button onClick={onRun}>Run</Button></div></div>
}
