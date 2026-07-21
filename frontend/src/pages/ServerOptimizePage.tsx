import Breadcrumb from '@/components/Breadcrumb'
import ServerOptimizeCard from '@/components/ServerOptimizeCard'

/*
 * Server Optimize — dedicated page (also linked from the sidebar).
 * The job runs via systemd-run transient unit in the background; survives
 * tab/browser close, status is read from the server (resume-on-reopen).
 * Future: per-service optimization (MariaDB / Nginx / PHP-FPM / Redis …) will be added here.
 */
export default function ServerOptimizePage() {
  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Tools and Settings', href: '/tools-settings' },
        { label: 'Server Optimize' },
      ]} />

      <div className="mb-5 max-w-3xl">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">Server Optimize</h1>
        <p className="mt-1 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
          Updates system packages and re-tunes MariaDB / Nginx / PHP settings based on
          server resources. The operation runs in the background — you can close this page;
          it may take a while and briefly affect services.
        </p>
      </div>

      <div className="max-w-3xl">
        <ServerOptimizeCard />
      </div>
    </div>
  )
}
