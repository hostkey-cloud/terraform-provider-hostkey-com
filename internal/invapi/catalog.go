package invapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Catalog list endpoints (presets/list, os/list, traffic_plans/list,
// software/list) are not documented with page/limit/offset parameters in the
// InvAPI apidocs pages reviewed for this provider, and the response types
// below have no page-cursor or total-count field. Treat this as informational:
// if InvAPI silently truncates a very large catalog, the provider only sees
// the first page. The HTTP body cap (maxResponseBodyBytes) is a defensive
// backstop, not pagination. Revisit if InvAPI documents pagination later.
type OSListResponse struct {
	Result     string          `json:"result"`
	OSList     []OSEntry       `json:"os_list"`
	OSExcluded json.RawMessage `json:"os_excluded"`
}

type OSEntry struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active int    `json:"active"`
}

type TrafficPlansListResponse struct {
	Result       string        `json:"result"`
	TrafficPlans []TrafficPlan `json:"traffic_plans"`
}

type TrafficPlan struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Active    int     `json:"active"`
	Location  string  `json:"location"`
	Locations string  `json:"locations"`
	Price     float64 `json:"price"`
	MainPlan  int     `json:"main_plan"`
}

type PresetsListFilter struct {
	Location string
}

type OSListFilter struct {
	Location   string
	ServerID   int
	InstanceID int // preset id
	BillPeriod string
}

type TrafficPlansListFilter struct {
	Location   string
	ServerID   int
	InstanceID int // preset id
}

type SoftwareListFilter struct {
	Location   string
	ServerID   int
	InstanceID int
	BillPeriod string
}

type SoftwareListResponse struct {
	Result   string          `json:"result"`
	Software []SoftwareEntry `json:"software"`
}

type SoftwareEntry struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active int    `json:"active"`
}

// catalogCacheEntry is a single memoized catalog response. Catalog
// endpoints (presets/os/software/traffic_plans list) change rarely, but were
// previously called with zero caching -- e.g. resolveOrderIDs and
// verifyOrderCatalog each independently called PresetsList for the same
// plan. A short-lived cache keyed by the request filter removes this
// redundancy without risking meaningfully stale data.
type catalogCacheEntry struct {
	expires time.Time
	value   any
}

func (c *Client) catalogCacheGet(key string) (any, bool) {
	c.catalogMu.Lock()
	defer c.catalogMu.Unlock()
	entry, ok := c.catalogCache[key]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.value, true
}

func (c *Client) catalogCacheSet(key string, value any) {
	ttl := c.catalogTTL
	if ttl <= 0 {
		ttl = defaultCatalogCacheTTL
	}
	c.catalogMu.Lock()
	defer c.catalogMu.Unlock()
	if c.catalogCache == nil {
		c.catalogCache = make(map[string]catalogCacheEntry)
	}
	c.catalogCache[key] = catalogCacheEntry{expires: time.Now().Add(ttl), value: value}
}

// InvalidateCatalogCache clears all memoized presets/os/software/traffic_plans
// list responses. Call this if a test or long-running process needs to force
// a fresh catalog read.
func (c *Client) InvalidateCatalogCache() {
	c.catalogMu.Lock()
	defer c.catalogMu.Unlock()
	c.catalogCache = nil
}

func (c *Client) PresetsList(ctx context.Context, filter PresetsListFilter) (*PresetListResponse, error) {
	key := "presets:" + filter.Location
	if cached, ok := c.catalogCacheGet(key); ok {
		if resp, ok := cached.(*PresetListResponse); ok {
			return resp, nil
		}
	}

	params := url.Values{}
	params.Set("action", "list")
	if filter.Location != "" {
		params.Set("location", filter.Location)
	}
	body, err := c.PostForm(ctx, "presets", params)
	if err != nil {
		return nil, fmt.Errorf("presets/list: %w", err)
	}
	var resp PresetListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("presets/list decode: %w", err)
	}
	c.catalogCacheSet(key, &resp)
	return &resp, nil
}

