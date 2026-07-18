import { useNavigate } from 'react-router-dom'

type Color = 'amber' | 'violet' | 'sky' | 'indigo' | 'emerald' | 'teal' | 'slate' | 'orange' | 'rose'

const BG: Record<Color, string> = {
  amber:   'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300',
  violet:  'bg-violet-100 dark:bg-violet-900/30 text-violet-700 dark:text-violet-300',
  sky:     'bg-sky-100 text-sky-700',
  indigo:  'bg-indigo-100 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-300',
  emerald: 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300',
  teal:    'bg-teal-100 text-teal-700',
  slate:   'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300',
  orange:  'bg-orange-100 text-orange-700',
  rose:    'bg-rose-100 text-rose-700',
}

export default function ToolCard({
  label, description, icon, color = 'slate', phase, warning, to, onClick,
}: {
  label: string
  description?: string
  icon: string
  color?: Color
  phase?: string
  warning?: string
  to?: string
  onClick?: () => void
}) {
  const navigate = useNavigate()
  const handleClick = () => {
    if (to) navigate(to)
    else if (onClick) onClick()
  }
  const body = (
    <>
      <div className={`w-10 h-10 rounded-2xl flex items-center justify-center flex-shrink-0 ${BG[color]}`}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} className="w-5 h-5">
          <path strokeLinecap="round" strokeLinejoin="round" d={icon} />
        </svg>
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium text-slate-900 dark:text-slate-100 truncate flex items-center gap-1.5">
          <span className="truncate">{label}</span>
          {phase && (
            <span className="text-[9px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1 py-0.5 rounded">
              {phase}
            </span>
          )}
        </div>
        {description && <div className="text-xs text-slate-500 dark:text-slate-500 truncate mt-0.5">{description}</div>}
        {warning && <div className="text-[11px] text-red-600 dark:text-red-400 truncate mt-0.5">{warning}</div>}
      </div>
    </>
  )
  const className = 'group flex items-start gap-3 p-3 rounded-2xl border border-slate-200 dark:border-slate-700 hover:border-slate-300 dark:hover:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800/50 hover:shadow-sm transition text-left w-full cursor-pointer'

  return <button type="button" onClick={handleClick} className={className}>{body}</button>
}