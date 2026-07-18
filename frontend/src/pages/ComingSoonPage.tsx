import Breadcrumb from '@/components/Breadcrumb'

interface Props {
  title: string
  description: string
  icon: string
  features: string[]
}

export default function ComingSoonPage({ title, description, icon, features }: Props) {
  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { label: 'Home', href: '/' },
        { label: title },
      ]} />

      <div className="flex items-center gap-3 mb-2">
        <span className="text-3xl">{icon}</span>
        <div>
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{title}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-500">{description}</p>
        </div>
      </div>

      <div className="bg-gradient-to-br from-brand-50/40 to-indigo-50/40 border-2 border-dashed border-brand-200 dark:border-brand-800 rounded-2xl p-8 mt-6">
        <div className="flex items-center gap-2 mb-4">
          <span className="text-[10px] uppercase tracking-wider bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 px-2 py-0.5 rounded font-bold">Coming Soon</span>
          <span className="text-xs text-slate-500 dark:text-slate-500">Roadmap</span>
        </div>
        <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-4">Planned Features</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {features.map(feature => (
            <div key={feature} className="flex items-start gap-2 px-3 py-2 bg-white dark:bg-slate-800/80 rounded border border-slate-100 dark:border-slate-800">
              <span className="text-emerald-500 flex-shrink-0">○</span>
              <span className="text-sm text-slate-700 dark:text-slate-300">{feature}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}