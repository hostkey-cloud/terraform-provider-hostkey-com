package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

var _ datasource.DataSource = &presetDataSource{}

type presetDataSource struct {
	client *invapi.Client
}

type presetDataSourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Locations   types.String `tfsdk:"locations"`
}

func NewPresetDataSource() datasource.DataSource {
	return &presetDataSource{}
}

func (d *presetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_preset"
}

func (d *presetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a Hostkey server preset by ID (presets/show).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Preset ID from presets/list.",
				Required:    true,
				Validators: []validator.Int64{
					int64AtLeast("id", 1),
				},
			},
			"name": schema.StringAttribute{
				Description: "Preset name (e.g. vm.pico).",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Hardware description from the preset.",
				Computed:    true,
			},
			"locations": schema.StringAttribute{
				Description: "Comma-separated available locations.",
				Computed:    true,
			},
		},
	}
}

func (d *presetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*invapi.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("Expected *invapi.Client, got %T", req.ProviderData),
		)
		return
	}
	d.client = client
}

func (d *presetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config presetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := url.Values{}
	params.Set("action", "show")
	params.Set("id", strconv.FormatInt(config.ID.ValueInt64(), 10))

	body, err := d.client.PostForm(ctx, "presets", params)
	if err != nil {
		resp.Diagnostics.AddError("Read preset failed", err.Error())
		return
	}

	var raw struct {
		Result  string          `json:"result"`
		Preset  json.RawMessage `json:"preset"`
		Presets json.RawMessage `json:"presets"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		resp.Diagnostics.AddError("Decode preset failed", err.Error())
		return
	}

	preset, err := decodePreset(raw.Preset, raw.Presets, int(config.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Decode preset body failed", err.Error())
		return
	}

	state := presetDataSourceModel{
		ID:          types.Int64Value(int64(preset.ID)),
		Name:        types.StringValue(preset.Name),
		Description: types.StringValue(preset.Description),
		Locations:   types.StringValue(preset.Locations),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

type presetDetail struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Locations   string `json:"locations"`
}

func decodePreset(single, list json.RawMessage, wantID int) (presetDetail, error) {
	if len(single) > 0 && string(single) != "null" {
		var p presetDetail
		if err := json.Unmarshal(single, &p); err == nil && p.ID != 0 && (wantID == 0 || p.ID == wantID) {
			return p, nil
		}
	}
	if len(list) > 0 && string(list) != "null" {
		var presets []presetDetail
		if err := json.Unmarshal(list, &presets); err != nil {
			return presetDetail{}, err
		}
		for _, p := range presets {
			if p.ID == wantID || wantID == 0 {
				return p, nil
			}
		}
		if len(presets) == 1 && (wantID == 0 || presets[0].ID == wantID) {
			return presets[0], nil
		}
		return presetDetail{}, fmt.Errorf("preset id %d not found in response", wantID)
	}
	return presetDetail{}, fmt.Errorf("empty preset payload")
}
