// Non-secret client-side state (session metadata, theme, dismissed notices) is
// kept in cookies, never localStorage. The credential itself stays in the
// HttpOnly `servika_session` cookie the browser sets and JS cannot read; these
// helpers only touch JS-readable, non-secret cookies.
//
// SameSite=Strict + Secure-on-HTTPS mirror the session cookie. Values are
// percent-encoded so JSON and separators survive the cookie grammar.

function secureAttr(): string {
  // localhost dev is plain HTTP; Secure there would silently drop the cookie.
  return typeof location !== 'undefined' && location.protocol === 'https:' ? '; Secure' : ''
}

export function getCookie(name: string): string | null {
  if (typeof document === 'undefined') return null
  const prefix = encodeURIComponent(name) + '='
  for (const part of document.cookie.split('; ')) {
    if (part.startsWith(prefix)) {
      return decodeURIComponent(part.slice(prefix.length))
    }
  }
  return null
}

// maxAgeSec omitted → session cookie (cleared when the browser closes).
export function setCookie(name: string, value: string, maxAgeSec?: number) {
  if (typeof document === 'undefined') return
  let c = `${encodeURIComponent(name)}=${encodeURIComponent(value)}; Path=/; SameSite=Strict${secureAttr()}`
  if (maxAgeSec !== undefined) c += `; Max-Age=${Math.max(0, Math.floor(maxAgeSec))}`
  document.cookie = c
}

export function deleteCookie(name: string) {
  if (typeof document === 'undefined') return
  document.cookie = `${encodeURIComponent(name)}=; Path=/; Max-Age=0; SameSite=Strict${secureAttr()}`
}
