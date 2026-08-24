package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

func catalogHasID(ids []int, want int) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func (r *serverResource) verifyOrderCatalog(ctx context.Context, plan *serverModel) error {
	if r.client == nil {
		return nil
	}
	if plan.LocationName.IsNull() || plan.LocationName.IsUnknown() {
		return nil
	}
	location := plan.LocationName.ValueString()
	if location == "" {
		return nil
	}

	presetID := 0
	if !plan.PresetID.IsNull() && !plan.PresetID.IsUnknown() {
		presetID = int(plan.PresetID.ValueInt64())
	}

	list, err := r.client.PresetsList(ctx, invapi.PresetsListFilter{Location: location})
	if err != nil {
		return fmt.Errorf("presets/list: %w", err)
	}
	var presetName string
	var presetIDs []int
	for _, p := range list.Presets {
		presetIDs = append(presetIDs, p.ID)
		if presetID > 0 && p.ID == presetID {
			presetName = p.Name
		}
	}

	if presetID > 0 && !catalogHasID(presetIDs, presetID) {
		return fmt.Errorf("%s", enhancePresetIDNotFound(ctx, r.client, location, presetID))
	}

	if !plan.PresetName.IsNull() && !plan.PresetName.IsUnknown() && strings.TrimSpace(plan.PresetName.ValueString()) != "" {
		want := strings.TrimSpace(plan.PresetName.ValueString())
		resolved, err := matchNamedID(want, presetsToNamed(list.Presets))
		if err != nil {
			return fmt.Errorf("preset_name: %w", enhancePresetNameNotFound(ctx, r.client, location, want, err))
		}
		if presetID > 0 && resolved != presetID {
			return fmt.Errorf("preset_name %q is catalog id %d, but preset_id is %d", want, resolved, presetID)
		}
		if presetName == "" {
			presetName = want
		}
	}

	if presetName == "" && presetID <= 0 {
		return nil
	}

	p, ok := lookupPreset(list.Presets, presetID, presetName)
	if ok {
		if err := verifyPresetActive(p, list.Presets, location); err != nil {
			return err
		}
		if err := validatePlanAgainstCatalogPreset(*plan, p); err != nil {
			return err
		}
	}

	if presetID <= 0 {
		return nil
	}

	ownOS := !plan.OwnOS.IsNull() && !plan.OwnOS.IsUnknown() && plan.OwnOS.ValueBool()
	hasTemplate := !plan.OSTemplate.IsNull() && !plan.OSTemplate.IsUnknown() && strings.TrimSpace(plan.OSTemplate.ValueString()) != ""
	if !ownOS && !hasTemplate && presetID > 0 {
		osList, err := r.client.OSList(ctx, invapi.OSListFilter{Location: location, InstanceID: presetID})
		if err != nil {
			return fmt.Errorf("os/list: %w", err)
		}
		if !plan.OSID.IsNull() && !plan.OSID.IsUnknown() && plan.OSID.ValueInt64() > 0 {
			if err := verifyActiveOSID(int(plan.OSID.ValueInt64()), osList.OSList); err != nil {
				return err
			}
		}
		if err := verifyNamedIDPair(plan.OSName, plan.OSID, osToNamed(osList.OSList), "os"); err != nil {
			return err
		}
	}

	if presetID > 0 && !plan.TrafficPlanID.IsNull() && !plan.TrafficPlanID.IsUnknown() && plan.TrafficPlanID.ValueInt64() > 0 {
		tpList, err := r.client.TrafficPlansList(ctx, invapi.TrafficPlansListFilter{Location: location, InstanceID: presetID})
		if err != nil {
			return fmt.Errorf("traffic_plans/list: %w", err)
		}
		if err := verifyActiveTrafficID(int(plan.TrafficPlanID.ValueInt64()), presetID, tpList.TrafficPlans, location); err != nil {
			return err
		}
		if err := verifyTrafficNameIDPair(plan.TrafficPlanName, plan.TrafficPlanID, tpList.TrafficPlans); err != nil {
			return err
		}
	}

	if presetID > 0 && !plan.SoftID.IsNull() && !plan.SoftID.IsUnknown() && plan.SoftID.ValueInt64() > 0 {
		softList, err := r.client.SoftwareList(ctx, invapi.SoftwareListFilter{Location: location, InstanceID: presetID})
		if err != nil {
			return fmt.Errorf("software/list: %w", err)
		}
		if err := verifyActiveSoftID(int(plan.SoftID.ValueInt64()), softList.Software); err != nil {
			return err
		}
		if err := verifyNamedIDPair(plan.SoftName, plan.SoftID, softToNamed(softList.Software), "soft"); err != nil {
			return err
		}
	}

	return nil
}

func verifyPresetActive(p invapi.Preset, all []invapi.Preset, location string) error {
	if presetsUseActiveFlag(all) && p.Active == 0 {
		return fmt.Errorf("preset %q (id %d) is inactive in location %s", p.Name, p.ID, location)
	}
	return nil
}

func enhancePresetIDNotFound(ctx context.Context, client *invapi.Client, location string, presetID int) string {
	base := fmt.Sprintf("preset_id %d is not in presets/list for location %s", presetID, location)
	if client == nil || presetID <= 0 {
		return base
	}
	global, err := client.PresetsList(ctx, invapi.PresetsListFilter{})
	if err != nil {
		return base
	}
	for _, p := range global.Presets {
		if p.ID != presetID {
			continue
		}
		locs := strings.TrimSpace(p.Locations)
		if locs == "" {
			return base
		}
		return fmt.Sprintf("%s (catalog lists %q for %s — set location_name to one of those)", base, p.Name, locs)
	}
	return base
}

