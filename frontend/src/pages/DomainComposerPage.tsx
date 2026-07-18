import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError as apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Status = { installed: boolean; version: string; composer_json: boolean; username: string; dir: string }

export default function DomainComposerPage() {
  const { id } = useParams()
  const [d, setD] = useState<Status | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [output, setOutput] = useState<string>('')
  const [runningCommand, setRunningCommand] = useState<string | null>(null)
  const [packageName, setPackageName] = useState('')

  function load() {
    if (!id) return
    setLoading(true)
    api.get<Status>(`/domains/${id}/composer`).then(r => setD(r.data)).catch(e => setError(apiError(e))).finally(() => setLoading(false))
  }
  useEffect(load, [id])

  async function run(command: string, pkt?: string) {
    setRunningCommand(command); setError(null); setOutput(`$ composer ${command}${pkt ? ' ' + pkt : ''}\n\nRunning…`)
    try {
      const { data } = await api.post(`/domains/${id}/composer`, { command, package: pkt || '' })
      setOutput(`$ composer ${command}${pkt ? ' ' + pkt : ''}\n\n${data.output || '(no output)'}\n\n${data.ok ? '✓ Completed' : '✗ Failed'}`)
      load()
    } catch (e) {
      setError(apiError(e, 'Could not run command')); setOutput('')
    } finally { setRunningCommand(null) }
  }

  if (loading) return <div className="px-6 py-5 text-slate-400">Loading…</div>
  if (!d) return <div className="px-6 py-5"><div className="text-sm text-red-600">{error || 'Not found'}</div></div>

  const btnBase = 'px-3 py-1.5 rounded-lg text-sm font-medium disabled:opacity-50'

  return (
    <div className="px-6 py-5">
      <div className="max-w-3xl mx-auto">
        <Breadcrumb items={[
          { label: 'Home', href: '/' },
          { label: 'Domains', href: '/domains' },
          { label: 'Composer' },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">PHP Composer</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          <span className="font-mono">{d.dir}</span> in <span className="font-mono">{d.username}</span> as the system user.
        </p>

        {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{error}</div>}

        {!d.installed ? (
          <div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-2xl p-5 text-sm text-amber-800 dark:text-amber-200">
            Composer is not installed on the server. An administrator must install it.
          </div>
        ) : (
          <>
            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <span className="text-xs font-mono text-slate-500">{d.version}</span>
                  <span className={`ml-2 text-xs ${d.composer_json ? 'text-emerald-600 dark:text-emerald-400' : 'text-slate-400'}`}>
                    {d.composer_json ? '✓ composer.json found' : 'composer.json not found'}
                  </span>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <button disabled={!!runningCommand} onClick={() => run('install')} className={`${btnBase} bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900`}>{runningCommand === 'install' ? '…' : 'install'}</button>
                <button disabled={!!runningCommand} onClick={() => run('update')} className={`${btnBase} bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900`}>{runningCommand === 'update' ? '…' : 'update'}</button>
                <button disabled={!!runningCommand} onClick={() => run('dump-autoload')} className={`${btnBase} border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800`}>dump-autoload</button>
                <button disabled={!!runningCommand} onClick={() => run('validate')} className={`${btnBase} border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800`}>validate</button>
                <button disabled={!!runningCommand} onClick={() => run('show')} className={`${btnBase} border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800`}>show</button>
              </div>
              <div className="mt-3 flex gap-2">
                <input value={packageName} onChange={e => setPackageName(e.target.value)} placeholder="vendor/package or vendor/package:^1.2"
                  className="flex-1 px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <button disabled={!!runningCommand || !packageName.trim()} onClick={() => run('require', packageName.trim())} className={`${btnBase} bg-emerald-600 hover:bg-emerald-700 text-white`}>require</button>
                <button disabled={!!runningCommand || !packageName.trim()} onClick={() => run('remove', packageName.trim())} className={`${btnBase} border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20`}>remove</button>
              </div>
            </div>

            {output && (
              <div className="bg-slate-900 rounded-2xl p-4 shadow-sm">
                <pre className="text-xs font-mono text-slate-100 whitespace-pre-wrap break-all max-h-96 overflow-y-auto">{output}</pre>
              </div>
            )}
          </>
        )}

        <div className="mt-4"><Link to={`/subscriptions/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Back to Subscription</Link></div>
      </div>
    </div>
  )
}
