import React, { Suspense } from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import App from './App'
import './styles.css'
import { bootTheme } from '@/lib/theme'
import { bootLang, initI18n, resolveBootLang } from '@/lib/i18n'
import DialogProvider from '@/components/DialogProvider'

// Boot resolves the language and loads its always-mounted namespaces BEFORE the
// first render. Translations are fetched on demand now, so a render that starts
// without them would paint the sign-in screen in one language and swap it for
// another a moment later.
async function boot() {
  bootTheme()
  const lang = await resolveBootLang()
  bootLang(lang)
  await initI18n(lang)

  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      {/* The sign-in routes are rendered straight from App, so the only other
          boundary (inside DashboardLayout, around the Outlet) never covers
          them. Without this one a page suspending on its namespace would take
          the whole app down instead of showing a fallback. React picks the
          nearest boundary, so navigation inside the panel still lands on the
          layout's own fallback rather than this one. */}
      <Suspense fallback={
        <div className="flex min-h-screen items-center justify-center bg-slate-50 text-sm font-medium text-slate-400 dark:bg-slate-950 dark:text-slate-500" role="status">
          {/* No text to translate here: this renders while a translation is
              still in flight, so anything but the product name could only be
              shown in the wrong language. */}
          Servika
        </div>
      }>
        <BrowserRouter>
          {/* Outside App so every route, including the pre-login screens, can ask
              for a themed confirm/prompt/notice instead of a browser box. */}
          <DialogProvider>
            <App />
          </DialogProvider>
        </BrowserRouter>
      </Suspense>
    </React.StrictMode>,
  )
}

void boot()
