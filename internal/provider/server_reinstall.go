package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

const defaultUpdateTimeout = 90 * time.Minute

// needsReinstall is true when install-time fields change on an existing server.
// InvAPI reinstall: eq/order_instance with id=<server> and without preset.
func needsReinstall(plan, state serverModel) bool {
	if installStringChanged(plan.OSName, state.OSName) {
		return true
	}
	if installIntChanged(plan.OSID, state.OSID) {
		return true
	}
	if installStringChanged(plan.SoftName, state.SoftName) {
		return true
	}
	if installIntChanged(plan.SoftID, state.SoftID) {
		return true
	}
	if rootPassChanged(plan.RootPass, state.RootPass) {
		return true
	}
	if installStringChanged(plan.SSHKey, state.SSHKey) {
		return true
	}
	if installStringChanged(plan.PostInstallScript, state.PostInstallScript) {
		return true
	}
	if installStringChanged(plan.OSTemplate, state.OSTemplate) {
		return true
	}
	if installBoolChanged(plan.OwnOS, state.OwnOS) {
		return true
	}
	if installIntChanged(plan.RootSize, state.RootSize) {
		return true
	}
	if installStringChanged(plan.DiskMirror, state.DiskMirror) {
		return true
	}
	if installBoolChanged(plan.NoLVM, state.NoLVM) {
		return true
	}
	if !plan.ReinstallTrigger.IsNull() && !plan.ReinstallTrigger.IsUnknown() &&
		plan.ReinstallTrigger.ValueString() != "" &&
		!plan.ReinstallTrigger.Equal(state.ReinstallTrigger) {
		return true
	}
	return false
}

type reinstallInProgressError struct {
	callback string
	cause    error
}

func (e reinstallInProgressError) Error() string {
	return fmt.Sprintf("reinstall callback wait failed (callback=%q): %v", e.callback, e.cause)
}

func (e reinstallInProgressError) Unwrap() error { return e.cause }

// install*Changed ignore plan-vs-null state (import / first apply) so catalog fields
// declared in HCL do not trigger an unintended reinstall.
func installStringChanged(plan, state types.String) bool {
	if plan.IsUnknown() || plan.IsNull() {
		return false
	}
	if state.IsNull() || state.IsUnknown() {
		return false
	}
	return !plan.Equal(state)
}

func installIntChanged(plan, state types.Int64) bool {
	if plan.IsUnknown() || plan.IsNull() {
		return false
	}
	if state.IsNull() || state.IsUnknown() {
		return false
	}
	return !plan.Equal(state)
}

func installBoolChanged(plan, state types.Bool) bool {
	if plan.IsUnknown() || plan.IsNull() {
		return false
	}
	if state.IsNull() || state.IsUnknown() {
		return false
	}
	return !plan.Equal(state)
}

func rootPassChanged(plan, state types.String) bool {
	if plan.IsUnknown() || plan.IsNull() || plan.ValueString() == "" {
		return false
	}
	if state.IsNull() || state.IsUnknown() || state.ValueString() == "" {
		return false
	}
	return !plan.Equal(state)
}

func buildReinstallRequest(plan serverModel, serverID int) invapi.OrderInstanceRequest {
	// Reinstall is eq/order_instance with id=<server> and no preset. Do not
	// forward create-only billing/network fields (location, traffic, extra IPv4, VLAN, IPv6).
	req := invapi.OrderInstanceRequest{
		ServerID: serverID,
		RootPass: plan.RootPass.ValueString(),
		OwnOS:    !plan.OwnOS.IsNull() && plan.OwnOS.ValueBool(),
	}
	if !plan.OSID.IsNull() {
		req.OSID = int(plan.OSID.ValueInt64())
	}
	if !plan.SoftID.IsNull() {
		req.SoftID = int(plan.SoftID.ValueInt64())
	}
	if !plan.SSHKey.IsNull() {
		req.SSHKey = plan.SSHKey.ValueString()
	}
	if !plan.PostInstallScript.IsNull() {
		req.PostInstallScript = plan.PostInstallScript.ValueString()
	}
	if !plan.OSTemplate.IsNull() {
		req.OSTemplate = plan.OSTemplate.ValueString()
	}
	if !plan.DeployOptions.IsNull() {
		req.DeployOptions = plan.DeployOptions.ValueString()
	}
	if !plan.RootSize.IsNull() {
		req.RootSize = int(plan.RootSize.ValueInt64())
	}
	if !plan.DiskMirror.IsNull() && plan.DiskMirror.ValueString() != "" {
		req.DiskMirror = strings.ToLower(strings.TrimSpace(plan.DiskMirror.ValueString()))
	}
	if !plan.NoLVM.IsNull() {
		v := plan.NoLVM.ValueBool()
		req.NoLVM = &v
	}
	return req
}

