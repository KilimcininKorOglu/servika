import { create } from 'zustand'
import { getCookie, setCookie, deleteCookie } from '@/lib/cookies'

export type User = {
  id: number
  name: string
  role: 'admin' | 'reseller' | 'user'
  full_name?: string
}

// The session token lives ONLY in an HttpOnly cookie (servika_session) that
// JavaScript cannot read. This store keeps just non-secret session metadata in
// JS-readable cookies (never localStorage) so route guards can decide
// synchronously; being "authenticated" means having a stored, non-expired user.
// The HttpOnly cookie alone carries the credential.
//
// isCustomer / customerDomainID / customerDomainName are derived here and read
// from the store by consumers, so no component touches the cookies directly.
type AuthState = {
  username: User | null
  expires_at: number | null
  isCustomer: boolean
  customerDomainID: string
  customerDomainName: string
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
  deleteCookie(KEY_CUSTOMER)
  deleteCookie(KEY_CUSTOMER_DOM)
  deleteCookie(KEY_CUSTOMER_DOMAIN)
}

// Persist metadata cookies with the token's own remaining lifetime, so they
// expire together with the credential instead of lingering as a stale session.
function remainingSec(exp: number): number {
  return Math.max(0, exp - Math.floor(Date.now() / 1000))
}

type InitialState = {
  username: User | null
  expires_at: number | null
  isCustomer: boolean
  customerDomainID: string
  customerDomainName: string
}

function loggedOut(): InitialState {
  return { username: null, expires_at: null, isCustomer: false, customerDomainID: '', customerDomainName: '' }
}

function initialStatus(): InitialState {
  if (typeof window === 'undefined') return loggedOut()
  const u = getCookie(KEY_USER)
  const e = getCookie(KEY_EXP)
  if (!u || !e) {
    customerFlagDelete()
    return loggedOut()
  }
  const exp = Number(e)
  if (!Number.isFinite(exp) || exp * 1000 < Date.now()) {
    deleteCookie(KEY_USER)
    deleteCookie(KEY_EXP)
    customerFlagDelete()
    return loggedOut()
  }
  try {
    return {
      username: JSON.parse(u) as User,
      expires_at: exp,
      isCustomer: getCookie(KEY_CUSTOMER) === '1',
      customerDomainID: getCookie(KEY_CUSTOMER_DOM) || '',
      customerDomainName: getCookie(KEY_CUSTOMER_DOMAIN) || '',
    }
  } catch {
    return loggedOut()
  }
}

export const useAuth = create<AuthState>((set) => ({
  ...initialStatus(),
  login: (username, expires_at) => {
    const ttl = remainingSec(expires_at)
    setCookie(KEY_USER, JSON.stringify(username), ttl)
    setCookie(KEY_EXP, String(expires_at), ttl)
    customerFlagDelete()
    set({ username, expires_at, isCustomer: false, customerDomainID: '', customerDomainName: '' })
  },
  loginCustomer: (expires_at, domainID, domainName, username) => {
    const syntheticUser: User = { id: 0, name: username, role: 'user', full_name: domainName }
    const ttl = remainingSec(expires_at)
    setCookie(KEY_USER, JSON.stringify(syntheticUser), ttl)
    setCookie(KEY_EXP, String(expires_at), ttl)
    setCookie(KEY_CUSTOMER, '1', ttl)
    setCookie(KEY_CUSTOMER_DOM, String(domainID), ttl)
    setCookie(KEY_CUSTOMER_DOMAIN, domainName, ttl)
    set({
      username: syntheticUser,
      expires_at,
      isCustomer: true,
      customerDomainID: String(domainID),
      customerDomainName: domainName,
    })
  },
  updateName: (fullName) => set((state) => {
    if (!state.username) return state
    const updatedUser = { ...state.username, full_name: fullName }
    // Keep the cookie's remaining lifetime aligned with the token, not extended.
    const ttl = state.expires_at ? remainingSec(state.expires_at) : undefined
    setCookie(KEY_USER, JSON.stringify(updatedUser), ttl)
    return { username: updatedUser }
  }),
  logout: () => {
    // Best-effort server-side cookie clear. Raw fetch (not the axios `api`
    // instance) avoids a circular import; the endpoint is public so it never
    // 401s back into this handler. Relative path stays same-origin in prod/dev.
    try {
      void fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'include' }).catch(() => {})
    } catch { /* ignore network/runtime errors; local state is cleared regardless */ }
    deleteCookie(KEY_USER)
    deleteCookie(KEY_EXP)
    customerFlagDelete()
    set({ username: null, expires_at: null, isCustomer: false, customerDomainID: '', customerDomainName: '' })
  },
  hydrate: () => {
    // initialStatus() runs at store creation; re-read here so a caller can
    // force a refresh (e.g. after the tab regains focus) without a reload.
    set(initialStatus())
  },
}))
