package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

var _ datasource.DataSource = &serversDataSource{}

type serversDataSource struct {
	client *invapi.Client
}

type serversDataSourceModel struct {
	Hostname types.String `tfsdk:"hostname"`
	IDs      types.List   `tfsdk:"ids"`
	Servers  types.List   `tfsdk:"servers"`
}

var serverItemAttrTypes = map[string]attr.Type{
	"id":        types.Int64Type,
	"hostname":  types.StringType,
	"main_ipv4": types.StringType,
	"status":    types.StringType,
	"location":  types.StringType,
}

func NewServersDataSource() datasource.DataSource {
	return &serversDataSource{}
}

func (d *serversDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_servers"
}

func (d *serversDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List servers available to the InvAPI account (eq/list, details via eq/show).",
		Attributes: map[string]schema.Attribute{
			"hostname": schema.StringAttribute{
				Description: "Optional exact hostname filter (as returned by eq/show).",
				Optional:    true,
			},
			"ids": schema.ListAttribute{
				Description: "Server IDs, sorted ascending.",
				Computed:    true,
				ElementType: types.Int64Type,
			},
			"servers": schema.ListAttribute{
				Description: "Servers with basic details.",
				Computed:    true,
				ElementType: types.ObjectType{AttrTypes: serverItemAttrTypes},
			},
		},
	}
}

func (d *serversDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *serversDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config serversDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	listed, err := d.client.EQList(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError("List servers failed", err.Error())
		return
	}
	ids, err := listed.IDs()
	if err != nil {
		resp.Diagnostics.AddError("Parse eq/list failed", err.Error())
		return
	}
	sort.Ints(ids)

	wantHostname := ""
	if !config.Hostname.IsNull() {
		wantHostname = config.Hostname.ValueString()
	}

	objType := types.ObjectType{AttrTypes: serverItemAttrTypes}
	idElems := make([]attr.Value, 0, len(ids))
	srvElems := make([]attr.Value, 0, len(ids))

	for _, id := range ids {
		show, err := d.client.EQShow(ctx, id)
		if err != nil {
			resp.Diagnostics.AddWarning(
				"Server details unavailable",
				fmt.Sprintf("eq/show for server %d failed: %s", id, err),
			)
			show = nil
		}

		hostname := invapi.ShowHostname(show)
		if wantHostname != "" && hostname != wantHostname {
			continue
		}

		mainIP, location := "", ""
		if show != nil {
			mainIP = invapi.MainIPv4(show)
			location = serverLocation(show.ServerData)
		}

		obj, diags := types.ObjectValue(serverItemAttrTypes, map[string]attr.Value{
			"id":        types.Int64Value(int64(id)),
			"hostname":  types.StringValue(hostname),
			"main_ipv4": types.StringValue(mainIP),
			"status":    types.StringValue(serverStatus(show)),
			"location":  types.StringValue(location),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		idElems = append(idElems, types.Int64Value(int64(id)))
		srvElems = append(srvElems, obj)
	}

	idList, diags := types.ListValue(types.Int64Type, idElems)
	resp.Diagnostics.Append(diags...)
	srvList, diags := types.ListValue(objType, srvElems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.IDs = idList
	config.Servers = srvList
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// serverLocation is a best-effort read of the DC location from eq/show server_data.
// The server_data shape is not stable across InvAPI versions, so only well-known
// top-level keys are trusted; an empty string means "not found", not "no location".
func serverLocation(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var sd map[string]any
	if err := json.Unmarshal(raw, &sd); err != nil {
		return ""
	}
	for _, k := range []string{"location", "location_name", "Location", "dc"} {
		switch v := sd[k].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case map[string]any:
			if s, ok := v["name"].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
