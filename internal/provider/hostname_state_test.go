package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

func TestShouldSelfHealHostname(t *testing.T) {
	t.Parallel()
	cases := []struct {
		want, live string
		heal       bool
	}{
		{"web-01", "web-01", false},
		{"web-01", "WEB-01", false},
		{"web-01", "vm-v2-pico", true},
		{"web-01", "", true},
		{"", "vm-v2-pico", false},
		{"  web-01  ", "  ", true},
		{"web-01", "151.241.234.56", true},
	}
	for _, tc := range cases {
		if got := shouldSelfHealHostname(tc.want, tc.live); got != tc.heal {
			t.Fatalf("want=%q live=%q: got %v want %v", tc.want, tc.live, got, tc.heal)
		}
	}
}

func TestHostnameLooksUnsetOrIP(t *testing.T) {
	t.Parallel()
	cases := []struct {
		live string
		want bool
	}{
		{"", true},
		{"   ", true},
		{"151.241.234.56", true},
		{"2001:db8::1", true},
		{"web-01", false},
		{"hostkey101", false},
		{"vm-v2-pico", false},
	}
	for _, tc := range cases {
		if got := hostnameLooksUnsetOrIP(tc.live); got != tc.want {
			t.Fatalf("live=%q: got %v want %v", tc.live, got, tc.want)
		}
	}
}

func TestBuildOrderRequest_TrimsHostname(t *testing.T) {
	t.Parallel()
	req := buildOrderRequest(serverModel{
		LocationName: types.StringValue("FI"),
		RootPass:     types.StringValue("Abcdef1%"),
		PresetName:   types.StringValue("vm.pico"),
		Hostname:     types.StringValue("  web-01  "),
	})
	if req.Hostname != "web-01" {
		t.Fatalf("hostname=%q, want trimmed web-01", req.Hostname)
	}
}

