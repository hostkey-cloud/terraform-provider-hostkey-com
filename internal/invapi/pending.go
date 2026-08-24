package invapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrPendingNotReady means this invoice/callback has no server id yet.
var ErrPendingNotReady = errors.New("pending deploy not ready")

func rejectKnownID(id, invoice int, known map[int]struct{}) error {
	if id <= 0 {
		return fmt.Errorf("invalid server id %d", id)
	}
	if _, existed := known[id]; existed {
		return fmt.Errorf("callback for invoice %d resolved to pre-existing server id %d", invoice, id)
	}
	return nil
}

func uniqueNewListID(known map[int]struct{}, ids []int) (int, error) {
	if len(known) == 0 && len(ids) > 1 {
		return 0, fmt.Errorf("missing pre-order snapshot; %d servers in eq/list, refusing to adopt an id", len(ids))
	}
	var newcomers []int
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			newcomers = append(newcomers, id)
		}
	}
	switch len(newcomers) {
	case 0:
		return 0, ErrPendingNotReady
	case 1:
		return newcomers[0], nil
	default:
		return 0, fmt.Errorf("multiple new server ids %v; need invoice callback to disambiguate", newcomers)
	}
}

// ShowHostname is the exported form of showHostname for callers outside this
// package (e.g. resource Read/Create) that need to compare the live server
// hostname reported by eq/show against the hostname that was requested.
// InvAPI's server_data / tags shape is inconsistent, so this is a best-effort
// lookup, not a guaranteed field; an empty string means "could not
// determine the live hostname from this response", not "no hostname".
func ShowHostname(show *ServerShowResponse) string {
	return showHostname(show)
}

func showHostname(show *ServerShowResponse) string {
	if show == nil {
		return ""
	}
	if got := showHostnameFromServerData(show.ServerData); got != "" {
		return got
	}
	return showHostnameFromTags(show.Tags)
}

func showHostnameFromServerData(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	// InvAPI "server_data" shape is not consistent: hostname may appear as a top-level
	// field or be nested inside other objects. We do a best-effort search for
	// well-known keys.
	var sd any
	if err := json.Unmarshal(raw, &sd); err != nil {
		return ""
	}

	// "hostname"/"server_name" are specific enough that a match anywhere in
	// the tree is trustworthy. A bare "name" key is common on unrelated nested
	// objects (preset, os, location, ...); trusting it at any depth previously
	// risked correlating a pending order to the wrong server. "name" is only
	// trusted at the top level of server_data.
	preciseKeys := map[string]struct{}{
		"hostname":    {},
		"server_name": {},
	}

	var walk func(v any) string
	walk = func(v any) string {
		switch t := v.(type) {
		case map[string]any:
			for k, val := range t {
				if _, ok := preciseKeys[strings.ToLower(k)]; !ok {
					continue
				}
				if s, ok := val.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						return s
					}
				}
			}
			for _, val := range t {
				if got := walk(val); got != "" {
					return got
				}
			}
			return ""
		case []any:
			for _, val := range t {
				if got := walk(val); got != "" {
					return got
				}
			}
			return ""
		default:
			return ""
		}
	}

	if got := walk(sd); got != "" {
		return got
	}

	if m, ok := sd.(map[string]any); ok {
		for k, val := range m {
			if strings.ToLower(k) != "name" {
				continue
			}
			if s, ok := val.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
	}

	return ""
}

// showHostnameFromTags reads hostname from eq/show top-level tags[] — a common
// InvAPI shape where server_data has no hostname/name but tags include
// {tag:"hostname", value:"..."} (or server_name).
func showHostnameFromTags(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var tags []Tag
	if err := json.Unmarshal(raw, &tags); err != nil {
		return ""
	}
	var hostname, serverName string
	for _, t := range tags {
		v := strings.TrimSpace(t.Value)
		if v == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(t.Tag)) {
		case "hostname":
			hostname = v
		case "server_name":
			serverName = v
		}
	}
	if hostname != "" {
		return hostname
	}
	return serverName
}