func presetsUseActiveFlag(presets []invapi.Preset) bool {
	for _, p := range presets {
		if p.Active != 0 {
			return true
		}
	}
	return false
}

func verifyActiveOSID(want int, list []invapi.OSEntry) error {
	for _, o := range list {
		if o.ID == want {
			if o.Active != 0 {
				return nil
			}
			return fmt.Errorf("os_id %d is inactive in catalog (OS)", want)
		}
	}
	return fmt.Errorf("os_id %d is not available in catalog (OS)", want)
}

func verifyActiveSoftID(want int, list []invapi.SoftwareEntry) error {
	for _, s := range list {
		if s.ID == want {
			if s.Active != 0 {
				return nil
			}
			return fmt.Errorf("soft_id %d is inactive in catalog (software)", want)
		}
	}
	return fmt.Errorf("soft_id %d is not available in catalog (software)", want)
}

func verifyActiveTrafficID(want, presetID int, list []invapi.TrafficPlan, location string) error {
	for _, p := range list {
		if p.ID == want {
			if p.Active != 0 {
				return nil
			}
			return fmt.Errorf("traffic_plan_id %d is inactive for preset_id %d in location %s", want, presetID, location)
		}
	}
	return fmt.Errorf("traffic_plan_id %d is not available for preset_id %d in location %s", want, presetID, location)
}

func verifyNamedIDPair(name types.String, id types.Int64, items []namedID, label string) error {
	if name.IsNull() || name.IsUnknown() || strings.TrimSpace(name.ValueString()) == "" {
		return nil
	}
	if id.IsNull() || id.IsUnknown() || id.ValueInt64() == 0 {
		return nil
	}
	want := strings.TrimSpace(name.ValueString())
	wantID := int(id.ValueInt64())
	resolved, err := matchNamedID(want, items)
	if err != nil {
		return fmt.Errorf("%s_name: %w", label, err)
	}
	if resolved != wantID {
		return fmt.Errorf("%s_name %q is catalog id %d, but %s_id is %d", label, want, resolved, label, wantID)
	}
	return nil
}

func verifyTrafficNameIDPair(name types.String, id types.Int64, plans []invapi.TrafficPlan) error {
	if name.IsNull() || name.IsUnknown() || strings.TrimSpace(name.ValueString()) == "" {
		return nil
	}
	if id.IsNull() || id.IsUnknown() || id.ValueInt64() == 0 {
		return nil
	}
	items := make([]trafficNamedID, 0, len(plans))
	for _, p := range plans {
		if p.Active == 0 {
			continue
		}
		items = append(items, trafficNamedID{ID: p.ID, Name: p.Name, Price: p.Price})
	}
	resolved, err := matchTrafficPlan(strings.TrimSpace(name.ValueString()), items)
	if err != nil {
		return fmt.Errorf("traffic_plan_name: %w", err)
	}
	if resolved != int(id.ValueInt64()) {
		return fmt.Errorf("traffic_plan_name %q is catalog id %d, but traffic_plan_id is %d", name.ValueString(), resolved, id.ValueInt64())
	}
	return nil
}

func osToNamed(list []invapi.OSEntry) []namedID {
	items := make([]namedID, 0, len(list))
	for _, o := range list {
		if o.Active == 0 {
			continue
		}
		items = append(items, namedID{ID: o.ID, Name: o.Name})
	}
	return items
}

func softToNamed(list []invapi.SoftwareEntry) []namedID {
	items := make([]namedID, 0, len(list))
	for _, s := range list {
		if s.Active == 0 {
			continue
		}
		items = append(items, namedID{ID: s.ID, Name: s.Name})
	}
	return items
}

func lookupPreset(presets []invapi.Preset, id int, name string) (invapi.Preset, bool) {
	if id > 0 {
		for _, p := range presets {
			if p.ID == id {
				return p, true
			}
		}
	}
	if strings.TrimSpace(name) != "" {
		for _, p := range presets {
			if strings.EqualFold(p.Name, name) {
				return p, true
			}
		}
	}
	return invapi.Preset{}, false
}

func validatePlanAgainstCatalogPreset(plan serverModel, p invapi.Preset) error {
	disks := invapi.DiskCount(p.HDD.String(), p.Description)
	dedicated := p.Dedicated()

	if !plan.DiskMirror.IsNull() && !plan.DiskMirror.IsUnknown() {
		if err := invapi.ValidateDiskMirror(plan.DiskMirror.ValueString(), disks, dedicated); err != nil {
			return err
		}
	}
	if !plan.NoLVM.IsNull() && !plan.NoLVM.IsUnknown() && plan.NoLVM.ValueBool() && !dedicated {
		return fmt.Errorf("no_lvm is only valid on dedicated presets (catalog virtual=0); %s is virtual=%d", p.Name, p.Virtual)
	}
	if !plan.IPv6Block.IsNull() && !plan.IPv6Block.IsUnknown() && plan.IPv6Block.ValueBool() {
		if !dedicated {
			return fmt.Errorf("ipv6_block is only valid on dedicated presets (catalog virtual=0); %s is virtual=%d", p.Name, p.Virtual)
		}
	}
	// root_size is a PERCENTAGE of total disk (1-100), not GB. Comparing it to
	// DiskCapacityGB would mix units; schema int64Between(1, 100) is the bound.
	return nil
}

func presetsToNamed(presets []invapi.Preset) []namedID {
	items := make([]namedID, 0, len(presets))
	for _, p := range presets {
		items = append(items, namedID{ID: p.ID, Name: p.Name})
	}
	return items
}
