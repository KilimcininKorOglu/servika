// Copy, and hand the text over by hand when the browser will not.
//
// Kept apart from lib/clipboard.ts because that file answers only whether the
// browser took the text; this one owns what the panel shows when it did not.
import { useTranslation } from 'react-i18next'
import { copyToClipboard } from '@/lib/clipboard'
import { useDialog } from '@/lib/dialog'

/**
 * Returns a copy function that falls back to a dialog holding the text.
 *
 * The boolean it resolves to is whether the CLIPBOARD took the text, so a caller
 * shows its "copied" feedback only when that actually happened. The screens that
 * hand over a one-time password depend on this: reporting a copy that did not
 * occur loses the password for good.
 */
export function useCopyOrOffer(): (text: string) => Promise<boolean> {
  const { t } = useTranslation('common')
  const { notify } = useDialog()

  return async (text: string) => {
    if (await copyToClipboard(text)) return true
    await notify({
      title: t('copyManually'),
      message: (
        // No autoFocus: the dialog gives the keyboard to its OK button so Enter
        // closes the box. Clicking the field selects the whole value instead.
        <input
          readOnly
          value={text}
          onFocus={event => event.currentTarget.select()}
          className="mt-1 w-full rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-3 py-2 text-sm font-mono text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-brand-500/40"
        />
      ),
    })
    return false
  }
}