func showContainsHostname(show *ServerShowResponse, wantHostname string) bool {
	wantHostname = strings.TrimSpace(wantHostname)
	if show == nil || wantHostname == "" {
		return false
	}
	if live := showHostname(show); live != "" && strings.EqualFold(live, wantHostname) {
		return true
	}
	if len(show.ServerData) == 0 {
		return false
	}

	var sd any
	if err := json.Unmarshal(show.ServerData, &sd); err != nil {
		return false
	}

	var walk func(v any) bool
	walk = func(v any) bool {
		switch t := v.(type) {
		case map[string]any:
			for _, raw := range t {
				if walk(raw) {
					return true
				}
			}
			return false
		case []any:
			for _, raw := range t {
				if walk(raw) {
					return true
				}
			}
			return false
		case string:
			s := strings.TrimSpace(t)
			if s == "" {
				return false
			}
			// Hostnames should be unique; require exact (case-insensitive) match.
			return strings.EqualFold(s, wantHostname)
		default:
			return false
		}
	}

	return walk(sd)
}

// isInvAPIPlaceholderHostname reports whether live looks like InvAPI's default
// name before order_instance applies the requested hostname (hostkey{id}).
// Empty live is intentionally NOT treated as a placeholder here: with
// wantHostname set, an empty single newcomer must keep waiting so parallel
// Creates cannot cross-link the first unnamed server (claim registry alone
// does not pick the correct waiter).
func isInvAPIPlaceholderHostname(live string, id int) bool {
	live = strings.TrimSpace(live)
	if live == "" || id <= 0 {
		return false
	}
	return strings.EqualFold(live, "hostkey"+strconv.Itoa(id))
}

func (c *Client) matchPendingIDs(ctx context.Context, known map[int]struct{}, ids []int, wantHostname, owner string) (int, error) {
	newcomers := availableNewcomerIDs(known, ids, owner)
	wantHostname = strings.TrimSpace(wantHostname)
	switch len(newcomers) {
	case 0:
		return 0, ErrPendingNotReady
	case 1:
		// Exactly one available newcomer. When wantHostname is empty (rare;
		// Create always sets a unique default), link immediately — hostname on
		// eq/show often lags. When wantHostname is set, require a match (including
		// tags[]) before claiming so parallel Creates cannot cross-link the first
		// empty-hostname newcomer; empty/mismatched → keep waiting.
		// Exception: InvAPI often leaves the default hostkey{id} until rename;
		// treat that as "hostname not applied yet", claim the sole newcomer, and
		// let Create self-heal via eq/rename_server.
		id := newcomers[0]
		if wantHostname != "" {
			show, err := c.EQShow(ctx, id)
			if err != nil {
				return 0, ErrPendingNotReady
			}
			live := showHostname(show)
			if live == "" {
				return 0, ErrPendingNotReady
			}
			if !strings.EqualFold(live, wantHostname) && !isInvAPIPlaceholderHostname(live, id) {
				return 0, ErrPendingNotReady
			}
		}
		if !tryClaimPendingServerID(id, owner) {
			return 0, ErrPendingNotReady
		}
		return id, nil
	}

	if wantHostname == "" {
		return 0, fmt.Errorf("multiple new server ids %v; need invoice callback or hostname match to disambiguate", newcomers)
	}

	var matched []int
	for _, id := range newcomers {
		show, err := c.EQShow(ctx, id)
		if err != nil {
			continue
		}
		if showContainsHostname(show, wantHostname) {
			matched = append(matched, id)
		}
	}
	switch len(matched) {
	case 0:
		return 0, fmt.Errorf("multiple new server ids %v; none matched hostname %q", newcomers, wantHostname)
	case 1:
		if !tryClaimPendingServerID(matched[0], owner) {
			return 0, ErrPendingNotReady
		}
		return matched[0], nil
	default:
		return 0, fmt.Errorf("multiple new server ids %v matched hostname %q", matched, wantHostname)
	}
}

