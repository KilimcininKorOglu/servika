import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Domain = { id: number; domain_name: string }

const TOOL_META: Record<string, { label: string; phase?: string; description: string }> = {
  connection: { label: 'Connection Details', description: 'FTP server, username, database connection string, and quick-copy actions.' },
  files: { label: 'File Manager', phase: 'F6', description: 'List, upload, download, and change permissions for files under public_html.' },
  databases: { label: 'Databases', phase: 'F5', description: 'MySQL databases, users, and phpMyAdmin integration.' },
  ftp: { label: 'FTP Accounts', phase: 'F4', description: 'Virtual FTP accounts, passwords, and home directories through Pure-FTPd.' },
  backups: { label: 'Backup and Restore', phase: 'F12', description: 'Back up tarballs and database dumps to SFTP, S3, or local storage.' },
  copy: { label: 'Copy Website', description: 'Clone an existing website to another domain.' },
  php: { label: 'PHP Settings', phase: 'F3', description: 'PHP-FPM pool selection, version changes, and php.ini parameters.' },
  logs: { label: 'Logs', phase: 'F10', description: 'Live access.log and error.log monitoring with WebSocket tailing.' },
  cron: { label: 'Scheduled Tasks', phase: 'F8', description: 'Per-user crontab editor.' },
  git: { label: 'Git', phase: 'F9', description: 'Connect repositories, configure deploy keys, and pull automatically with webhooks.' },
  composer: { label: 'PHP Composer', phase: 'F3', description: 'Web interface for composer install and update.' },
  performance: { label: 'Performance', description: 'Accelerators such as OPcache, gzip, and lazy loading.' },
  ssl: { label: 'SSL/TLS Certificate', phase: 'F7', description: 'Automatic Let\'s Encrypt installation and renewal.' },
  'password-protection': { label: 'Password-Protected Directories', phase: 'F7', description: 'Protect directories with .htpasswd.' },
  stats: { label: 'Statistics', phase: 'F10', description: 'Disk, traffic, and visitor reports.' },
  imunify: { label: 'Imunify', description: 'Antivirus and WAF integration.' },
}

export default function ToolPage() {
  const { id, slug } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`).then(response => setDomain(response.data)).catch(requestError => setError(apiError(requestError)))
  }, [id])

  const meta = TOOL_META[slug || ''] || { label: slug || 'Tool', description: 'Not implemented yet.' }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Domains', href: '/domains' },
        { label: domain?.domain_name || '...', href: `/subscriptions/${id}` },
        { label: meta.label },
      ]} />

      <div className="flex items-center gap-3 mb-2">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{meta.label}</h1>
        {meta.phase && (
          <span className="text-[10px] font-semibold uppercase tracking-wider bg-amber-100 dark:bg-amber-900/30 text-amber-800 dark:text-amber-200 px-2 py-0.5 rounded">
            {meta.phase} · Not Ready
          </span>
        )}
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-1">
        {domain ? <>Domain: <Link to={`/subscriptions/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">{domain.domain_name}</Link></> : '...'}
      </p>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">{meta.description}</p>
      {error && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>}

      <div className="bg-white dark:bg-slate-800 border-2 border-dashed border-slate-200 dark:border-slate-700 rounded-2xl p-12 text-center">
        <div className="w-16 h-16 mx-auto rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center mb-3">
          <svg className="w-8 h-8 text-slate-400 dark:text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <h3 className="text-base font-semibold text-slate-700 dark:text-slate-300 mb-1">Under construction</h3>
        <p className="text-sm text-slate-500 dark:text-slate-500">
          This module will become available {meta.phase ? <>in <span className="font-mono text-brand-700 dark:text-brand-300">{meta.phase}</span></> : 'in a later phase'}.
        </p>
        <Link to={`/subscriptions/${id}`} className="inline-block mt-4 text-sm text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">
          ← Return to domain dashboard
        </Link>
      </div>
    </div>
  )
}