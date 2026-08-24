package invapi

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultUserAgent = ProviderBinaryName + "/dev"

	// maxResponseBodyBytes caps how much of an InvAPI response body the client
	// buffers into memory. InvAPI responses are small JSON documents; without a
	// cap, a misbehaving intermediary, a same-origin redirect to a huge file, or
	// a compromised endpoint could force the process to read an unbounded body
	// into memory.
	maxResponseBodyBytes = 8 << 20 // 8 MiB

	// maxRetryAfter bounds how long the client will honor a server-provided
	// Retry-After header, so a malicious or misconfigured server cannot stall
	// a plan/apply indefinitely.
	maxRetryAfter = 30 * time.Second

	// defaultCatalogCacheTTL controls how long presets/os/software/traffic_plans
	// list responses are memoized. These catalogs change rarely; caching for a
	// short window eliminates redundant round-trips within the same
	// terraform plan/apply run without risking long-lived staleness.
	defaultCatalogCacheTTL = 30 * time.Second
)

type Config struct {
	BaseURL     string
	HTTPClient  *http.Client
	MaxRetries  int
	HTTPTimeout time.Duration
	UserAgent   string
}

type Client struct {
	mu         sync.RWMutex
	baseURL    string
	httpClient *http.Client
	maxRetries int
	userAgent  string
	auth       *TokenManager

	// catalogMu/catalogCache/catalogTTL implement the short-lived in-memory
	// catalog cache described in catalog.go.
	catalogMu    sync.Mutex
	catalogCache map[string]catalogCacheEntry
	catalogTTL   time.Duration
}

func NewClient(cfg Config, auth *TokenManager) (*Client, error) {
	base := strings.TrimSuffix(cfg.BaseURL, "/") + "/"
	if base == "/" {
		base = DefaultBaseURL
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.HTTPTimeout
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		httpClient = defaultHTTPClient(timeout)
	}

	retries := cfg.MaxRetries
	if retries <= 0 {
		retries = 3
	}

	ua := strings.TrimSpace(cfg.UserAgent)
	if ua == "" {
		ua = defaultUserAgent
	}

	return &Client{
		baseURL:    base,
		httpClient: httpClient,
		maxRetries: retries,
		userAgent:  ua,
		auth:       auth,
		catalogTTL: defaultCatalogCacheTTL,
	}, nil
}

func defaultHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: sameOriginRedirect}
}

// BaseURLForRegion is kept for smoke/tools that still pass a flag; this fork
// always returns the portal default (region is not a provider attribute).
func BaseURLForRegion(region string) string {
	_ = region
	return DefaultBaseURL
}

func (c *Client) SetAuth(auth *TokenManager) {
	c.auth = auth
}

func (c *Client) BaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

func (c *Client) setBaseURL(u string) {
	c.mu.Lock()
	c.baseURL = u
	c.mu.Unlock()
}

func (c *Client) moduleURL(module string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL + module + ".php"
}

func (c *Client) PostForm(ctx context.Context, module string, params url.Values) ([]byte, error) {
	return c.postForm(ctx, module, params, true)
}

func (c *Client) PostFormWithoutAuth(ctx context.Context, module string, params url.Values) ([]byte, error) {
	return c.postForm(ctx, module, params, false)
}

