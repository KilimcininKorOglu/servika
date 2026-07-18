export default function NotReadyYet({
  title, phase, description,
}: { title: string; phase: string; description: string }) {
  return (
    <div className="px-8 py-6">
      <div className="flex items-center mb-1">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{title}</h1>
        <span className="ml-3 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wider bg-amber-100 dark:bg-amber-900/30 text-amber-800 dark:text-amber-200 rounded">
          {phase} · Not Ready
        </span>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-8">{description}</p>

      <div className="bg-white dark:bg-slate-800 border-2 border-dashed border-slate-200 dark:border-slate-700 rounded-2xl p-12 text-center">
        <div className="w-16 h-16 mx-auto rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center mb-4">
          <svg className="w-8 h-8 text-slate-400 dark:text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-1">Under construction</h3>
        <p className="text-sm text-slate-500 dark:text-slate-500 max-w-md mx-auto">
          This module will become available in <span className="font-mono text-brand-700 dark:text-brand-300">{phase}</span> phase.
          Only the interface shell is currently available.
        </p>
      </div>
    </div>
  )
}