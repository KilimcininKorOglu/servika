import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import App from './App'
import './styles.css'
import { bootTheme } from '@/lib/theme'
import '@/lib/i18n'
import { bootLang, applyServerDefaultLang } from '@/lib/i18n'
import DialogProvider from '@/components/DialogProvider'

bootTheme()
bootLang()
// When the visitor has no language cookie yet, adopt the server-default the admin
// chose at install time. Runs after boot; never blocks the first paint.
applyServerDefaultLang()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      {/* Outside App so every route, including the pre-login screens, can ask
          for a themed confirm/prompt/notice instead of a browser box. */}
      <DialogProvider>
        <App />
      </DialogProvider>
    </BrowserRouter>
  </React.StrictMode>,
)
