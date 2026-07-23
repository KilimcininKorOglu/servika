// Mobile bottom navigation for narrow screens.
import { NavLink } from 'react-router-dom'

type MobileNavItem = {
  to: string
  label: string
  icon: string
  end?: boolean
  onClick?: () => void
}

function itemClass({ isActive }: { isActive: boolean }) {
  return `flex flex-1 flex-col items-center justify-center gap-0.5 py-1.5 min-w-0 transition ${
    isActive
      ? 'text-brand-600 dark:text-brand-400'
      : 'text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200'
  }`
}

export default function MobileNavBar({ items }: { items: MobileNavItem[] }) {
  return (
    <nav
      className="lg:hidden fixed bottom-0 inset-x-0 z-30 flex items-stretch border-t border-slate-200 dark:border-slate-800 bg-white/95 dark:bg-slate-900/95 backdrop-blur pb-[env(safe-area-inset-bottom)]"
      aria-label="Mobile navigation"
    >
      {items.map((item) => (
        <NavLink key={item.to} to={item.to} end={item.end} className={itemClass} onClick={item.onClick}>
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.7} aria-hidden>
            <path strokeLinecap="round" strokeLinejoin="round" d={item.icon} />
          </svg>
          <span className="w-full truncate px-1 text-center text-[11px] leading-tight">{item.label}</span>
        </NavLink>
      ))}
    </nav>
  )
}
