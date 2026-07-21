import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, apiError } from '@/lib/api'
import { useAuth } from '@/store/auth'

type LoginResponse = {
  token?: string
  expires_at?: number
  user?: { id: number; name: string; role: 'admin' | 'reseller' | 'user'; full_name?: string }
  two_factor_required?: boolean
}

export default function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [requiresTwoFactor, setRequiresTwoFactor] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()
  const login = useAuth((state) => state.login)

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault()
    setError(null); setLoading(true)
    try {
      const { data } = await api.post<LoginResponse>('/auth/login', { username, password, code })
      if (data.two_factor_required) {
        setRequiresTwoFactor(true); setLoading(false)
        return
      }
      login(data.token!, data.user!, data.expires_at!)
      navigate('/', { replace: true })
    } catch (caughtError) {
      setError(apiError(caughtError, 'Login failed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-slate-50 to-orange-50 dark:from-slate-950 dark:to-slate-900 px-4">
      <div className="w-full max-w-md">
        <div className="flex items-center justify-center mb-8">
          <div className="w-12 h-12 rounded-2xl bg-brand-600 flex items-center justify-center shadow-lg shadow-brand-600/30">
            <svg viewBox="0 0 32 32" className="w-7 h-7 text-white" fill="currentColor">
              <path d="M9 10h14v3H9zM9 15h14v3H9zM9 20h9v3H9z" />
            </svg>
          </div>
          <div className="ml-3">
            <div className="text-xl font-semibold text-slate-900 dark:text-slate-100">Servika</div>
            <div className="text-xs text-slate-500 dark:text-slate-500">Hosting Control Panel</div>
          </div>
        </div>

        <div className="bg-white dark:bg-slate-800 rounded-2xl shadow-xl border border-slate-200 dark:border-slate-700/60 p-8">
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">Welcome</h1>
          <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">Sign in to continue.</p>

          <form onSubmit={onSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">Username</label>
              <input
                type="text"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
                autoFocus
                required
                className="w-full px-3.5 py-2.5 border border-slate-300 dark:border-slate-600 rounded-lg focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">Password</label>
              <input
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
                required
                readOnly={requiresTwoFactor}
                className="w-full px-3.5 py-2.5 border border-slate-300 dark:border-slate-600 rounded-lg focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition disabled:opacity-60"
              />
            </div>

            {requiresTwoFactor && (
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">Two-factor authentication code</label>
                <input
                  type="text"
                  inputMode="numeric"
                  value={code}
                  onChange={(event) => setCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
                  autoFocus
                  placeholder="000000"
                  className="w-full px-3.5 py-2.5 text-center text-lg font-mono tracking-[0.4em] border border-slate-300 dark:border-slate-600 rounded-lg focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition"
                />
                <p className="text-xs text-slate-400 dark:text-slate-500 mt-1.5">Enter the six-digit code from your authenticator app.</p>
              </div>
            )}

            {error && (
              <div className="px-3.5 py-2.5 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={loading}
              className="w-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 font-medium py-2.5 rounded-lg transition shadow-lg shadow-brand-600/20 disabled:shadow-none"
            >
              {loading ? 'Signing in…' : requiresTwoFactor ? 'Verify and sign in' : 'Sign in'}
            </button>
          </form>
        </div>

        <p className="text-center text-xs text-slate-400 dark:text-slate-500 mt-6">
          Servika · version 0.2.0-f1
        </p>
      </div>
    </div>
  )
}
