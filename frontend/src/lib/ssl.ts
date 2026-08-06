// What a domain's certificate means, decided once.
//
// Four screens report SSL and each draws it in its own shape: an inline pill on
// the domain list, a lock icon on the dashboard, a tool card on the domain
// overview, a table cell on the SSL list. Sharing a component would mean one
// component with four variants, so what is shared here is the DECISION instead:
// every screen branches on the same answer and renders it in its own idiom.
//
// This mirrors `domains.SSLSourceIsTrusted` in the backend. Keep the two in step.

/** The values `domains.ssl_source` can hold. */
export const SSL_SOURCE_LETSENCRYPT = 'letsencrypt'
export const SSL_SOURCE_SELF_SIGNED = 'self-signed'
export const SSL_SOURCE_IMPORTED = 'imported'

/**
 * `trusted` is a certificate a browser accepts, `selfSigned` is the fail-safe
 * that leaves the visitor on a full-page warning, `none` is no certificate.
 */
export type SslState = 'trusted' | 'selfSigned' | 'none'

/**
 * Only the self-signed fail-safe is untrusted. An imported certificate arrives
 * from a cPanel migration and is as real as one Servika ordered, and an unknown
 * source stays trusted: it is what a domain with no certificate carries and what
 * rows written before the column existed still hold, so treating it as bad would
 * raise a false alarm on both.
 */
export function sslState(enabled?: boolean, source?: string): SslState {
  if (!enabled) return 'none'
  return source === SSL_SOURCE_SELF_SIGNED ? 'selfSigned' : 'trusted'
}
