import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'

// A standing warning shown while sshd still answers on the default port.
//
// It is not dismissible on purpose: the exposure lasts as long as the port does.
// The backend reads the port from `sshd -T`, so a change made in an include under
// /etc/ssh/sshd_config.d/ is picked up too and the warning disappears on its own.

type SSHSecurity = { ports: number[]; default_port: boolean; default_value: number }

export default function SSHPortWarning() {
  const { t } = useTranslation('SSHPortWarning')
  const [security, setSecurity] = useState<SSHSecurity | null>(null)

  useEffect(() => {
    api.get<SSHSecurity>('/system/ssh-security')
      .then(response => setSecurity(response.data))
      .catch(() => { /* unreachable or not permitted; show nothing rather than a false all-clear */ })
  }, [])

  if (!security?.default_port) return null

  // Another port alongside 22 means the move was started but never finished: the
  // new port works while the old one is still exposed.
  const otherPorts = security.ports.filter(port => port !== security.default_value)

  return (
    <div role="alert"
      className="mb-6 rounded-2xl border border-red-300 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-4">
      <div className="flex items-start gap-3">
        <div className="w-10 h-10 shrink-0 rounded-lg bg-red-100 dark:bg-red-900/40 text-red-700 dark:text-red-300 flex items-center justify-center">
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" />
          </svg>
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm font-semibold text-red-800 dark:text-red-200">{t('title')}</span>
            <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300">
              {t('badge')}
            </span>
          </div>

          <p className="text-xs text-red-800/90 dark:text-red-300 mt-1">
            {otherPorts.length > 0
              ? t('descriptionMixed', { ports: otherPorts.join(', ') })
              : t('description')}
          </p>

          <details className="mt-2">
            <summary className="text-xs font-medium text-red-800 dark:text-red-200 cursor-pointer hover:underline">
              {t('how')}
            </summary>
            <ol className="mt-2 space-y-1.5 text-[11px] text-red-900/90 dark:text-red-300 list-decimal list-inside">
              <li>{t('step1')} <code className="font-mono bg-red-100 dark:bg-red-900/40 px-1 rounded">/etc/ssh/sshd_config.d/99-port.conf</code> &rarr; <code className="font-mono bg-red-100 dark:bg-red-900/40 px-1 rounded">Port 2222</code></li>
              <li>{t('step2')} <code className="font-mono bg-red-100 dark:bg-red-900/40 px-1 rounded">semanage port -a -t ssh_port_t -p tcp 2222</code></li>
              <li>{t('step3')} <Link to="/firewall" className="underline font-medium">{t('firewallLink')}</Link></li>
              <li>{t('step4')} <code className="font-mono bg-red-100 dark:bg-red-900/40 px-1 rounded">sshd -t &amp;&amp; systemctl restart sshd</code></li>
              <li className="font-medium">{t('step5')}</li>
            </ol>
            <p className="mt-2 text-[11px] text-red-900/80 dark:text-red-300/80">{t('note')}</p>
          </details>
        </div>
      </div>
    </div>
  )
}
