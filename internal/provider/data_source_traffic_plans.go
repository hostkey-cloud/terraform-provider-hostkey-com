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

var _ datasource.DataSource = &trafficPlansDataSource{}

type trafficPlansDataSource struct {
	client *invapi.Client
}

type trafficPlansDataSourceModel struct {
	Location     types.String `tfsdk:"location"`
	InstanceID   types.Int64  `tfsdk:"instance_id"`
	ServerID     types.Int64  `tfsdk:"server_id"`
	Name         types.String `tfsdk:"name"`
	TrafficPlans types.List   `tfsdk:"traffic_plans"`
}

func NewTrafficPlansDataSource() datasource.DataSource {
	return &trafficPlansDataSource{}
}

func (d *trafficPlansDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_traffic_plans"
}

func (d *trafficPlansDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List Hostkey traffic plans (traffic_plans/list).",
		Attributes: map[string]schema.Attribute{
			"location": schema.StringAttribute{
				Description: "DC location filter.",
				Optional:    true,
				Validators: []validator.String{
					locationCodeValidator(),
				},
			},
			"instance_id": schema.Int64Attribute{
				Description: "Preset ID for compatible plans.",
				Optional:    true,
				Validators: []validator.Int64{
					int64AtLeast("instance_id", 1),
				},
			},
			"server_id": schema.Int64Attribute{
				Description: "Existing server ID for compatible plans.",
				Optional:    true,
				Validators: []validator.Int64{
					invapiServerIDValidator(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Optional substring filter on plan name.",
				Optional:    true,
			},
			"traffic_plans": schema.ListAttribute{
				Description: "Matching traffic plans.",
				Computed:    true,
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"id":        types.Int64Type,
						"name":      types.StringType,
						"active":    types.BoolType,
						"location":  types.StringType,
						"locations": types.StringType,
						"main_plan": types.BoolType,
					},
				},
			},
		},
	}
}

func (d *trafficPlansDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *trafficPlansDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config trafficPlansDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := invapi.TrafficPlansListFilter{
		Location: config.Location.ValueString(),
	}
	if !config.InstanceID.IsNull() {
		filter.InstanceID = int(config.InstanceID.ValueInt64())
	}
	if !config.ServerID.IsNull() {
		filter.ServerID = int(config.ServerID.ValueInt64())
	}

	list, err := d.client.TrafficPlansList(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError(
			"traffic_plans/list failed",
			err.Error()+"\n\nSpecify location (and optionally instance_id). "+
				"See docs/data-sources/traffic_plans.md — InvAPI traffic_plans/list: https://hostkey.com/documentation/apidocs/traffic_plans/#traffic_planslist",
		)
		return
	}

	wantName := config.Name.ValueString()
	objType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"id": types.Int64Type, "name": types.StringType, "active": types.BoolType,
		"location": types.StringType, "locations": types.StringType, "main_plan": types.BoolType,
	}}
	vals := make([]attr.Value, 0, len(list.TrafficPlans))
	for _, p := range list.TrafficPlans {
		if wantName != "" && !containsFold(p.Name, wantName) {
			continue
		}
		vals = append(vals, types.ObjectValueMust(objType.AttrTypes, map[string]attr.Value{
			"id":        types.Int64Value(int64(p.ID)),
			"name":      types.StringValue(p.Name),
			"active":    types.BoolValue(p.Active != 0),
			"location":  types.StringValue(p.Location),
			"locations": types.StringValue(p.Locations),
			"main_plan": types.BoolValue(p.MainPlan != 0),
		}))
	}
	config.TrafficPlans = types.ListValueMust(objType, vals)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
