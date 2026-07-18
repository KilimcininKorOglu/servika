// Manages the panel's dark, light, and system themes.
// - localStorage key stores light, dark, or system mode.
// - system: follows the prefers-color-scheme: dark media query
// - Class: <html class="dark"> matches Tailwind darkMode: 'class'.

export type Theme = 'light' | 'dark' | 'system'

const KEY = 'servika.theme'

export function getTheme(): Theme {
  if (typeof window === 'undefined') return 'light'
  const v = localStorage.getItem(KEY) as Theme | null
  // Default: light — use the light theme until the user selects another option.
  return v === 'dark' || v === 'light' || v === 'system' ? v : 'light'
}

export function effectiveTheme(t: Theme): 'light' | 'dark' {
  if (t === 'system') {
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  return t
}

export function applyTheme(t: Theme) {
  const eff = effectiveTheme(t)
  const html = document.documentElement
  if (eff === 'dark') html.classList.add('dark')
  else html.classList.remove('dark')
}

export function setTheme(t: Theme) {
  localStorage.setItem(KEY, t)
  applyTheme(t)
  window.dispatchEvent(new CustomEvent('servika:theme-change', { detail: t }))
}

// Call during boot from main.tsx before the initial render.
// main.tsx must call this immediately after importing it to prevent FOUC.
export function bootTheme() {
  applyTheme(getTheme())
  // Follow OS preference changes while the system theme is selected.
  if (window.matchMedia) {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    mq.addEventListener?.('change', () => {
      if (getTheme() === 'system') applyTheme('system')
    })
  }
}
