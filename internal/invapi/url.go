package invapi

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ErrNotFound means the InvAPI object is gone (safe to drop Terraform state).
var ErrNotFound = errors.New("not found")

func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("stopped after %d redirects", len(via))
	}
	if len(via) == 0 {
		return nil
	}
	orig := via[0].URL
	if req.URL.Scheme != orig.Scheme || req.URL.Host != orig.Host {
		return fmt.Errorf("refusing cross-origin redirect from %s://%s to %s", orig.Scheme, orig.Host, req.URL)
	}
	return nil
}

// ValidateBaseURL requires https except loopback http (local tests).
func ValidateBaseURL(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("must not be empty")
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("must be a valid URL: %w", err)
	}
	if u.Host == "" {
		return fmt.Errorf("URL must include a host")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Host) {
			return nil
		}
		return fmt.Errorf("URL scheme must be https (http is allowed only for localhost); got %q", s)
	default:
		return fmt.Errorf("URL scheme must be https; got %q", u.Scheme)
	}
}

func CanonicalInvAPIBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty InvAPI URL")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	raw = strings.TrimSuffix(raw, "/") + "/"
	if err := ValidateBaseURL(raw); err != nil {
		return "", err
	}
	return raw, nil
}

// ValidateConfiguredBaseURL is ValidateBaseURL plus this fork's portal allowlist
// (own TLD or localhost). The sibling portal is rejected with a pointer to the
// other Terraform provider.
func ValidateConfiguredBaseURL(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("must not be empty")
	}
	if err := ValidateBaseURL(s); err != nil {
		return err
	}
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if isLoopbackHost(u.Host) {
		return nil
	}
	if isSiblingPortalHost(u.Host) {
		return fmt.Errorf("InvAPI host %q belongs to the other Hostkey portal; use Terraform provider %s (default %s)", u.Host, SiblingProviderSource, SiblingAPIHostHint)
	}
	if !isHostkeyAPIHost(u.Host) {
		return fmt.Errorf("InvAPI host %q is not a %s InvAPI endpoint (allowed: *.%s or localhost)", u.Host, PortalDomain, PortalDomain)
	}
	return nil
}

func allowedInvAPIRewrite(current, next string) error {
	cur, err := url.Parse(current)
	if err != nil {
		return err
	}
	nxt, err := url.Parse(next)
	if err != nil {
		return err
	}
	if cur.Scheme == "https" && nxt.Scheme == "http" && !isLoopbackHost(nxt.Host) {
		return fmt.Errorf("refusing TLS downgrade to %s", next)
	}
	if sameURLHost(cur, nxt) || isLoopbackHost(nxt.Host) || isHostkeyAPIHost(nxt.Host) {
		return nil
	}
	return fmt.Errorf("login invapi host %q is not a Hostkey InvAPI endpoint", nxt.Host)
}

func hostnameOnly(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

func isHostkeyAPIHost(host string) bool {
	h := hostnameOnly(host)
	if h == PortalDomain {
		return true
	}
	return strings.HasSuffix(h, "."+PortalDomain)
}

func isSiblingPortalHost(host string) bool {
	h := hostnameOnly(host)
	if h == SiblingPortalDomain {
		return true
	}
	return strings.HasSuffix(h, "."+SiblingPortalDomain)
}

func sameURLHost(a, b *url.URL) bool {
	return strings.EqualFold(a.Host, b.Host)
}

func isLoopbackHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
