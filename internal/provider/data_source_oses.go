package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

var _ datasource.DataSource = &osesDataSource{}

type osesDataSource struct {
	client *invapi.Client
}

type osesDataSourceModel struct {
	Location   types.String `tfsdk:"location"`
	InstanceID types.Int64  `tfsdk:"instance_id"`
	ServerID   types.Int64  `tfsdk:"server_id"`
	BillPeriod types.String `tfsdk:"bill_period"`
	Name       types.String `tfsdk:"name"`
	OSes       types.List   `tfsdk:"oses"`
}

func NewOSesDataSource() datasource.DataSource {
	return &osesDataSource{}
}

func (d *osesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oses"
}

func (d *osesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List Hostkey operating systems (os/list).",
		Attributes: map[string]schema.Attribute{
			"location": schema.StringAttribute{
				Description: "DC location filter.",
				Optional:    true,
				Validators: []validator.String{
					locationCodeValidator(),
				},
			},
			"instance_id": schema.Int64Attribute{
				Description: "Preset ID — filter OS compatible with this preset.",
				Optional:    true,
				Validators: []validator.Int64{
					int64AtLeast("instance_id", 1),
				},
			},
			"server_id": schema.Int64Attribute{
				Description: "Existing server ID — filter OS compatible with hardware.",
				Optional:    true,
				Validators: []validator.Int64{
					invapiServerIDValidator(),
				},
			},
			"bill_period": schema.StringAttribute{
				Description: "Billing period filter (monthly/hourly).",
				Optional:    true,
				Validators: []validator.String{
					oneOfStrings("bill_period", "hourly", "monthly", "quarterly", "semi-annually", "annually"),
				},
			},
			"name": schema.StringAttribute{
				Description: "Optional substring filter on OS name (case-sensitive).",
				Optional:    true,
			},
			"oses": schema.ListAttribute{
				Description: "Matching operating systems.",
				Computed:    true,
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"id":     types.Int64Type,
						"name":   types.StringType,
						"active": types.BoolType,
					},
				},
			},
		},
	}
}

func (d *osesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*invapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *invapi.Client, got %T", req.ProviderData))
		return
	}
	d.client = client
}

func (d *osesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config osesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := invapi.OSListFilter{
		Location:   config.Location.ValueString(),
		BillPeriod: config.BillPeriod.ValueString(),
	}
	if !config.InstanceID.IsNull() {
		filter.InstanceID = int(config.InstanceID.ValueInt64())
	}
	if !config.ServerID.IsNull() {
		filter.ServerID = int(config.ServerID.ValueInt64())
	}

	list, err := d.client.OSList(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("os/list failed", err.Error())
		return
	}

	wantName := config.Name.ValueString()
	objType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"id": types.Int64Type, "name": types.StringType, "active": types.BoolType,
	}}
	vals := make([]attr.Value, 0, len(list.OSList))
	for _, o := range list.OSList {
		if wantName != "" && !containsFold(o.Name, wantName) {
			continue
		}
		vals = append(vals, types.ObjectValueMust(objType.AttrTypes, map[string]attr.Value{
			"id":     types.Int64Value(int64(o.ID)),
			"name":   types.StringValue(o.Name),
			"active": types.BoolValue(o.Active != 0),
		}))
	}
	config.OSes = types.ListValueMust(objType, vals)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func containsFold(hay, needle string) bool {
	return len(needle) == 0 || (len(hay) >= len(needle) && (hay == needle || indexFold(hay, needle) >= 0))
}

func indexFold(s, substr string) int {
	ls, lsub := len(s), len(substr)
	for i := 0; i+lsub <= ls; i++ {
		if equalFoldASCII(s[i:i+lsub], substr) {
			return i
		}
	}
	return -1
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
