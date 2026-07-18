import { create } from 'zustand'

export type User = {
  id: number
  name: string
  role: 'admin' | 'reseller' | 'user'
  full_name?: string
}

type AuthState = {
  token: string | null
  username: User | null
  expires_at: number | null
  login: (token: string, username: User, expires_at: number) => void
  loginCustomer: (token: string, expires_at: number, domainID: number, domainName: string, username: string) => void
  updateName: (fullName: string) => void
  logout: () => void
  hydrate: () => void
}

const KEY_TOKEN = 'servika.token'
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
    return { token: null as string | null, username: null as User | null, expires_at: null as number | null }
  }
  const t = localStorage.getItem(KEY_TOKEN)
  const u = localStorage.getItem(KEY_USER)
  const e = localStorage.getItem(KEY_EXP)
  if (!t || !u || !e) {
    customerFlagDelete()
    return { token: null, username: null, expires_at: null }
  }
  const exp = Number(e)
  if (!Number.isFinite(exp) || exp * 1000 < Date.now()) {
    localStorage.removeItem(KEY_TOKEN)
    localStorage.removeItem(KEY_USER)
    localStorage.removeItem(KEY_EXP)
    customerFlagDelete()
    return { token: null, username: null, expires_at: null }
  }
  try {
    return { token: t, username: JSON.parse(u) as User, expires_at: exp }
  } catch {
    return { token: null, username: null, expires_at: null }
  }
}

export const useAuth = create<AuthState>((set) => ({
  ...initialStatus(),
  login: (token, username, expires_at) => {
    localStorage.setItem(KEY_TOKEN, token)
    localStorage.setItem(KEY_USER, JSON.stringify(username))
    localStorage.setItem(KEY_EXP, String(expires_at))
    customerFlagDelete()
    set({ token, username, expires_at })
  },
  loginCustomer: (token, expires_at, domainID, domainName, username) => {
    const syntheticUser: User = { id: 0, name: username, role: 'user', full_name: domainName }
    localStorage.setItem(KEY_TOKEN, token)
    localStorage.setItem(KEY_USER, JSON.stringify(syntheticUser))
    localStorage.setItem(KEY_EXP, String(expires_at))
    localStorage.setItem(KEY_CUSTOMER, '1')
    localStorage.setItem(KEY_CUSTOMER_DOM, String(domainID))
    localStorage.setItem(KEY_CUSTOMER_DOMAIN, domainName)
    set({ token, username: syntheticUser, expires_at })
  },
  updateName: (fullName) => set((state) => {
    if (!state.username) return state
    const updatedUser = { ...state.username, full_name: fullName }
    try { localStorage.setItem(KEY_USER, JSON.stringify(updatedUser)) } catch { /* Ignore storage failures. */ }
    return { username: updatedUser }
  }),
  logout: () => {
    localStorage.removeItem(KEY_TOKEN)
    localStorage.removeItem(KEY_USER)
    localStorage.removeItem(KEY_EXP)
    customerFlagDelete()
    set({ token: null, username: null, expires_at: null })
  },
  hydrate: () => {
    /* initialStatus() handles this during the first render. */
  },
}))
