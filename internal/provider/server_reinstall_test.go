package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNeedsReinstall(t *testing.T) {
	base := serverModel{
		OSName:   types.StringValue("Ubuntu 22.04"),
		OSID:     types.Int64Value(187),
		RootPass: types.StringValue("Abcdef1%"),
	}

	same := base
	if needsReinstall(same, base) {
		t.Fatal("expected no reinstall when unchanged")
	}

	osChange := base
	osChange.OSName = types.StringValue("Ubuntu 24.04")
	osChange.OSID = types.Int64Value(237)
	if !needsReinstall(osChange, base) {
		t.Fatal("expected reinstall on OS change")
	}

	passChange := base
	passChange.RootPass = types.StringValue("Abcdef2%")
	if !needsReinstall(passChange, base) {
		t.Fatal("expected reinstall on root_pass change")
	}

	trigger := base
	trigger.ReinstallTrigger = types.StringValue("wipe-1")
	stateNoTrig := base
	if !needsReinstall(trigger, stateNoTrig) {
		t.Fatal("expected reinstall on reinstall_trigger set")
	}

	presetOnly := base
	presetOnly.PresetName = types.StringValue("vm.mini")
	statePreset := base
	statePreset.PresetName = types.StringValue("vm.pico")
	if needsReinstall(presetOnly, statePreset) {
		t.Fatal("preset change is replace, not reinstall")
	}

	imported := base
	importedState := serverModel{
		OSName: types.StringNull(),
		OSID:   types.Int64Null(),
	}
	if needsReinstall(imported, importedState) {
		t.Fatal("imported server with null install fields must not reinstall on first apply")
	}

	importedPass := base
	importedPassState := serverModel{RootPass: types.StringNull()}
	if needsReinstall(importedPass, importedPassState) {
		t.Fatal("root_pass on imported server must not reinstall when state had no password")
	}
}

func TestBuildReinstallRequestOmitsCreateFields(t *testing.T) {
	plan := serverModel{
		LocationName:      types.StringValue("NL"),
		RootPass:          types.StringValue("Abcdef1%"),
		OSID:              types.Int64Value(187),
		SoftID:            types.Int64Value(1),
		TrafficPlanID:     types.Int64Value(59),
		IPv4Amount:        types.Int64Value(2),
		VLAN:              types.Int64Value(10),
		IPv6Block:         types.BoolValue(true),
		DiskMirror:        types.StringValue("raid1"),
		PresetName:        types.StringValue("bm.v2-promo"),
		Hostname:          types.StringValue(" web-01 "),
		SSHKey:            types.StringValue("ssh-ed25519 AAAA"),
		PostInstallScript: types.StringValue("echo hi"),
	}
	req := buildReinstallRequest(plan, 42)
	if req.ServerID != 42 {
		t.Fatalf("id=%d", req.ServerID)
	}
	if req.Preset != "" {
		t.Fatalf("preset=%q", req.Preset)
	}
	if req.TrafficPlan != 0 || req.IPv4Amount != 0 || req.VLAN != 0 || req.IPv6Block != nil {
		t.Fatalf("create-only network fields leaked: %+v", req)
	}
	if req.LocationName != "NL" {
		t.Fatalf("location_name=%q (required for reinstall OS check)", req.LocationName)
	}
	if req.OSID != 187 || req.RootPass != "Abcdef1%" || req.DiskMirror != "raid1" {
		t.Fatalf("install fields missing: %+v", req)
	}
	if req.Hostname != "web-01" {
		t.Fatalf("hostname=%q", req.Hostname)
	}
	if req.SSHKey == "" || req.PostInstallScript == "" {
		t.Fatal("ssh/script should be sent on reinstall")
	}
}
