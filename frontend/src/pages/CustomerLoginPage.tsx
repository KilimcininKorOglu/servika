import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiError as apiError } from '@/lib/api'
import { useAuth } from '@/store/auth'
import axios from 'axios'

export default function CustomerLoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  async function signIn(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true); setError(null)
    try {
      const r = await axios.post('/api/v1/customer/login', { username, password })
      const { token, expires_at, domain_id, domain_name } = r.data
      // This atomic update stores both the token and the customer flags.
      useAuth.getState().loginCustomer(token, expires_at, domain_id, domain_name, username)
      navigate('/subscriptions/' + domain_id, { replace: true })
      setTimeout(() => window.location.reload(), 100)
    } catch (e) {
      setError(apiError(e, 'Login failed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-slate-100 to-brand-50 px-4">
      <div className="w-full max-w-md bg-white dark:bg-slate-800 rounded-2xl shadow-xl p-7">
        <div className="text-center mb-6">
          <div className="inline-flex items-center justify-center w-14 h-14 rounded-2xl bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 text-2xl mb-3">🌐</div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">Customer Portal</h1>
          <p className="text-sm text-slate-500 dark:text-slate-500 mt-1">Sign in with your account credentials</p>
        </div>

        {error && (
          <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{error}</div>
        )}

        <form onSubmit={signIn} className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Username</label>
            <input type="text" value={username} onChange={e => setUsername(e.target.value)}
              autoComplete="username" required autoFocus
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded font-mono text-sm focus:border-brand-500 outline-none" />
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">Password</label>
            <input type="password" value={password} onChange={e => setPassword(e.target.value)}
              autoComplete="current-password" required
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded font-mono text-sm focus:border-brand-500 outline-none" />
          </div>
          <button type="submit" disabled={loading || !username || !password}
            className="w-full px-4 py-2.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 font-medium rounded-md">
            {loading ? 'Signing in…' : 'Sign In'}
          </button>
        </form>
      </div>
    </div>
  )
}