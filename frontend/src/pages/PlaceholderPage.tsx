import Breadcrumb from '@/components/Breadcrumb'

export default function PlaceholderPage({
  title, phase, description, parent,
}: {
  title: string; phase?: string; description: string
  parent?: { label: string; href: string }
}) {
  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        ...(parent ? [parent] : []),
        { label: title },
      ]} />
      <div className="flex items-center gap-3 mb-2">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{title}</h1>
        {phase && (
          <span className="text-[10px] font-semibold uppercase tracking-wider bg-amber-100 dark:bg-amber-900/30 text-amber-800 dark:text-amber-200 px-2 py-0.5 rounded">
            {phase} · Not Ready
          </span>
        )}
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">{description}</p>

      <div className="bg-white dark:bg-slate-800 border-2 border-dashed border-slate-200 dark:border-slate-700 rounded-2xl p-12 text-center">
        <div className="w-16 h-16 mx-auto rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center mb-3">
          <svg className="w-8 h-8 text-slate-400 dark:text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <h3 className="text-base font-semibold text-slate-700 dark:text-slate-300 mb-1">Under construction</h3>
        <p className="text-sm text-slate-500 dark:text-slate-500">This module will become available {phase ? <>in <span className="font-mono text-brand-700 dark:text-brand-300">{phase}</span></> : 'in a later phase'}.</p>
      </div>
    </div>
  )
}