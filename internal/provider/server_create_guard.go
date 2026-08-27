package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

const (
	pendingIDPrefix             = "pending:"
	privateKnownKey             = "known_server_ids"
	privateCallbackKey          = "order_callback"
	privateReinstallCallbackKey = "reinstall_callback"
	privateTerminalKey          = "order_terminal_error"
)

type privateData interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
}

func pendingID(invoice int) string {
	return fmt.Sprintf("%s%d", pendingIDPrefix, invoice)
}

func parsePendingInvoice(id string) (int, bool) {
	if !strings.HasPrefix(id, pendingIDPrefix) {
		return 0, false
	}
	raw := strings.TrimPrefix(id, pendingIDPrefix)
	if strings.HasPrefix(raw, "billing-") {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func snapshotKnownIDs(list *invapi.ServerListResponse) (map[int]struct{}, error) {
	if list == nil {
		return nil, fmt.Errorf("eq/list returned no payload")
	}
	ids, err := list.IDs()
	if err != nil {
		return nil, err
	}
	known := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		known[id] = struct{}{}
	}
	return known, nil
}

func setPrivateKnownIDs(ctx context.Context, priv privateData, known map[int]struct{}) error {
	if priv == nil {
		return nil
	}
	ids := keysInt(known)
	b, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	diags := priv.SetKey(ctx, privateKnownKey, b)
	if diags.HasError() {
		return fmt.Errorf("set private known ids: %s", diags[0].Detail())
	}
	return nil
}

// acceptNewServerID rejects ids that were already in eq/list before order_instance.
func acceptNewServerID(id int, known map[int]struct{}) error {
	if id <= 0 {
		return fmt.Errorf("invalid server id %d", id)
	}
	if _, existed := known[id]; existed {
		return fmt.Errorf("server id %d was already in eq/list before order (not a new deploy)", id)
	}
	return nil
}

func getPrivateKnownIDs(ctx context.Context, priv privateData) (map[int]struct{}, diag.Diagnostics) {
	out := map[int]struct{}{}
	var diags diag.Diagnostics
	if priv == nil {
		return out, diags
	}
	val, d := priv.GetKey(ctx, privateKnownKey)
	diags.Append(d...)
	if diags.HasError() || len(val) == 0 {
		return out, diags
	}
	var ids []int
	if err := json.Unmarshal(val, &ids); err != nil {
		diags.AddWarning("private state", "could not decode known_server_ids")
		return out, diags
	}
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, diags
}

func setPrivateCallback(ctx context.Context, priv privateData, callback string) error {
	callback = strings.TrimSpace(callback)
	if priv == nil || callback == "" {
		return nil
	}
	diags := priv.SetKey(ctx, privateCallbackKey, mustPrivateJSONString(callback))
	if diags.HasError() {
		return fmt.Errorf("set private callback: %s", diags[0].Detail())
	}
	return nil
}

func getPrivateCallback(ctx context.Context, priv privateData) string {
	if priv == nil {
		return ""
	}
	val, diags := priv.GetKey(ctx, privateCallbackKey)
	if diags.HasError() || len(val) == 0 {
		return ""
	}
	return parsePrivateJSONString(val)
}

func setPrivateReinstallCallback(ctx context.Context, priv privateData, callback string) error {
	callback = strings.TrimSpace(callback)
	if priv == nil {
		return nil
	}
	// Empty means "not in progress". Store JSON "" so Plugin Framework accepts the value.
	diags := priv.SetKey(ctx, privateReinstallCallbackKey, mustPrivateJSONString(callback))
	if diags.HasError() {
		return fmt.Errorf("set private reinstall callback: %s", diags[0].Detail())
	}
	return nil
}

func getPrivateReinstallCallback(ctx context.Context, priv privateData) string {
	if priv == nil {
		return ""
	}
	val, diags := priv.GetKey(ctx, privateReinstallCallbackKey)
	if diags.HasError() || len(val) == 0 {
		return ""
	}
	return parsePrivateJSONString(val)
}

func setPrivateTerminalError(ctx context.Context, priv privateData, msg string) error {
	msg = strings.TrimSpace(msg)
	if priv == nil {
		return nil
	}
	diags := priv.SetKey(ctx, privateTerminalKey, mustPrivateJSONString(msg))
	if diags.HasError() {
		if msg == "" {
			return fmt.Errorf("clear private terminal error: %s", diags[0].Detail())
		}
		return fmt.Errorf("set private terminal error: %s", diags[0].Detail())
	}
	return nil
}

func getPrivateTerminalError(ctx context.Context, priv privateData) string {
	if priv == nil {
		return ""
	}
	val, diags := priv.GetKey(ctx, privateTerminalKey)
	if diags.HasError() || len(val) == 0 {
		return ""
	}
	return parsePrivateJSONString(val)
}

// Plugin Framework private state values must be valid JSON (not raw strings).
func mustPrivateJSONString(s string) []byte {
	b, err := json.Marshal(s)
	if err != nil {
		return []byte(`""`)
	}
	return b
}

func parsePrivateJSONString(val []byte) string {
	var s string
	if err := json.Unmarshal(val, &s); err == nil {
		return strings.TrimSpace(s)
	}
	// Legacy: raw (non-JSON) bytes from older provider builds.
	return strings.TrimSpace(string(val))
}

// pendingDeployWarningTitleDetail chooses Warning title/detail for a pending wait
// outcome. Terminal cancel/fail uses a distinct title so operators do not mistake
// it for a soft timeout; state stays pending:<invoice> either way.
func pendingDeployWarningTitleDetail(err error, invoice int, callback, pendingID string) (title, detail string) {
	cb := strings.TrimSpace(callback)
	id := strings.TrimSpace(pendingID)
	if id == "" && invoice > 0 {
		id = fmt.Sprintf("%s%d", pendingIDPrefix, invoice)
	}
	if invapi.IsPendingPayment(err) {
		title = "Waiting for invoice payment"
		detail = fmt.Sprintf(
			"%v. Apply waited for payment until create timeout; Pay this invoice in the Hostkey panel (Profile в†’ Billing / Invoices; enable auto-pay from credit balance or top up). State kept as %s. Re-run terraform apply after payment вЂ” it resumes this invoice and will not place a new order. Or keep the first apply running and pay while it still shows Still creating...",
			err, id,
		)
		return title, detail
	}
	if invapi.IsPendingTerminal(err) {
		title = "Deploy cancelled or failed"
		detail = fmt.Sprintf(
			"%v. State kept as %s so terraform destroy can drop tracking (state-only; cancel the invoice in the Hostkey panel if it is still billed). Re-run apply will not place a new order and will not wait the full create timeout again.",
			err, id,
		)
		if cb != "" {
			detail = fmt.Sprintf("%s callback=%q.", detail, cb)
		}
		return title, detail
	}
	title = "Deploy still in progress"
	detail = fmt.Sprintf(
		"%v; callback=%q invoice=%d. State kept as %s. Re-run apply to wait for this invoice (will not place a new order).",
		err, cb, invoice, id,
	)
	return title, detail
}

func pendingInvoiceFromState(state serverModel) (int, bool) {
	if n, ok := parsePendingInvoice(state.ID.ValueString()); ok {
		return n, true
	}
	if !state.Invoice.IsNull() && !state.Invoice.IsUnknown() && state.Invoice.ValueInt64() > 0 {
		return int(state.Invoice.ValueInt64()), true
	}
	return 0, false
}