func (c *Client) matchPendingListID(ctx context.Context, known map[int]struct{}, wantHostname, owner string) (int, error) {
	list, listErr := c.EQList(ctx, nil)
	if listErr != nil {
		return 0, listErr
	}
	ids, idErr := list.IDs()
	if idErr != nil {
		return 0, idErr
	}
	return c.matchPendingIDs(ctx, known, ids, wantHostname, owner)
}

// LookupPendingServer is one poll for the server created by this invoice (and optional callback).
// When invoice > 0 it prefers deploy_keys/callback, but can safely fall back to
// eq/list when there is a single new server id (only if wantHostname is empty) or a
// hostname match disambiguates (server_data and/or tags[]). Before returning an id
// it claims it process-locally so concurrent waiters cannot both adopt the same newcomer.
func (c *Client) LookupPendingServer(ctx context.Context, invoice int, callback string, known map[int]struct{}, wantHostname string) (id int, resolvedCallback string, err error) {
	if known == nil {
		known = map[int]struct{}{}
	}
	callback = strings.TrimSpace(callback)
	owner := pendingClaimOwner(invoice, callback)
	if callback == "" && invoice > 0 {
		upd, updErr := c.EQUpdateServers(ctx)
		if updErr == nil {
			callback = DeployKeyForInvoice(upd.DeployKeysMap(), invoice)
			if callback != "" {
				owner = pendingClaimOwner(invoice, callback)
			}
			if callback == "" && strings.TrimSpace(wantHostname) != "" {
				ids, idErr := upd.IDs()
				if idErr == nil {
					sid, matchErr := c.matchPendingIDs(ctx, known, ids, wantHostname, owner)
					if matchErr == nil {
						return sid, "", nil
					}
					if len(ids) > 0 {
						// Fail closed for invoice-bound resolution: if update_servers already
						// has candidate ids but correlation is not yet deterministic, keep
						// waiting instead of broadening to eq/list.
						return 0, "", ErrPendingNotReady
					}
				}
			}
		}
		// update_servers may fail (e.g. stale session "Invalid hash"); fall through to
		// eq/list when hostname can disambiguate, or ErrPendingNotReady when it cannot.
	}
	if callback != "" {
		if owner == "" {
			owner = pendingClaimOwner(invoice, callback)
		}
		check, cbErr := c.CallbackCheck(ctx, callback)
		if cbErr != nil {
			// PostForm turns result=Error / AsyncKey-not-found into errors before
			// callbackTerminal can parse the body. Recover definitive cancel/fail
			// here, but try list/hostname correlation first in case deploy already
			// finished and InvAPI cleaned up the callback.
			termErr := terminalFromCallbackCheckErr(cbErr)
			gone := isCallbackKeyGone(cbErr)
			if termErr != nil || gone {
				if sid, listErr := c.matchPendingListID(ctx, known, wantHostname, owner); listErr == nil && sid > 0 {
					return sid, callback, nil
				}
				if gone {
					return 0, callback, callbackGoneTerminal(callback, cbErr)
				}
				return 0, callback, termErr
			}
			return 0, callback, cbErr
		}
		if _, termErr := callbackTerminal(check); termErr != nil {
			return 0, callback, termErr
		}
		sid := CallbackServerID(check)
		if sid == 0 {
			if invoice > 0 {
				// Prefer eq/update_servers.servers ids: it is often a subset and
				// frequently yields a single newcomer id even when eq/list shows
				// multiple. This lets us resolve without relying on eq/show
				// hostname fields that may lag or be missing.
				upd, updErr := c.EQUpdateServers(ctx)
				if updErr == nil {
					if ids, idErr := upd.IDs(); idErr == nil {
						if matched, matchErr := c.matchPendingIDs(ctx, known, ids, wantHostname, owner); matchErr == nil {
							return matched, callback, nil
						}
						if len(ids) > 0 {
							// Concurrency-safe behavior: avoid falling back to broader
							// eq/list when update_servers cannot yet be correlated.
							return 0, callback, ErrPendingNotReady
						}
						// deploy_keys / servers empty after we already have a callback:
						// invoice may have been cancelled in the panel. Keep waiting
						// unless callbackTerminal already fired; empty list alone is
						// not definitive while the async key still exists.
					}
				}
				sid, listErr := c.matchPendingListID(ctx, known, wantHostname, owner)
				if listErr != nil {
					return 0, callback, listErr
				}
				return sid, callback, nil
			}
			return 0, callback, ErrPendingNotReady
		}
		if err := rejectKnownID(sid, invoice, known); err != nil {
			return 0, callback, err
		}
		if !tryClaimPendingServerID(sid, owner) {
			// Another concurrent waiter already claimed this id (e.g. via the
			// single-newcomer path). Keep waiting rather than double-linking.
			return 0, callback, ErrPendingNotReady
		}
		return sid, callback, nil
	}
	if invoice > 0 {
		if strings.TrimSpace(wantHostname) == "" {
			return 0, "", ErrPendingNotReady
		}
		sid, listErr := c.matchPendingListID(ctx, known, wantHostname, owner)
		if listErr != nil {
			return 0, "", listErr
		}
		return sid, "", nil
	}

	sid, matchErr := c.matchPendingListID(ctx, known, "", owner)
	if matchErr != nil {
		return 0, "", matchErr
	}
	return sid, "", nil
}