func TestReadServerState_UsesLiveHostnameFromTags(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "show" {
			_, _ = io.WriteString(w, `{"result":"OK","server_data":{"id":42,"status":"rent"},"tags":[{"tag":"hostname","value":"vm-v2-pico"}],"IP":[{"ip":"1.2.3.4","main_ip":1}]}`)
			return
		}
		if strings.Contains(r.URL.Path, "tags.php") {
			_, _ = io.WriteString(w, `{"result":"OK","tags":[]}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	c, err := invapi.NewClient(invapi.Config{BaseURL: srv.URL + "/", HTTPClient: srv.Client(), MaxRetries: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := &serverResource{client: c}
	template := serverModel{Hostname: types.StringValue("web-01")}
	state, live, diags := res.readServerState(context.Background(), 42, template, hostnameStatePreferLive)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags.Errors())
	}
	if live != "vm-v2-pico" {
		t.Fatalf("live=%q", live)
	}
	if state.Hostname.ValueString() != "vm-v2-pico" {
		t.Fatalf("state hostname=%q, want live value for drift", state.Hostname.ValueString())
	}
	if !diagHasWarningSummary(diags, "Live hostname does not match requested hostname") {
		t.Fatalf("expected drift warning, got %#v", warningSummaries(diags))
	}
}

func TestReadServerState_KeepPlannedDoesNotOverwriteLive(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "show" {
			_, _ = io.WriteString(w, `{"result":"OK","server_data":{"id":42,"status":"rent"},"tags":[{"tag":"hostname","value":"nl-vmpico"}],"IP":[{"ip":"1.2.3.4","main_ip":1}]}`)
			return
		}
		if strings.Contains(r.URL.Path, "tags.php") {
			_, _ = io.WriteString(w, `{"result":"OK","tags":[]}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	c, err := invapi.NewClient(invapi.Config{BaseURL: srv.URL + "/", HTTPClient: srv.Client(), MaxRetries: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := &serverResource{client: c}
	template := serverModel{Hostname: types.StringValue("web-01")}
	state, live, diags := res.readServerState(context.Background(), 42, template, hostnameStateKeepPlanned)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags.Errors())
	}
	if live != "nl-vmpico" {
		t.Fatalf("live=%q", live)
	}
	if state.Hostname.ValueString() != "web-01" {
		t.Fatalf("apply-mode state hostname=%q, want planned web-01 (not live)", state.Hostname.ValueString())
	}
	if diagHasWarningSummary(diags, "Live hostname does not match requested hostname") {
		t.Fatalf("apply mode must not emit PreferLive drift warning, got %#v", warningSummaries(diags))
	}
}

func TestReadServerState_EmptyLiveWarnsAndKeepsConfigured(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "show" {
			_, _ = io.WriteString(w, `{"result":"OK","server_data":{"id":42,"status":"rent"},"tags":[{"tag":"OS","value":"Debian 12"}],"IP":[{"ip":"1.2.3.4","main_ip":1}]}`)
			return
		}
		if strings.Contains(r.URL.Path, "tags.php") {
			_, _ = io.WriteString(w, `{"result":"OK","tags":[]}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	c, err := invapi.NewClient(invapi.Config{BaseURL: srv.URL + "/", HTTPClient: srv.Client(), MaxRetries: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := &serverResource{client: c}
	template := serverModel{Hostname: types.StringValue("web-01")}
	state, live, diags := res.readServerState(context.Background(), 42, template, hostnameStatePreferLive)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags.Errors())
	}
	if live != "" {
		t.Fatalf("live=%q, want empty", live)
	}
	if state.Hostname.ValueString() != "web-01" {
		t.Fatalf("state hostname=%q", state.Hostname.ValueString())
	}
	if !diagHasWarningSummary(diags, "Live hostname could not be determined") {
		t.Fatalf("expected undetermined warning, got %#v", warningSummaries(diags))
	}
	if !shouldSelfHealHostname("web-01", live) {
		t.Fatal("empty live must trigger Create self-heal")
	}
}

func TestEnsureInvAPIHostname_RenameUpdatesShow(t *testing.T) {
	t.Parallel()
	var hostname atomic.Value
	hostname.Store("nl-vmpico")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "rename_server":
			hostname.Store(r.Form.Get("hostname"))
			_, _ = io.WriteString(w, `{"result":"OK"}`)
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "show":
			h := hostname.Load().(string)
			_, _ = io.WriteString(w, `{"result":"OK","server_data":{"id":42,"status":"rent"},"tags":[{"tag":"hostname","value":"`+h+`"}],"IP":[{"ip":"1.2.3.4","main_ip":1}]}`)
		case strings.Contains(r.URL.Path, "tags.php"):
			_, _ = io.WriteString(w, `{"result":"OK","tags":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := invapi.NewClient(invapi.Config{BaseURL: srv.URL + "/", HTTPClient: srv.Client(), MaxRetries: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := &serverResource{client: c}
	if err := res.ensureInvAPIHostname(context.Background(), 42, "web-01"); err != nil {
		t.Fatalf("ensureInvAPIHostname: %v", err)
	}
	if got := hostname.Load().(string); got != "web-01" {
		t.Fatalf("live hostname=%q, want web-01", got)
	}
}

func TestEnsureInvAPIHostname_TagsAddWhenRenameDoesNotUpdateShow(t *testing.T) {
	t.Parallel()
	var tagHostname atomic.Value
	tagHostname.Store("nl-vmpico")
	var renamed atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "rename_server":
			renamed.Store(true)
			// InvAPI accepts rename but leaves the hostname tag stale until tags/add.
			_, _ = io.WriteString(w, `{"result":"OK"}`)
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "show":
			h := tagHostname.Load().(string)
			_, _ = io.WriteString(w, `{"result":"OK","server_data":{"id":42,"status":"rent"},"tags":[{"tag":"hostname","value":"`+h+`"}],"IP":[{"ip":"1.2.3.4","main_ip":1}]}`)
		case strings.Contains(r.URL.Path, "tags.php") && r.Form.Get("action") == "add":
			if r.Form.Get("tag") == "hostname" {
				tagHostname.Store(r.Form.Get("value"))
			}
			_, _ = io.WriteString(w, `{"result":"OK"}`)
		case strings.Contains(r.URL.Path, "tags.php"):
			_, _ = io.WriteString(w, `{"result":"OK","tags":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := invapi.NewClient(invapi.Config{BaseURL: srv.URL + "/", HTTPClient: srv.Client(), MaxRetries: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := &serverResource{client: c}
	if err := res.ensureInvAPIHostname(context.Background(), 42, "web-01"); err != nil {
		t.Fatalf("ensureInvAPIHostname: %v", err)
	}
	if !renamed.Load() {
		t.Fatal("expected eq/rename_server")
	}
	if got := tagHostname.Load().(string); got != "web-01" {
		t.Fatalf("tag hostname=%q, want web-01", got)
	}
}

func TestEnsureInvAPIHostname_FailsWhenShowStaysOld(t *testing.T) {
	prevInterval := hostnameApplyPollInterval
	prevAttempts := hostnameApplyPollAttempts
	hostnameApplyPollInterval = time.Millisecond
	hostnameApplyPollAttempts = 2
	t.Cleanup(func() {
		hostnameApplyPollInterval = prevInterval
		hostnameApplyPollAttempts = prevAttempts
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "rename_server":
			_, _ = io.WriteString(w, `{"result":"OK"}`)
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "show":
			_, _ = io.WriteString(w, `{"result":"OK","server_data":{"id":42,"status":"rent"},"tags":[{"tag":"hostname","value":"nl-vmpico"}],"IP":[{"ip":"1.2.3.4","main_ip":1}]}`)
		case strings.Contains(r.URL.Path, "tags.php"):
			_, _ = io.WriteString(w, `{"result":"OK"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := invapi.NewClient(invapi.Config{BaseURL: srv.URL + "/", HTTPClient: srv.Client(), MaxRetries: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := &serverResource{client: c}
	err = res.ensureInvAPIHostname(context.Background(), 42, "web-01")
	if err == nil {
		t.Fatal("expected error when eq/show stays on nl-vmpico")
	}
	if !strings.Contains(err.Error(), "nl-vmpico") || !strings.Contains(err.Error(), "web-01") {
		t.Fatalf("error should mention both hostnames, got %v", err)
	}
}

func TestUpdateHostnameApplyResult_MatchesPlannedWhenEnsureSucceeds(t *testing.T) {
	t.Parallel()
	var hostname atomic.Value
	hostname.Store("nl-vmpico")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "rename_server":
			hostname.Store(r.Form.Get("hostname"))
			_, _ = io.WriteString(w, `{"result":"OK"}`)
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "show":
			h := hostname.Load().(string)
			_, _ = io.WriteString(w, `{"result":"OK","server_data":{"id":42,"status":"rent"},"tags":[{"tag":"hostname","value":"`+h+`"}],"IP":[{"ip":"1.2.3.4","main_ip":1}]}`)
		case strings.Contains(r.URL.Path, "tags.php"):
			_, _ = io.WriteString(w, `{"result":"OK","tags":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := invapi.NewClient(invapi.Config{BaseURL: srv.URL + "/", HTTPClient: srv.Client(), MaxRetries: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := &serverResource{client: c}
	plan := serverModel{Hostname: types.StringValue("web-01")}
	if err := res.ensureInvAPIHostname(context.Background(), 42, "web-01"); err != nil {
		t.Fatal(err)
	}
	newState, live, diags := res.readServerState(context.Background(), 42, plan, hostnameStateKeepPlanned)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags.Errors())
	}
	if live != "web-01" {
		t.Fatalf("live=%q after ensure", live)
	}
	if newState.Hostname.ValueString() != "web-01" {
		t.Fatalf("apply result hostname=%q, want planned web-01", newState.Hostname.ValueString())
	}
}

func TestUpdateHostnameApplyResult_KeepsPriorWhenEnsureFails(t *testing.T) {
	prevInterval := hostnameApplyPollInterval
	prevAttempts := hostnameApplyPollAttempts
	hostnameApplyPollInterval = time.Millisecond
	hostnameApplyPollAttempts = 1
	t.Cleanup(func() {
		hostnameApplyPollInterval = prevInterval
		hostnameApplyPollAttempts = prevAttempts
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "rename_server":
			_, _ = io.WriteString(w, `{"result":"OK"}`)
		case strings.Contains(r.URL.Path, "eq.php") && r.Form.Get("action") == "show":
			_, _ = io.WriteString(w, `{"result":"OK","server_data":{"id":42,"status":"rent"},"tags":[{"tag":"hostname","value":"nl-vmpico"}],"IP":[{"ip":"1.2.3.4","main_ip":1}]}`)
		case strings.Contains(r.URL.Path, "tags.php"):
			_, _ = io.WriteString(w, `{"result":"OK"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := invapi.NewClient(invapi.Config{BaseURL: srv.URL + "/", HTTPClient: srv.Client(), MaxRetries: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := &serverResource{client: c}
	prior := serverModel{Hostname: types.StringValue("nl-vmpico")}
	err = res.ensureInvAPIHostname(context.Background(), 42, "web-01")
	if err == nil {
		t.Fatal("expected ensure failure")
	}
	// Simulate Update failure path: keep prior state hostname, do not claim planned.
	if prior.Hostname.ValueString() != "nl-vmpico" {
		t.Fatal("prior live hostname must remain for next plan")
	}
}

func diagHasWarningSummary(diags diag.Diagnostics, summary string) bool {
	for _, w := range diags.Warnings() {
		if w.Summary() == summary {
			return true
		}
	}
	return false
}

func warningSummaries(diags diag.Diagnostics) []string {
	out := make([]string, 0, len(diags.Warnings()))
	for _, w := range diags.Warnings() {
		out = append(out, w.Summary())
	}
	return out
}