func (c *Client) OSList(ctx context.Context, filter OSListFilter) (*OSListResponse, error) {
	key := fmt.Sprintf("os:%s:%d:%d:%s", filter.Location, filter.ServerID, filter.InstanceID, filter.BillPeriod)
	if cached, ok := c.catalogCacheGet(key); ok {
		if resp, ok := cached.(*OSListResponse); ok {
			return resp, nil
		}
	}

	params := url.Values{}
	params.Set("action", "list")
	if filter.Location != "" {
		params.Set("location", filter.Location)
	}
	if filter.ServerID > 0 {
		params.Set("id", strconv.Itoa(filter.ServerID))
	}
	if filter.InstanceID > 0 {
		params.Set("instance_id", strconv.Itoa(filter.InstanceID))
	}
	if filter.BillPeriod != "" {
		params.Set("bill_period", filter.BillPeriod)
	}
	body, err := c.PostForm(ctx, "os", params)
	if err != nil {
		return nil, fmt.Errorf("os/list: %w", err)
	}
	var resp OSListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("os/list decode: %w", err)
	}
	c.catalogCacheSet(key, &resp)
	return &resp, nil
}

func (c *Client) TrafficPlansList(ctx context.Context, filter TrafficPlansListFilter) (*TrafficPlansListResponse, error) {
	key := fmt.Sprintf("traffic:%s:%d:%d", filter.Location, filter.ServerID, filter.InstanceID)
	if cached, ok := c.catalogCacheGet(key); ok {
		if resp, ok := cached.(*TrafficPlansListResponse); ok {
			return resp, nil
		}
	}

	params := url.Values{}
	params.Set("action", "list")
	if filter.Location != "" {
		params.Set("location", filter.Location)
	}
	if filter.ServerID > 0 {
		params.Set("id", strconv.Itoa(filter.ServerID))
	}
	if filter.InstanceID > 0 {
		params.Set("instance", strconv.Itoa(filter.InstanceID))
	}
	// InvAPI quirk: traffic_plans/list works in public mode (no token).
	// Sending a Customer session token often returns "invalid request".
	// Docs: https://hostkey.com/documentation/apidocs/traffic_plans/#traffic_planslist
	body, err := c.PostFormWithoutAuth(ctx, "traffic_plans", params)
	if err != nil {
		return nil, fmt.Errorf("traffic_plans/list: %w", err)
	}
	var resp TrafficPlansListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("traffic_plans/list decode: %w", err)
	}
	if !strings.EqualFold(resp.Result, "OK") && resp.Result != "" {
		return nil, fmt.Errorf("traffic_plans/list: result=%s", resp.Result)
	}
	c.catalogCacheSet(key, &resp)
	return &resp, nil
}

func (c *Client) SoftwareList(ctx context.Context, filter SoftwareListFilter) (*SoftwareListResponse, error) {
	key := fmt.Sprintf("software:%s:%d:%d:%s", filter.Location, filter.ServerID, filter.InstanceID, filter.BillPeriod)
	if cached, ok := c.catalogCacheGet(key); ok {
		if resp, ok := cached.(*SoftwareListResponse); ok {
			return resp, nil
		}
	}

	params := url.Values{}
	params.Set("action", "list")
	if filter.Location != "" {
		params.Set("location", filter.Location)
	}
	if filter.ServerID > 0 {
		params.Set("id", strconv.Itoa(filter.ServerID))
	}
	if filter.InstanceID > 0 {
		params.Set("instance_id", strconv.Itoa(filter.InstanceID))
	}
	if filter.BillPeriod != "" {
		params.Set("bill_period", filter.BillPeriod)
	}
	body, err := c.PostForm(ctx, "software", params)
	if err != nil {
		return nil, fmt.Errorf("software/list: %w", err)
	}
	var resp SoftwareListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("software/list decode: %w", err)
	}
	c.catalogCacheSet(key, &resp)
	return &resp, nil
}
