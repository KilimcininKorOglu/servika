import { create } from 'zustand'

export type User = {
  id: number
  name: string
  role: 'admin' | 'reseller' | 'user'
  full_name?: string
}

// The session token now lives ONLY in an HttpOnly cookie (servika_session) that
// JavaScript cannot read. This store keeps just non-secret session metadata so
// route guards can decide synchronously; being "authenticated" means having a
// stored, non-expired user. The cookie alone carries the credential.
type AuthState = {
  username: User | null
  expires_at: number | null
  login: (username: User, expires_at: number) => void
  loginCustomer: (expires_at: number, domainID: number, domainName: string, username: string) => void
  updateName: (fullName: string) => void
  logout: () => void
  hydrate: () => void
}

const KEY_USER  = 'servika.user'
const KEY_EXP   = 'servika.exp'

const KEY_CUSTOMER      = 'servika.customer'
const KEY_CUSTOMER_DOM  = 'servika.customer.domain_id'
const KEY_CUSTOMER_DOMAIN = 'servika.customer.domain_name'

function customerFlagDelete() {
  localStorage.removeItem(KEY_CUSTOMER)
  localStorage.removeItem(KEY_CUSTOMER_DOM)
  localStorage.removeItem(KEY_CUSTOMER_DOMAIN)
}

function initialStatus() {
  if (typeof window === 'undefined') {
    return { username: null as User | null, expires_at: null as number | null }
  }
  const u = localStorage.getItem(KEY_USER)
  const e = localStorage.getItem(KEY_EXP)
  if (!u || !e) {
    customerFlagDelete()
    return { username: null, expires_at: null }
  }
  const exp = Number(e)
  if (!Number.isFinite(exp) || exp * 1000 < Date.now()) {
    localStorage.removeItem(KEY_USER)
    localStorage.removeItem(KEY_EXP)
    customerFlagDelete()
    return { username: null, expires_at: null }
  }
  try {
    return { username: JSON.parse(u) as User, expires_at: exp }
  } catch {
    return { username: null, expires_at: null }
  }
}

export const useAuth = create<AuthState>((set) => ({
  ...initialStatus(),
  login: (username, expires_at) => {
    localStorage.setItem(KEY_USER, JSON.stringify(username))
    localStorage.setItem(KEY_EXP, String(expires_at))
    customerFlagDelete()
    set({ username, expires_at })
  },
  loginCustomer: (expires_at, domainID, domainName, username) => {
    const syntheticUser: User = { id: 0, name: username, role: 'user', full_name: domainName }
    localStorage.setItem(KEY_USER, JSON.stringify(syntheticUser))
    localStorage.setItem(KEY_EXP, String(expires_at))
    localStorage.setItem(KEY_CUSTOMER, '1')
    localStorage.setItem(KEY_CUSTOMER_DOM, String(domainID))
    localStorage.setItem(KEY_CUSTOMER_DOMAIN, domainName)
    set({ username: syntheticUser, expires_at })
  },
  updateName: (fullName) => set((state) => {
    if (!state.username) return state
    const updatedUser = { ...state.username, full_name: fullName }
    try { localStorage.setItem(KEY_USER, JSON.stringify(updatedUser)) } catch { /* Ignore storage failures. */ }
    return { username: updatedUser }
  }),
  logout: () => {
    // Best-effort server-side cookie clear. Raw fetch (not the axios `api`
    // instance) avoids a circular import; the endpoint is public so it never
    // 401s back into this handler. Relative path stays same-origin in prod/dev.
    try {
      void fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'include' }).catch(() => {})
    } catch { /* ignore network/runtime errors; local state is cleared regardless */ }
    localStorage.removeItem(KEY_USER)
    localStorage.removeItem(KEY_EXP)
    customerFlagDelete()
    set({ username: null, expires_at: null })
  },
  hydrate: () => {
    /* initialStatus() handles this during the first render. */
  },
}))
