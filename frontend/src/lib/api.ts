import axios, { AxiosError } from 'axios'
import { useAuth } from '@/store/auth'

const baseURL = (import.meta.env.VITE_API_BASE as string) || '/api/v1'

export const api = axios.create({
  baseURL,
  timeout: 30_000,
})

api.interceptors.request.use((cfg) => {
  const tok = useAuth.getState().token
  if (tok) {
    cfg.headers = cfg.headers || {}
    cfg.headers.Authorization = `Bearer ${tok}`
  }
  return cfg
})

api.interceptors.response.use(
  (r) => r,
  (err: AxiosError<{ error?: string }>) => {
    if (err.response?.status === 401) {
      const s = useAuth.getState()
      if (s.token) s.logout()
    }
    return Promise.reject(err)
  },
)

export function apiError(err: unknown, fallback = 'An unexpected error occurred'): string {
  const error = err as AxiosError<{ error?: string }>
  if (error?.response?.data?.error) return error.response.data.error
  if (error?.message) return error.message
  return fallback
}
