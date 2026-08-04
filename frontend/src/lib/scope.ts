import { useParams } from 'react-router'

// useResourceScope resolves the API base path and the back link for pages that
// serve both a domain and one of its subdomains. When the route carries an :sid
// param the page is subdomain-scoped, so every request is prefixed with
// /domains/:id/subdomain/:sid and acts on the subdomain's own document root.
export function useResourceScope() {
  const { id, sid } = useParams()
  const isSubdomain = Boolean(sid)
  return {
    id,
    sid,
    isSubdomain,
    base: isSubdomain ? `/domains/${id}/subdomain/${sid}` : `/domains/${id}`,
    backHref: isSubdomain ? `/domains/${id}/subdomain/${sid}` : `/subscriptions/${id}`,
    backLabel: isSubdomain ? '← Back to subdomain' : '← Back to subscription',
  }
}