func (r *serverResource) applyReinstall(ctx context.Context, serverID int, plan serverModel, respPriv privateData) error {
	if err := r.resolveOrderIDs(ctx, &plan, &plan); err != nil {
		return fmt.Errorf("catalog resolve: %w", err)
	}
	if err := r.verifyOrderCatalog(ctx, &plan); err != nil {
		return fmt.Errorf("catalog verify: %w", err)
	}

	ownOS := !plan.OwnOS.IsNull() && plan.OwnOS.ValueBool()
	if !ownOS && (plan.OSID.IsNull() || plan.OSID.ValueInt64() == 0) &&
		(plan.OSTemplate.IsNull() || plan.OSTemplate.ValueString() == "") {
		return fmt.Errorf("reinstall requires os_id/os_name (or own_os=true / os_template)")
	}

	// FAQ: refresh session server list before reinstall if id not found.
	if _, err := r.client.EQUpdateServers(ctx); err != nil {
		tflog.Warn(ctx, "eq/update_servers before reinstall failed", map[string]any{"err": err.Error()})
	}

	orderReq := buildReinstallRequest(plan, serverID)
	tflog.Info(ctx, "Starting server reinstall via eq/order_instance", map[string]any{
		"server_id": serverID,
		"os_id":     orderReq.OSID,
		"soft_id":   orderReq.SoftID,
	})

	orderResp, err := r.client.EQOrderInstance(ctx, orderReq)
	if err != nil {
		return err
	}

	if orderResp.Callback != "" {
		// Persist reinstall callback so a failed WaitForCallback won't trigger
		// a second reinstall (double wipe) on the next apply.
		if err := setPrivateReinstallCallback(ctx, respPriv, orderResp.Callback); err != nil {
			tflog.Warn(ctx, "set private reinstall callback failed", map[string]any{"err": err.Error()})
		}

		timeout, diags := plan.Timeouts.Update(ctx, defaultUpdateTimeout)
		if diags.HasError() {
			timeout = defaultUpdateTimeout
		}
		_, waitErr := r.client.WaitForCallback(ctx, orderResp.Callback, invapi.WaitOptions{
			PollInterval: pollIntervalFrom(plan),
			Timeout:      timeout,
		})
		if waitErr != nil {
			return reinstallInProgressError{callback: orderResp.Callback, cause: waitErr}
		}
	}

	// Clear callback marker (if any) on success.
	if err := setPrivateReinstallCallback(ctx, respPriv, ""); err != nil {
		tflog.Warn(ctx, "clear private reinstall callback failed", map[string]any{"err": err.Error()})
	}

	// Keep same id; confirm still visible.
	if orderResp.ID > 0 && orderResp.ID != serverID {
		return fmt.Errorf("reinstall returned unexpected id %d (expected %d)", orderResp.ID, serverID)
	}
	if _, err := r.client.EQShow(ctx, serverID); err != nil {
		return fmt.Errorf("reinstall finished but eq/show failed: %w", err)
	}
	tflog.Info(ctx, "Reinstall finished", map[string]any{
		"server_id": serverID,
		"status":    orderResp.DeployStatus,
	})
	return nil
}