// WaitForPendingServer polls LookupPendingServer until this invoice has a server id or timeout.
// Transient InvAPI/DNS errors are retried until Timeout; terminal deploy errors stop immediately.
func (c *Client) WaitForPendingServer(ctx context.Context, invoice int, callback string, known map[int]struct{}, wantHostname string, opts WaitOptions) (id int, resolvedCallback string, err error) {
	interval := opts.PollInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error
	resolvedCallback = strings.TrimSpace(callback)

	if invoice > 0 {
		if payStatus, payErr := c.WHMCSInvoicePaymentStatus(ctx, invoice); payErr == nil && OrderAwaitsPayment(payStatus) {
			return 0, resolvedCallback, &PendingPaymentError{Invoice: invoice, Status: payStatus}
		}
	}

	for {
		sid, cb, lookErr := c.LookupPendingServer(ctx, invoice, resolvedCallback, known, wantHostname)
		if cb != "" {
			resolvedCallback = cb
		}
		if lookErr == nil && sid > 0 {
			return sid, resolvedCallback, nil
		}
		if lookErr != nil && !errors.Is(lookErr, ErrPendingNotReady) {
			lastErr = lookErr
			if IsPendingTerminal(lookErr) {
				return 0, resolvedCallback, lookErr
			}
		} else {
			lastErr = ErrPendingNotReady
		}

		if opts.OnPoll != nil {
			status := ""
			if resolvedCallback != "" {
				if check, cbErr := c.CallbackCheck(ctx, resolvedCallback); cbErr == nil {
					status = callbackStatusHint(check)
				}
			}
			if status == "" && lookErr != nil {
				status = lookErr.Error()
			}
			if status == "" {
				status = "pending"
			}
			opts.OnPoll(status)
		}

		if time.Now().After(deadline) {
			if lastErr != nil && !errors.Is(lastErr, ErrPendingNotReady) {
				return 0, resolvedCallback, fmt.Errorf("timed out waiting for invoice %d after %s: %w", invoice, timeout, lastErr)
			}
			return 0, resolvedCallback, fmt.Errorf("timed out waiting for invoice %d after %s (deploy_keys/callback not ready)", invoice, timeout)
		}
		select {
		case <-ctx.Done():
			return 0, resolvedCallback, ctx.Err()
		case <-ticker.C:
		}
	}
}
