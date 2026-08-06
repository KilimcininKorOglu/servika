import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { dismissError, getReportedErrors, subscribeErrors, type ReportedError } from '@/lib/errors'

// Mounted once by DashboardLayout. Background requests that have nowhere to put
// an inline error surface here instead of failing invisibly.
export default function ErrorSurface() {
  const { t } = useTranslation('ErrorSurface')
  const [entries, setEntries] = useState<ReportedError[]>(getReportedErrors)

  useEffect(() => subscribeErrors(setEntries), [])

  if (entries.length === 0) return null

  return (
    <div
      className="fixed bottom-4 right-4 z-50 flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2"
      // The banner reports something the user did not ask for right now, so it
      // is announced politely rather than interrupting what they are reading.
      role="status"
      aria-live="polite"
    >
      {entries.map(entry => (
        <div
          key={entry.id}
          className="rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 shadow-lg dark:border-amber-800 dark:bg-amber-900/30"
        >
          <div className="flex items-start gap-2">
            <svg className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
            </svg>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-amber-900 dark:text-amber-200">
                {t('title', { context: entry.context })}
              </p>
              {/* The detail comes from the API, which is English by contract.
                  It is a diagnostic line, so it stays secondary to the label. */}
              <p className="mt-0.5 break-words font-mono text-[11px] text-amber-800/80 dark:text-amber-300/80">
                {entry.detail}
              </p>
            </div>
            <button
              type="button"
              onClick={() => dismissError(entry.id)}
              aria-label={t('dismiss')}
              className="shrink-0 rounded p-0.5 text-amber-700 hover:bg-amber-100 dark:text-amber-400 dark:hover:bg-amber-900/50"
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      ))}
    </div>
  )
}
