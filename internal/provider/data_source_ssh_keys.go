package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

var _ datasource.DataSource = &sshKeysDataSource{}

type sshKeysDataSource struct {
	client *invapi.Client
}

type sshKeysDataSourceModel struct {
	Name types.String `tfsdk:"name"`
	Keys types.List   `tfsdk:"keys"`
}

func NewSSHKeysDataSource() datasource.DataSource {
	return &sshKeysDataSource{}
}

func (d *sshKeysDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_keys"
}

func (d *sshKeysDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List public SSH keys stored in the InvAPI account (ssh_keys/list).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Optional exact name filter.",
				Optional:    true,
			},
			"keys": schema.ListAttribute{
				Description: "Stored SSH keys.",
				Computed:    true,
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"id":      types.Int64Type,
						"name":    types.StringType,
						"key":     types.StringType,
						"default": types.BoolType,
						"created": types.StringType,
					},
				},
			},
		},
	}
}

func (d *sshKeysDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *sshKeysDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sshKeysDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	listed, err := d.client.SSHKeysList(ctx)
	if err != nil {
		resp.Diagnostics.AddError("List SSH keys failed", err.Error())
		return
	}

	wantName := ""
	if !config.Name.IsNull() {
		wantName = config.Name.ValueString()
	}

	objType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":      types.Int64Type,
			"name":    types.StringType,
			"key":     types.StringType,
			"default": types.BoolType,
			"created": types.StringType,
		},
	}
	elems := make([]attr.Value, 0, len(listed))
	for _, k := range listed {
		if wantName != "" && k.Name != wantName {
			continue
		}
		obj, diags := types.ObjectValue(objType.AttrTypes, map[string]attr.Value{
			"id":      types.Int64Value(int64(k.ID)),
			"name":    types.StringValue(k.Name),
			"key":     types.StringValue(k.Key),
			"default": types.BoolValue(k.Default != 0),
			"created": types.StringValue(k.Created),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		elems = append(elems, obj)
	}

	list, diags := types.ListValue(objType, elems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Keys = list
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
