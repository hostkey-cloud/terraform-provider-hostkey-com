package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestStringNonBlankValidator(t *testing.T) {
	v := stringNonBlank("cancellation_reason")

	var resp validator.StringResponse
	v.ValidateString(context.Background(), validator.StringRequest{
		ConfigValue: types.StringValue("   "),
	}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for blank cancellation_reason")
	}

	resp = validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		ConfigValue: types.StringValue("Project finished"),
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
}
