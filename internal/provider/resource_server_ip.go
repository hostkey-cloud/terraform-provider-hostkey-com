package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	_ resource.Resource                = &serverIPResource{}
	_ resource.ResourceWithImportState = &serverIPResource{}
)

type serverIPResource struct {
	client *invapi.Client
}

type serverIPModel struct {
	ID       types.String `tfsdk:"id"`
	ServerID types.Int64  `tfsdk:"server_id"`
	IP       types.String `tfsdk:"ip"`
	Port     types.String `tfsdk:"port"`
	VLAN     types.Int64  `tfsdk:"vlan"`
}

func NewServerIPResource() resource.Resource {
	return &serverIPResource{}
}

func (r *serverIPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_ip"
}

func (r *serverIPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Additional IPv4 on a Hostkey server (net/add_ipv4, net/remove_ipv4). " +
			"Like Timeweb twc_server_ip: one resource ≈ one address. " +
			"Omit ip to let InvAPI pick a free address; set ip to request a specific one.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Resource id: <server_id>/<ip>.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server_id": schema.Int64Attribute{
				Description: "InvAPI server id.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					invapiServerIDValidator(),
				},
			},
			"ip": schema.StringAttribute{
				Description: "IPv4 address. Optional on create (InvAPI assigns). Required after create / for import.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					ipv4AddressValidator(),
				},
			},
			"port": schema.StringAttribute{
				Description: "Network port name (default eth0).",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					networkPortValidator(),
				},
			},
			"vlan": schema.Int64Attribute{
				Description: "VLAN from InvAPI when returned by add_ipv4.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *serverIPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serverIPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverIPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverID := int(plan.ServerID.ValueInt64())
	addReq := invapi.NetAddIPv4Request{
		ServerID: serverID,
		Amount:   1,
	}
	if !plan.Port.IsNull() && plan.Port.ValueString() != "" {
		addReq.Port = plan.Port.ValueString()
	}
	if !plan.IP.IsNull() && !plan.IP.IsUnknown() && plan.IP.ValueString() != "" {
		addReq.IP = plan.IP.ValueString()
	}

	addResp, err := r.client.NetAddIPv4(ctx, addReq)
	if err != nil {
		resp.Diagnostics.AddError("Add IPv4 failed", err.Error())
		return
	}

	ips := addResp.ParsedIPs()
	assigned := ""
	vlan := int64(0)
	if len(ips) > 0 {
		assigned = ips[0].IP
		vlan = int64(ips[0].VLAN)
	}
	if assigned == "" && addReq.IP != "" {
		assigned = addReq.IP
	}
	if assigned == "" {
		resp.Diagnostics.AddError("Add IPv4 failed", "InvAPI returned OK but no IP in response")
		return
	}

	state := serverIPModel{
		ID:       types.StringValue(fmt.Sprintf("%d/%s", serverID, assigned)),
		ServerID: types.Int64Value(int64(serverID)),
		IP:       types.StringValue(assigned),
		Port:     plan.Port,
		VLAN:     types.Int64Value(vlan),
	}
	if vlan == 0 {
		state.VLAN = types.Int64Null()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverIPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverIPModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverID := int(state.ServerID.ValueInt64())
	ip := state.IP.ValueString()
	ok, err := r.client.ServerHasIPv4(ctx, serverID, ip)
	if err != nil {
		resp.Diagnostics.AddError("Read IPv4 failed", err.Error())
		return
	}
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverIPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverIPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverIPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverIPModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.NetRemoveIPv4(ctx, int(state.ServerID.ValueInt64()), state.IP.ValueString()); err != nil {
		resp.Diagnostics.AddError("Remove IPv4 failed", err.Error())
		return
	}
}

func (r *serverIPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(validateServerIPImportID(req.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	parts := strings.SplitN(req.ID, "/", 2)
	serverID, _ := strconv.ParseInt(parts[0], 10, 64)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), serverID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip"), parts[1])...)
}
