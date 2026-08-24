package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

func TestValidateRootPass(t *testing.T) {
	if err := validateRootPass("87GxtAkn5R+"); err != nil {
		t.Fatalf("valid pass rejected: %v", err)
	}
	if err := validateRootPass("short1A+"); err != nil {
		t.Fatalf("8-char valid rejected: %v", err)
	}
	if err := validateRootPass("87GxtAkn5R"); err == nil {
		t.Fatal("expected missing special char")
	}
	if err := validateRootPass("+BadStart1"); err == nil {
		t.Fatal("expected leading special rejected")
	}
	if err := validateRootPass("Bad@Pass1+"); err == nil {
		t.Fatal("expected @ rejected")
	}
}

func TestPendingID(t *testing.T) {
	const invoice = 123456
	id := pendingID(invoice)
	if id != "pending:123456" {
		t.Fatalf("got %s", id)
	}
	n, ok := parsePendingInvoice(id)
	if !ok || n != invoice {
		t.Fatalf("parse failed: %d %v", n, ok)
	}
}

func TestAcceptNewServerID(t *testing.T) {
	known := map[int]struct{}{100: {}, 200: {}}
	if err := acceptNewServerID(300, known); err != nil {
		t.Fatalf("new id: %v", err)
	}
	if err := acceptNewServerID(100, known); err == nil {
		t.Fatal("expected error for pre-existing id")
	}
	if err := acceptNewServerID(0, known); err == nil {
		t.Fatal("expected error for zero id")
	}
}

func TestSnapshotKnownIDs(t *testing.T) {
	_, err := snapshotKnownIDs(nil)
	if err == nil {
		t.Fatal("nil list should error")
	}
	list := &invapi.ServerListResponse{Servers: []byte(`[10,20]`)}
	known, err := snapshotKnownIDs(list)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := known[10]; !ok {
		t.Fatal("missing 10")
	}
	empty := &invapi.ServerListResponse{Servers: []byte(`[]`)}
	known, err = snapshotKnownIDs(empty)
	if err != nil {
		t.Fatal(err)
	}
	if len(known) != 0 {
		t.Fatalf("empty list: %#v", known)
	}
}

func TestPendingInvoiceFromState(t *testing.T) {
	n, ok := pendingInvoiceFromState(serverModel{ID: types.StringValue("pending:603548")})
	if !ok || n != 603548 {
		t.Fatalf("id field: %d %v", n, ok)
	}
	n, ok = pendingInvoiceFromState(serverModel{
		ID:      types.StringValue("pending:x"),
		Invoice: types.Int64Value(99),
	})
	if !ok || n != 99 {
		t.Fatalf("invoice field: %d %v", n, ok)
	}
}

func TestKeepPendingComputedPreservesKnownPowerState(t *testing.T) {
	plan := serverModel{
		PowerState: types.StringUnknown(),
	}
	state := serverModel{
		ID:         types.StringValue("pending:123"),
		Invoice:    types.Int64Value(123),
		Status:     types.StringValue("Pending"),
		MainIPv4:   types.StringValue(""),
		PowerState: types.StringNull(),
	}

	keepPendingComputed(&plan, state)

	if !plan.PowerState.IsNull() {
		t.Fatalf("power_state should stay known-null, got %#v", plan.PowerState)
	}
}

func TestPendingCreateStateNormalizesUnknownPowerState(t *testing.T) {
	pending := serverModel{
		PowerState: types.StringUnknown(),
	}

	if pending.PowerState.IsUnknown() {
		pending.PowerState = types.StringNull()
	}

	if !pending.PowerState.IsNull() {
		t.Fatalf("expected power_state null, got %#v", pending.PowerState)
	}
}

func TestPendingDeployWarningTitleDetail_TerminalVsTimeout(t *testing.T) {
	title, detail := pendingDeployWarningTitleDetail(
		&invapi.PendingTerminalError{Message: "deploy cancelled or failed: async callback key no longer exists"},
		603548, "cb-x", "pending:603548",
	)
	if title != "Deploy cancelled or failed" {
		t.Fatalf("title=%q", title)
	}
	if !strings.Contains(detail, "will not wait the full create timeout") {
		t.Fatalf("detail=%q", detail)
	}

	title, detail = pendingDeployWarningTitleDetail(
		errors.New("timed out waiting for invoice 603548 after 1s"),
		603548, "cb-x", "pending:603548",
	)
	if title != "Deploy still in progress" {
		t.Fatalf("title=%q", title)
	}
	if !strings.Contains(detail, "Re-run apply to wait") {
		t.Fatalf("detail=%q", detail)
	}
}

type memPrivate map[string][]byte

func (m memPrivate) SetKey(_ context.Context, key string, value []byte) diag.Diagnostics {
	if m == nil {
		return nil
	}
	m[key] = append([]byte(nil), value...)
	return nil
}

func (m memPrivate) GetKey(_ context.Context, key string) ([]byte, diag.Diagnostics) {
	if m == nil {
		return nil, nil
	}
	return append([]byte(nil), m[key]...), nil
}

func TestPrivateTerminalErrorRoundTrip(t *testing.T) {
	ctx := context.Background()
	priv := memPrivate{}
	if err := setPrivateTerminalError(ctx, priv, "deploy cancelled or failed: x"); err != nil {
		t.Fatal(err)
	}
	if got := getPrivateTerminalError(ctx, priv); got != "deploy cancelled or failed: x" {
		t.Fatalf("got %q", got)
	}
	if err := setPrivateTerminalError(ctx, priv, ""); err != nil {
		t.Fatal(err)
	}
	if got := getPrivateTerminalError(ctx, priv); got != "" {
		t.Fatalf("cleared got %q", got)
	}
}
