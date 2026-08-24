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

var _ datasource.DataSource = &dnsDomainsDataSource{}

type dnsDomainsDataSource struct {
	client *invapi.Client
}

type dnsDomainsDataSourceModel struct {
	Domains types.List `tfsdk:"domains"`
}

func NewDNSDomainsDataSource() datasource.DataSource {
	return &dnsDomainsDataSource{}
}

func (d *dnsDomainsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_domains"
}

func (d *dnsDomainsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List Hostkey DNS domains (pdns/list_domains).",
		Attributes: map[string]schema.Attribute{
			"domains": schema.ListAttribute{
				Computed: true,
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"id":   types.Int64Type,
						"name": types.StringType,
					},
				},
			},
		},
	}
}

func (d *dnsDomainsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *dnsDomainsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config dnsDomainsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	listed, err := d.client.PDNSListDomains(ctx)
	if err != nil {
		resp.Diagnostics.AddError("List DNS domains failed", err.Error())
		return
	}
	objType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"id": types.Int64Type, "name": types.StringType,
	}}
	vals := make([]attr.Value, 0, len(listed))
	for _, dmn := range listed {
		vals = append(vals, types.ObjectValueMust(objType.AttrTypes, map[string]attr.Value{
			"id":   types.Int64Value(int64(dmn.ID)),
			"name": types.StringValue(dmn.Name),
		}))
	}
	config.Domains = types.ListValueMust(objType, vals)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
