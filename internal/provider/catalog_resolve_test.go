package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

// Fixture only — exercises matchNamedID (exact / partial / ambiguous / missing).
// Real OS/preset/soft/traffic catalogs come from InvAPI and change over time;
// list them with data.hostkey_oses / presets / software / traffic_plans.
func TestMatchNamedID(t *testing.T) {
	items := []namedID{
		{ID: 187, Name: "Ubuntu 22.04"},
		{ID: 237, Name: "Ubuntu 24.04"},
		{ID: 180, Name: "Debian 11"},
	}

	id, err := matchNamedID("Ubuntu 22.04", items)
	if err != nil || id != 187 {
		t.Fatalf("exact: id=%d err=%v", id, err)
	}

	id, err = matchNamedID("Debian 11", items)
	if err != nil || id != 180 {
		t.Fatalf("exact debian: id=%d err=%v", id, err)
	}

	_, err = matchNamedID("Ubuntu", items)
	if err == nil {
		t.Fatal("expected ambiguous Ubuntu")
	}

	_, err = matchNamedID("Debian", items)
	if err == nil {
		t.Fatal("expected not found for substring Debian (exact match required)")
	}

	_, err = matchNamedID("Windows", items)
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestPresetCatalogLocations(t *testing.T) {
	presets := []invapi.Preset{
		{ID: 111, Name: "bm.v1-str-8t", Locations: "NL,FI"},
		{ID: 1, Name: "vm.pico", Locations: "NL,RU,FI"},
	}
	got := presetCatalogLocations(presets, "bm.v1-str-8t")
	if got != "NL,FI" {
		t.Fatalf("locations=%q", got)
	}
	if presetCatalogLocations(presets, "missing") != "" {
		t.Fatal("expected empty for unknown name")
	}
	// Case-insensitive name match; locations stay catalog order/casing.
	if presetCatalogLocations(presets, "BM.V1-STR-8T") != "NL,FI" {
		t.Fatal("expected case-insensitive name match")
	}
}

func TestEnhancePresetNameNotFoundNilClient(t *testing.T) {
	base := fmt.Errorf(`name "bm.v1-str-8t" not found (exact catalog match required)`)
	got := enhancePresetNameNotFound(t.Context(), nil, "RU", "bm.v1-str-8t", base)
	if got.Error() != base.Error() {
		t.Fatalf("nil client must keep original error: %v", got)
	}
}

func TestMatchTrafficPlan(t *testing.T) {
	items := []trafficNamedID{
		{ID: 12, Name: "1Gbps 50TB", Price: 0},
		{ID: 33, Name: "1Gbps 50TB", Price: 0},
		{ID: 14, Name: "1Gbps unmetered", Price: 100},
		{ID: 35, Name: "1Gbps unmetered", Price: 10000},
	}

	_, err := matchTrafficPlan("1Gbps 50TB - FREE", items)
	if err == nil {
		t.Fatal("expected ambiguous FREE when two rows share name+price 0")
	}

	id, err := matchTrafficPlan("1Gbps 50TB - FREE", []trafficNamedID{
		{ID: 12, Name: "1Gbps 50TB", Price: 0},
		{ID: 35, Name: "1Gbps unmetered", Price: 10000},
	})
	if err != nil || id != 12 {
		t.Fatalf("unique free: id=%d err=%v", id, err)
	}

	id, err = matchTrafficPlan("1Gbps unmetered (10000 P)", items)
	if err != nil || id != 35 {
		t.Fatalf("10000P: id=%d err=%v", id, err)
	}

	id, err = matchTrafficPlan("1Gbps unmetered (100 P)", items)
	if err != nil || id != 14 {
		t.Fatalf("100P: id=%d err=%v", id, err)
	}

	_, err = matchTrafficPlan("1Gbps unmetered", items)
	if err == nil {
		t.Fatal("expected ambiguous unmetered without price hint")
	}

	_, err = matchTrafficPlan("1Gbps 50TB", items)
	if err == nil {
		t.Fatal("expected ambiguous duplicate same-price name without traffic_plan_id")
	}
}

func TestConfiguredIDConflict(t *testing.T) {
	if msg := configuredIDConflict(types.Int64Value(237), 187, "os", "Ubuntu 22.04"); msg == "" {
		t.Fatal("expected conflict when config os_id disagrees with catalog id for os_name")
	}
	if msg := configuredIDConflict(types.Int64Value(187), 187, "os", "Ubuntu 22.04"); msg != "" {
		t.Fatalf("matching config id must not conflict: %s", msg)
	}
	if msg := configuredIDConflict(types.Int64Null(), 187, "os", "Ubuntu 22.04"); msg != "" {
		t.Fatalf("null config id (name-only) must allow sync: %s", msg)
	}
}

func TestVerifyNamedIDPairNameOnlyChange(t *testing.T) {
	items := []namedID{
		{ID: 187, Name: "Ubuntu 22.04"},
		{ID: 237, Name: "Ubuntu 24.04"},
	}

	staleID := types.Int64Value(187)
	newName := types.StringValue("Ubuntu 24.04")
	if err := verifyNamedIDPair(newName, staleID, items, "os"); err == nil {
		t.Fatal("stale os_id must fail when os_name changed")
	}

	resolved, err := matchNamedID(newName.ValueString(), items)
	if err != nil || resolved != 237 {
		t.Fatalf("resolve: id=%d err=%v", resolved, err)
	}
	if err := verifyNamedIDPair(newName, types.Int64Value(int64(resolved)), items, "os"); err != nil {
		t.Fatalf("synced id must pass: %v", err)
	}
}
