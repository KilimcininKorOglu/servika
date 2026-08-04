import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'
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

const tabs: Tab[] = ['overview', 'install', 'commands', 'env', 'deploy', 'workers']

export default function DomainLaravelPage() {
  const { t } = useTranslation('DomainLaravelPage')
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
      setSuccess(t('messages.actionCompleted'))
      if (refresh) load()
    } catch (error) {
      setError(apiError(error, t('messages.actionFailed')))
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

  if (loading && !status) return <div className="px-6 py-5 text-sm text-slate-400">{t('loading')}</div>

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[{ label: t('breadcrumb.home'), href: '/' }, { label: t('breadcrumb.domains'), href: '/domains' }, { label: t('breadcrumb.laravel') }]} />
      <div className="mb-5 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('title')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">{t('subtitle')}</p>
        </div>
        <Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">{t('backToSubscription')}</Link>
      </div>

      {error && <div className="mb-3 px-3 py-2 rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{error}</div>}
      {success && <div className="mb-3 px-3 py-2 rounded-lg border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/20 text-sm text-emerald-700 dark:text-emerald-300">{success}</div>}

      <div className="mb-4 flex flex-wrap gap-2">
        {tabs.map(tab => (
          <button key={tab} onClick={() => setActive(tab)} className={`px-3 py-1.5 rounded-lg text-sm font-medium ${active === tab ? 'bg-slate-900 text-white dark:bg-white dark:text-slate-900' : 'border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300'}`}>{t(`tabs.${tab}`)}</button>
        ))}
      </div>

      {active === 'overview' && status && (
        <Card title={t('overview.title')}>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 text-sm">
            <Metric label={t('overview.installed')} value={status.installed ? t('overview.yes') : t('overview.no')} />
            <Metric label={t('overview.appRoot')} value={status.app_root} />
            <Metric label={t('overview.documentPath')} value={status.directory} mono />
            <Metric label={t('overview.php')} value={`${status.php_version} (${status.php_binary})`} />
            <Metric label={t('overview.composerManifest')} value={status.composer_json ? t('overview.found') : t('overview.missing')} />
            <Metric label={t('overview.git')} value={status.git_present ? status.last_commit || t('overview.repositoryFound') : t('overview.notConnected')} />
            <Metric label={t('overview.maintenance')} value={status.maintenance ? t('overview.enabled') : t('overview.disabled')} />
            <Metric label={t('overview.schedule')} value={status.schedule_enabled ? t('overview.enabled') : t('overview.disabled')} />
            <Metric label={t('overview.queue')} value={status.queue_enabled ? t('overview.enabled') : t('overview.disabled')} />
          </div>
        </Card>
      )}

      {active === 'install' && (
        <Card title={t('install.title')}>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Field label={t('install.mode')}><select value={installMode} onChange={e => setInstallMode(e.target.value as InstallMode)} className={fieldClass}><option value="remote">{t('install.modeRemote')}</option><option value="scaffold">{t('install.modeScaffold')}</option><option value="local">{t('install.modeLocal')}</option></select></Field>
            <Field label={t('install.appRoot')}><RootSelect value={appRoot} candidates={candidates?.candidates || []} onChange={setAppRoot} onSave={saveAppRoot} /></Field>
            {installMode === 'remote' && <Field label={t('install.repositoryUrl')}><input value={repoURL} onChange={e => setRepoURL(e.target.value)} className={fieldClass} placeholder={t('install.repositoryUrlPlaceholder')} /></Field>}
            {installMode === 'remote' && <Field label={t('install.branch')}><input value={branch} onChange={e => setBranch(e.target.value)} className={fieldClass} placeholder={t('install.branchPlaceholder')} /></Field>}
          </div>
          <div className="mt-4 flex flex-wrap gap-2"><Button disabled={!!running} onClick={startInstall}>{t('install.startInstall')}</Button><Button variant="secondary" disabled={!!running} onClick={pollInstall}>{t('install.checkInstallStatus')}</Button></div>
        </Card>
      )}

      {active === 'commands' && (
        <Card title={t('commands.title')}>
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            <CommandBox title="Artisan" value={artisanCommand} setValue={setArtisanCommand} options={['about','migrate','migrate:status','config:cache','cache:clear','queue:restart','storage:link']} onRun={runArtisan} />
            <CommandBox title="Composer" value={composerCommand} setValue={setComposerCommand} options={['install','update','dump-autoload','validate','show','diagnose','require','remove']} onRun={runComposer}><input value={composerPackage} onChange={e => setComposerPackage(e.target.value)} className={`${fieldClass} mt-2`} placeholder={t('commands.packagePlaceholder')} /></CommandBox>
            <CommandBox title="npm" value={npmCommand} setValue={setNpmCommand} options={['install','ci','run','prune','ls','outdated','audit','--version']} onRun={runNpm}><input value={npmScript} onChange={e => setNpmScript(e.target.value)} className={`${fieldClass} mt-2`} placeholder={t('commands.scriptPlaceholder')} /><NodeSelect value={nodeVersion} versions={nodeVersions} onChange={setNodeVersion} /></CommandBox>
          </div>
        </Card>
      )}

      {active === 'env' && (
        <Card title={t('env.title')}>
          {!envLoaded ? <Button disabled={!!running} onClick={loadEnv}>{t('env.load')}</Button> : <><textarea value={envContent} onChange={e => setEnvContent(e.target.value)} rows={16} className={`${fieldClass} font-mono text-xs`} /><div className="mt-3"><Button disabled={!!running} onClick={saveEnv}>{t('env.save')}</Button></div></>}
        </Card>
      )}

      {active === 'deploy' && (
        <Card title={t('deploy.title')}>
          <div className="flex flex-wrap gap-2 mb-4"><NodeSelect value={nodeVersion} versions={nodeVersions} onChange={setNodeVersion} /><Button disabled={!!running} onClick={startDeploy}>{t('deploy.deployWithMigrate')}</Button><Button variant="secondary" disabled={!!running} onClick={pollDeploy}>{t('deploy.checkDeployStatus')}</Button><Button variant="secondary" disabled={!!running} onClick={() => setMaintenance(!status?.maintenance)}>{status?.maintenance ? t('deploy.disableMaintenance') : t('deploy.enableMaintenance')}</Button></div>
        </Card>
      )}

      {active === 'workers' && status && (
        <Card title={t('workers.title')}>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-4">
            <Field label={t('workers.queueTimeout')}><input type="number" value={queueTimeout} onChange={e => setQueueTimeout(parseInt(e.target.value) || 60)} className={fieldClass} /></Field>
            <Field label={t('workers.maxJobs')}><input type="number" value={queueMaxJobs} onChange={e => setQueueMaxJobs(parseInt(e.target.value) || 1000)} className={fieldClass} /></Field>
            <Field label={t('workers.connection')}><input value={queueConnection} onChange={e => setQueueConnection(e.target.value)} className={fieldClass} /></Field>
          </div>
          <div className="flex flex-wrap gap-2"><Button disabled={!!running} onClick={() => setSchedule(!status.schedule_enabled)}>{status.schedule_enabled ? t('workers.disableSchedule') : t('workers.enableSchedule')}</Button><Button disabled={!!running} onClick={() => setQueue(!status.queue_enabled)}>{status.queue_enabled ? t('workers.disableQueue') : t('workers.enableQueue')}</Button></div>
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
  const { t } = useTranslation('DomainLaravelPage')
  return <div className="flex gap-2"><input list="laravel-root-candidates" value={value} onChange={e => onChange(e.target.value)} className={fieldClass} /><datalist id="laravel-root-candidates">{candidates.map(candidate => <option key={candidate || 'public_html'} value={candidate || 'public_html'} />)}</datalist><Button variant="secondary" onClick={() => onSave(value)}>{t('common.save')}</Button></div>
}

function NodeSelect({ value, versions, onChange }: { value: string; versions: string[]; onChange: (value: string) => void }) {
  const list = versions.length ? versions : ['system']
  return <select value={value} onChange={e => onChange(e.target.value)} className={`${fieldClass} max-w-[180px]`}>{list.map(version => <option key={version} value={version}>{version}</option>)}</select>
}

function CommandBox({ title, value, setValue, options, onRun, children }: { title: string; value: string; setValue: (value: string) => void; options: string[]; onRun: () => void; children?: ReactNode }) {
  const { t } = useTranslation('DomainLaravelPage')
  return <div className="rounded-xl border border-slate-200 dark:border-slate-700 p-4"><h3 className="text-sm font-semibold mb-2 text-slate-900 dark:text-slate-100">{title}</h3><select value={value} onChange={e => setValue(e.target.value)} className={fieldClass}>{options.map(option => <option key={option} value={option}>{option}</option>)}</select>{children}<div className="mt-3"><Button onClick={onRun}>{t('commands.run')}</Button></div></div>
}
