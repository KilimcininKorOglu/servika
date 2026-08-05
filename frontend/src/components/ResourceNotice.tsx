import type { ReactNode } from 'react'

// The amber notice shown next to a control that starts or intensifies a service
// paid for by the whole server rather than by the domain being configured.
//
// It carries no margin or alignment of its own: each page places it where it
// belongs, above the control it describes.
export default function ResourceNotice({ children }: { children: ReactNode }) {
  return (
    <div className="inline-flex items-start gap-2 text-left px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg">
      <svg className="w-4 h-4 shrink-0 mt-0.5 text-amber-500 dark:text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" />
      </svg>
      <span className="text-xs text-amber-800 dark:text-amber-300">{children}</span>
    </div>
  )
}
