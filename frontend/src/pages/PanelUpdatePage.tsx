import { useTranslation } from 'react-i18next'
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
  const { t } = useTranslation('PanelUpdatePage')
  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: t('breadcrumb.home'), href: '/' },
        { label: t('breadcrumb.toolsAndSettings'), href: '/tools-settings' },
        { label: t('breadcrumb.panelUpdate') },
      ]} />

      <div className="mb-5 max-w-3xl">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">{t('title')}</h1>
        <p className="mt-1 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
          {t('description')}
        </p>
      </div>

      <div className="max-w-3xl">
        <PanelUpdate />
      </div>
    </div>
  )
}
