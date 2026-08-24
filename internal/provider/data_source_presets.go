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

var _ datasource.DataSource = &presetsDataSource{}

type presetsDataSource struct {
	client *invapi.Client
}

type presetsDataSourceModel struct {
	Location types.String `tfsdk:"location"`
	Name     types.String `tfsdk:"name"`
	Presets  types.List   `tfsdk:"presets"`
}

func NewPresetsDataSource() datasource.DataSource {
	return &presetsDataSource{}
}

func (d *presetsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_presets"
}

func (d *presetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List Hostkey presets (presets/list), optionally filtered by location and name.",
		Attributes: map[string]schema.Attribute{
			"location": schema.StringAttribute{
				Description: "DC location filter (e.g. NL).",
				Optional:    true,
				Validators: []validator.String{
					locationCodeValidator(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Optional exact preset name filter (e.g. vm.pico).",
				Optional:    true,
			},
			"presets": schema.ListAttribute{
				Description: "Matching presets.",
				Computed:    true,
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"id":          types.Int64Type,
						"name":        types.StringType,
						"description": types.StringType,
						"locations":   types.StringType,
					},
				},
			},
		},
	}
}

func (d *presetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *presetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config presetsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, err := d.client.PresetsList(ctx, invapi.PresetsListFilter{
		Location: config.Location.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("presets/list failed", err.Error())
		return
	}

	wantName := config.Name.ValueString()
	vals := make([]attr.Value, 0, len(list.Presets))
	for _, p := range list.Presets {
		if wantName != "" && p.Name != wantName {
			continue
		}
		vals = append(vals, types.ObjectValueMust(
			map[string]attr.Type{
				"id":          types.Int64Type,
				"name":        types.StringType,
				"description": types.StringType,
				"locations":   types.StringType,
			},
			map[string]attr.Value{
				"id":          types.Int64Value(int64(p.ID)),
				"name":        types.StringValue(p.Name),
				"description": types.StringValue(p.Description),
				"locations":   types.StringValue(firstNonEmpty(p.Locations, p.Location)),
			},
		))
	}

	config.Presets = types.ListValueMust(
		types.ObjectType{AttrTypes: map[string]attr.Type{
			"id": types.Int64Type, "name": types.StringType, "description": types.StringType, "locations": types.StringType,
		}},
		vals,
	)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
