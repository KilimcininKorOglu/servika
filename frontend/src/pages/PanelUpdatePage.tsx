import Breadcrumb from '@/components/Breadcrumb'
import PanelUpdate from '@/components/PanelUpdate'

/*
 * Panel Update dedicated page.
 * The update runs in the background (systemd-run transient unit): it survives
 * tab/browser close and even panel self-restart. The PanelUpdate component
 * reads status from the server (systemctl is-active) and re-connects to live
 * progress when the page is reopened.
 */
export default function PanelUpdatePage() {
  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: 'Tools and Settings', href: '/tools-settings' },
        { label: 'Panel Update' },
      ]} />

      <div className="mb-5 max-w-3xl">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">Panel Update</h1>
        <p className="mt-1 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
          Update the panel to the latest release from GitHub. Environment variables,
          database, and sites are preserved; a failed health check automatically rolls back
          to the previous version. The operation runs in the background, so you can close
          this page and the update continues uninterrupted.
        </p>
      </div>

      <div className="max-w-3xl">
        <PanelUpdate />
      </div>
    </div>
  )
}
