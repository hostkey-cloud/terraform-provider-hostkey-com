package provider

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

func TestDecodePresetRequiresIDMatch(t *testing.T) {
	const wantMismatch = 42
	list, _ := json.Marshal([]presetDetail{{ID: 99, Name: "other"}})
	_, err := decodePreset(nil, list, wantMismatch)
	if err == nil {
		t.Fatal("expected error when single list row is not wantID")
	}

	p, err := decodePreset(nil, list, 99)
	if err != nil || p.ID != 99 {
		t.Fatalf("matching id: %+v err=%v", p, err)
	}

	single, _ := json.Marshal(presetDetail{ID: 1, Name: "wrong"})
	_, err = decodePreset(single, nil, wantMismatch)
	if err == nil {
		t.Fatal("expected error when single object id does not match")
	}
}

func TestCatalogHasID(t *testing.T) {
	if catalogHasID([]int{1, 2, 3}, 2) != true {
		t.Fatal("expected found")
	}
	if catalogHasID([]int{1, 2, 3}, 9) {
		t.Fatal("expected missing")
	}
}

func TestFilterActiveNamedIDs(t *testing.T) {
	items := []namedID{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}
	out := filterActiveNamedIDs(items, []int{0, 1})
	if len(out) != 1 || out[0].ID != 2 {
		t.Fatalf("got %+v", out)
	}
	allZero := filterActiveNamedIDs(items, []int{0, 0})
	if len(allZero) != 0 {
		t.Fatalf("expected empty when all inactive: %+v", allZero)
	}
}

func TestValidatePlanAgainstCatalogPreset(t *testing.T) {
	promo := invapi.Preset{ID: 243, Name: "bm.v2-promo", HDD: "1000", Description: "BM EPYC 3151/32/1TB NVMe", Virtual: 0}
	two := invapi.Preset{ID: 21, Name: "bm.v2-max", HDD: "2x1000", Description: "BM E2288G/128/2x960GB SSD", Virtual: 0}
	vm := invapi.Preset{ID: 1, Name: "vm.pico", HDD: "20", Description: "VM", Virtual: 1}

	planHBA := serverModel{DiskMirror: types.StringValue("hba")}
	if err := validatePlanAgainstCatalogPreset(planHBA, promo); err == nil {
		t.Fatal("1-disk catalog + hba must error")
	}
	if err := validatePlanAgainstCatalogPreset(serverModel{}, promo); err != nil {
		t.Fatalf("omit disk_mirror on 1-disk: %v", err)
	}
	if err := validatePlanAgainstCatalogPreset(serverModel{DiskMirror: types.StringValue("raid1")}, two); err != nil {
		t.Fatalf("2-disk raid1: %v", err)
	}
	if err := validatePlanAgainstCatalogPreset(serverModel{DiskMirror: types.StringValue("raid10")}, two); err == nil {
		t.Fatal("2-disk raid10 must error")
	}
	if err := validatePlanAgainstCatalogPreset(planHBA, vm); err == nil {
		t.Fatal("disk_mirror on virtual preset must error")
	}
}
