// Promise-based replacement for window.confirm / prompt / alert.
//
// The browser's own boxes are not just off-theme. After a few of them in one
// tab every major browser offers "prevent this page from creating additional
// dialogs", and once that is ticked alert() does nothing, confirm() always
// returns false and prompt() always returns null. On a page like the file
// manager, where alert(apiError(...)) is the ONLY place an error is surfaced,
// that turns a failed delete into a delete that silently did nothing.
//
// The API is promise-based on purpose: it drops into a call site one for one,
// so `if (!confirm(msg)) return` becomes `if (!(await confirm({ message: msg })))
// return` without splitting the surrounding function into state and JSX. The
// declarative components/ConfirmDialog.tsx stays as it is for the screens that
// already use it; this draws with the same components/Modal.tsx, so there is
// still one visual language.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import Modal from './Modal'
import { DialogContext } from '@/lib/dialog'
import type { AskOptions, DialogAPI, NotifyOptions } from '@/lib/dialog'

type Kind = 'confirm' | 'ask' | 'notify'

interface Request {
  id: number
  kind: Kind
  options: AskOptions & NotifyOptions
  resolve: (result: boolean | string | null) => void
}

export default function DialogProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation('common')
  const [queue, setQueue] = useState<Request[]>([])
  const nextID = useRef(0)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const confirmRef = useRef<HTMLButtonElement | null>(null)

  // Requests queue rather than replace one another. Replacing would leave the
  // displaced promise unsettled, and its caller waiting for an answer that can
  // never arrive.
  const open = useCallback((kind: Kind, options: AskOptions & NotifyOptions) => {
    return new Promise<boolean | string | null>(resolve => {
      nextID.current += 1
      setQueue(pending => [...pending, { id: nextID.current, kind, options, resolve }])
    })
  }, [])

  const api = useMemo<DialogAPI>(() => ({
    confirm: async options => (await open('confirm', options)) as boolean,
    ask: async options => (await open('ask', options)) as string | null,
    notify: async options => { await open('notify', options) },
  }), [open])

  const current = queue[0] ?? null

  // Give the dialog the keyboard as soon as it appears: the text field when
  // there is one, otherwise the button Enter should activate.
  useEffect(() => {
    if (!current) return
    const focus = window.setTimeout(() => {
      if (current.kind === 'ask') inputRef.current?.focus()
      else confirmRef.current?.focus()
    }, 30)
    return () => window.clearTimeout(focus)
  }, [current])

  function settle(result: boolean | string | null) {
    if (!current) return
    setQueue(pending => pending.slice(1))
    current.resolve(result)
  }

  function cancel() { settle(current?.kind === 'ask' ? null : false) }

  function accept() {
    if (!current) return
    settle(current.kind === 'ask' ? (inputRef.current?.value ?? '') : true)
  }

  const options = current?.options
  const dangerous = !!options?.dangerous
  const isError = current?.kind === 'notify' && options?.tone === 'error'
  const accented = dangerous || isError

  function defaultTitle() {
    if (!current) return ''
    if (current.kind === 'confirm') return t('areYouSure')
    if (current.kind === 'ask') return t('enterValue')
    return isError ? t('error') : t('notice')
  }

  return (
    <DialogContext.Provider value={api}>
      {children}
      {current && options && (
        // Escape closes through Modal's own handler, which cancels.
        <Modal open title={options.title ?? defaultTitle()} onClose={cancel} width="sm">
          {options.message !== undefined && options.message !== '' && (
            <div className="text-sm leading-relaxed text-slate-600 dark:text-slate-400 break-words">
              {options.message}
            </div>
          )}

          {current.kind === 'ask' && (
            <input
              key={current.id}
              ref={inputRef}
              type={options.type === 'password' ? 'password' : 'text'}
              defaultValue={options.defaultValue ?? ''}
              placeholder={options.placeholder}
              onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); accept() } }}
              className="mt-4 w-full rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-3 py-2 text-sm text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-brand-500/40 focus:border-brand-400"
            />
          )}

          <div className="flex justify-end gap-2 mt-5">
            {current.kind !== 'notify' && (
              <button
                type="button"
                onClick={cancel}
                className="px-4 py-2 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 rounded-md text-sm"
              >
                {options.cancelLabel ?? t('cancel')}
              </button>
            )}
            <button
              type="button"
              ref={confirmRef}
              onClick={accept}
              className={`px-4 py-2 text-white rounded-md text-sm font-medium ${
                accented ? 'bg-red-600 hover:bg-red-700' : 'bg-brand-600 hover:bg-brand-700'
              }`}
            >
              {options.confirmLabel ?? (current.kind === 'notify' ? t('ok') : t('confirm'))}
            </button>
          </div>
        </Modal>
      )}
    </DialogContext.Provider>
  )
}
