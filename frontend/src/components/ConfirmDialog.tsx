import { useState } from 'react'
import Modal from './Modal'

export default function ConfirmDialog({
  open, title, message, confirmText = 'Confirm', dangerous = false,
  onConfirm, onCancel,
}: {
  open: boolean
  title: string
  message: string
  confirmText?: string
  dangerous?: boolean
  onConfirm: () => Promise<void> | void
  onCancel: () => void
}) {
  const [loading, setLoading] = useState(false)

  async function handleConfirm() {
    setLoading(true)
    try { await onConfirm() } finally { setLoading(false) }
  }

  return (
    <Modal open={open} title={title} onClose={onCancel} width="sm">
      <p className="text-sm text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-5">{message}</p>
      <div className="flex justify-end gap-2">
        <button onClick={onCancel} disabled={loading} className="px-4 py-2 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 rounded-md text-sm">
          Cancel
        </button>
        <button
          onClick={handleConfirm}
          disabled={loading}
          className={`px-4 py-2 text-white rounded-md text-sm font-medium ${
            dangerous ? 'bg-red-600 hover:bg-red-700 disabled:bg-red-300' : 'bg-brand-600 hover:bg-brand-700 disabled:opacity-60'
          }`}
        >
          {loading ? 'Processing…' : confirmText}
        </button>
      </div>
    </Modal>
  )
}