func (c *Client) postForm(ctx context.Context, module string, params url.Values, withAuth bool) ([]byte, error) {
	var lastErr error
	authRetried := false
	maxAttempts := c.maxRetries
	if isNonRetryableForm(module, params) {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		body, status, retryAfter, err := c.doPostOnce(ctx, module, params, withAuth)
		if err == nil {
			if apiErr := decodeAPIError(body); apiErr != nil {
				if withAuth && !authRetried && maxAttempts > 1 && isAuthFailure(status, apiErr) {
					authRetried = true
					if c.auth != nil {
						c.auth.Invalidate()
					}
					lastErr = wrapHTTPError(module, status, apiErr)
					continue
				}
				return nil, wrapHTTPError(module, status, apiErr)
			}
			return body, nil
		}

		lastErr = wrapHTTPError(module, status, err)

		if withAuth && !authRetried && maxAttempts > 1 && isAuthFailure(status, err) {
			authRetried = true
			if c.auth != nil {
				c.auth.Invalidate()
			}
			continue
		}

		if maxAttempts == 1 || (!retryableStatus(status) && status != 0) {
			return nil, lastErr
		}

		// Prefer a server-provided Retry-After (e.g. on 429/503) over our own
		// backoff schedule, capped at maxRetryAfter.
		wait := backoff(attempt)
		if retryAfter > 0 {
			wait = retryAfter
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	return nil, fmt.Errorf("invapi %s: request failed after retries: %w", module, lastErr)
}

// nonRetryableActions lists (module, action) InvAPI calls that create a new
// resource with no server-side idempotency key: there is no way to tell
// "the write already applied" from "the write never happened" on retry. If
// the HTTP response for one of these is lost after InvAPI already applied
// it (timeout, dropped connection, or 5xx after a successful write), blindly
// retrying could duplicate a paid order, an IPv4 lease, a DNS record, an SSH
// key, or a tag. Reads, deletes, and idempotent sets are safe to retry.
var nonRetryableActions = map[string]map[string]bool{
	"eq":       {"order_instance": true},
	"net":      {"add_ipv4": true},
	"pdns":     {"add_domain": true, "add_dns": true},
	"ssh_keys": {"add": true},
	"tags":     {"add": true},
}

// isNonRetryableForm marks create-style InvAPI actions that must not be replayed on timeout or 5xx.
func isNonRetryableForm(module string, params url.Values) bool {
	actions, ok := nonRetryableActions[module]
	if !ok {
		return false
	}
	return actions[params.Get("action")]
}

// doPostOnce performs a single HTTP attempt. It returns the decoded
// Retry-After duration (0 if absent/unparseable) alongside the usual
// body/status/err so postForm can honor a server-requested delay.
func (c *Client) doPostOnce(ctx context.Context, module string, params url.Values, withAuth bool) ([]byte, int, time.Duration, error) {
	reqParams := cloneValues(params)
	if withAuth && reqParams.Get("token") == "" && c.auth != nil {
		token, err := c.auth.Token(ctx)
		if err != nil {
			return nil, 0, 0, err
		}
		reqParams.Set("token", token)
	}

	encoded := reqParams.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.moduleURL(module), strings.NewReader(encoded))
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(encoded)), nil
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))

	// Read at most maxResponseBodyBytes+1: if the extra byte is present, the
	// body exceeded the cap and we fail closed instead of buffering more.
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if readErr != nil {
		return nil, resp.StatusCode, retryAfter, readErr
	}
	if len(body) > maxResponseBodyBytes {
		return nil, resp.StatusCode, retryAfter, fmt.Errorf("invapi %s: response body exceeds %d byte limit", module, maxResponseBodyBytes)
	}

	if resp.StatusCode >= 400 {
		return body, resp.StatusCode, retryAfter, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(body, 512))
	}

	return body, resp.StatusCode, retryAfter, nil
}

// parseRetryAfter decodes an HTTP Retry-After header (either delay-seconds or
// an HTTP-date form) into a non-negative duration capped at maxRetryAfter.
// It returns 0 when the header is absent, negative, or unparseable.
func parseRetryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil {
		if secs <= 0 {
			return 0
		}
		d := time.Duration(secs) * time.Second
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		return d
	}
	if t, err := http.ParseTime(header); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return 0
		}
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		return d
	}
	return 0
}

func cloneValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for k, vs := range in {
		cp := make([]string, len(vs))
		copy(cp, vs)
		out[k] = cp
	}
	return out
}

func retryableStatus(status int) bool {
	switch status {
	case 0, 429, 502, 503, 504:
		return true
	default:
		return status >= 500
	}
}

func isAuthFailure(status int, err error) bool {
	if status == http.StatusUnauthorized {
		return true
	}
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"unauthorized",
		"invalid token",
		"invalid hash",
		"token expired",
		"token is invalid",
		"not authorized",
		"authentication failed",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// backoff returns an exponential-with-full-jitter delay for retry attempt
// (0-indexed): the nominal delay doubles each attempt up to a cap, and the
// actual wait is randomized within [d/2, d) so concurrent retries (e.g.
// several hostkey_server resources retrying within the same apply) do not
// all wake up and hit InvAPI at the same instant.
func backoff(attempt int) time.Duration {
	const (
		base     = 300 * time.Millisecond
		capDelay = 8 * time.Second
	)
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 10 {
		attempt = 10 // avoid shift overflow; capDelay is reached well before this
	}
	d := base * time.Duration(int64(1)<<uint(attempt))
	if d > capDelay || d <= 0 {
		d = capDelay
	}
	half := int64(d) / 2
	if half <= 0 {
		return d
	}
	n, err := rand.Int(rand.Reader, big.NewInt(half))
	if err != nil {
		return d
	}
	return time.Duration(half) + time.Duration(n.Int64())
}

func wrapHTTPError(module string, status int, err error) error {
	if err == nil {
		return nil
	}
	if status > 0 {
		return fmt.Errorf("invapi %s (HTTP %d): %w", module, status, err)
	}
	return fmt.Errorf("invapi %s: %w", module, err)
}
