import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import App from './App'
import './styles.css'
import { bootTheme } from '@/lib/theme'
import '@/lib/i18n'
import { bootLang, applyServerDefaultLang } from '@/lib/i18n'

bootTheme()
bootLang()
// When the visitor has no language cookie yet, adopt the server-default the admin
// chose at install time. Runs after boot; never blocks the first paint.
applyServerDefaultLang()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>,
)
