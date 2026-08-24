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

var _ datasource.DataSource = &softwareDataSource{}

type softwareDataSource struct {
	client *invapi.Client
}

type softwareDataSourceModel struct {
	Location   types.String `tfsdk:"location"`
	InstanceID types.Int64  `tfsdk:"instance_id"`
	ServerID   types.Int64  `tfsdk:"server_id"`
	BillPeriod types.String `tfsdk:"bill_period"`
	Name       types.String `tfsdk:"name"`
	Software   types.List   `tfsdk:"software"`
}

func NewSoftwareDataSource() datasource.DataSource {
	return &softwareDataSource{}
}

func (d *softwareDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_software"
}

func (d *softwareDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List marketplace software (software/list) compatible with a preset or server.",
		Attributes: map[string]schema.Attribute{
			"location": schema.StringAttribute{
				Description: "DC location filter.",
				Optional:    true,
				Validators: []validator.String{
					locationCodeValidator(),
				},
			},
			"instance_id": schema.Int64Attribute{
				Description: "Preset ID.",
				Optional:    true,
				Validators: []validator.Int64{
					int64AtLeast("instance_id", 1),
				},
			},
			"server_id": schema.Int64Attribute{
				Description: "Existing server ID.",
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
				Description: "Optional substring filter on software name.",
				Optional:    true,
			},
			"software": schema.ListAttribute{
				Description: "Matching software entries.",
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

func (d *softwareDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *softwareDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config softwareDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := invapi.SoftwareListFilter{
		Location:   config.Location.ValueString(),
		BillPeriod: config.BillPeriod.ValueString(),
	}
	if !config.InstanceID.IsNull() {
		filter.InstanceID = int(config.InstanceID.ValueInt64())
	}
	if !config.ServerID.IsNull() {
		filter.ServerID = int(config.ServerID.ValueInt64())
	}

	list, err := d.client.SoftwareList(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("software/list failed", err.Error())
		return
	}

	wantName := config.Name.ValueString()
	objType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"id": types.Int64Type, "name": types.StringType, "active": types.BoolType,
	}}
	vals := make([]attr.Value, 0, len(list.Software))
	for _, s := range list.Software {
		if wantName != "" && !containsFold(s.Name, wantName) {
			continue
		}
		vals = append(vals, types.ObjectValueMust(objType.AttrTypes, map[string]attr.Value{
			"id":     types.Int64Value(int64(s.ID)),
			"name":   types.StringValue(s.Name),
			"active": types.BoolValue(s.Active != 0),
		}))
	}
	config.Software = types.ListValueMust(objType, vals)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
