package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

var (
	_ resource.Resource                = &dnsDomainResource{}
	_ resource.ResourceWithImportState = &dnsDomainResource{}
)

type dnsDomainResource struct {
	client *invapi.Client
}

type dnsDomainModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	ServerID types.Int64  `tfsdk:"server_id"`
}

func NewDNSDomainResource() resource.Resource {
	return &dnsDomainResource{}
}

func (r *dnsDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_domain"
}

func (r *dnsDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Hostkey DNS domain/zone (pdns/add_domain, delete_domain). Like Timeweb DNS zone.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "InvAPI domain id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "FQDN zone name (e.g. example.com).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					dnsZoneValidator(),
				},
			},
			"server_id": schema.Int64Attribute{
				Description: "Optional InvAPI server id to associate with the domain.",
				Optional:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					invapiServerIDValidator(),
				},
			},
		},
	}
}

func (r *dnsDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*invapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *invapi.Client, got %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *dnsDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	serverID := 0
	if !plan.ServerID.IsNull() {
		serverID = int(plan.ServerID.ValueInt64())
	}
	created, err := r.client.PDNSAddDomain(ctx, plan.Name.ValueString(), serverID)
	if err != nil {
		resp.Diagnostics.AddError("Create DNS domain failed", err.Error())
		return
	}
	plan.ID = types.StringValue(strconv.Itoa(created.ID))
	plan.Name = types.StringValue(created.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	domains, err := r.client.PDNSListDomains(ctx)
	if err != nil {
		resp.Diagnostics.AddError("List DNS domains failed", err.Error())
		return
	}
	id, _ := strconv.Atoi(state.ID.ValueString())
	found := false
	for _, d := range domains {
		if d.ID == id || (state.Name.ValueString() != "" && d.Name == state.Name.ValueString()) {
			state.ID = types.StringValue(strconv.Itoa(d.ID))
			state.Name = types.StringValue(d.Name)
			found = true
			break
		}
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid domain id", err.Error())
		return
	}
	if err := r.client.PDNSDeleteDomain(ctx, id, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete DNS domain failed", err.Error())
		return
	}
}

func (r *dnsDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(validateDNSDomainImportID(req.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if n, err := strconv.Atoi(req.ID); err == nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.Itoa(n))...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
