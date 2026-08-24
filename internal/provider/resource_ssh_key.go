package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hostkey-cloud/terraform-provider-hostkey-com/internal/invapi"
)

var (
	_ resource.Resource                = &sshKeyResource{}
	_ resource.ResourceWithImportState = &sshKeyResource{}
	_ resource.ResourceWithModifyPlan  = &sshKeyResource{}
)

type sshKeyResource struct {
	client *invapi.Client
}

type sshKeyModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Key     types.String `tfsdk:"key"`
	Default types.Bool   `tfsdk:"default"`
	Created types.String `tfsdk:"created"`
}

func NewSSHKeyResource() resource.Resource {
	return &sshKeyResource{}
}

func (r *sshKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

func (r *sshKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a public SSH key in InvAPI account storage (ssh_keys). " +
			"Use the public key material here; pass the same key string to hostkey_server.ssh_key on order if needed. " +
			"InvAPI Customer tokens typically cannot edit keys — name/key/default changes force replace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "InvAPI SSH key id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Display name in InvAPI SSH key storage.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringMaxLen("name", maxSSHKeyNameLen),
				},
			},
			"key": schema.StringAttribute{
				Description: "Public SSH key (ssh-ed25519 / ssh-rsa / …). Comment after the key is optional.",
				Required:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					sshPublicKeyValidator(),
				},
			},
			"default": schema.BoolAttribute{
				Description: "Make this the account default SSH key (InvAPI params[default]).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"created": schema.StringAttribute{
				Description: "Creation timestamp from InvAPI.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *sshKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *sshKeyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan sshKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Default.IsNull() && !plan.Default.IsUnknown() && plan.Default.ValueBool() {
		resp.Diagnostics.AddAttributeWarning(
			path.Root("default"),
			"Default SSH key affects future server deploys",
			"Setting default=true makes this the account default SSH key in InvAPI. Future server orders that rely on the account default key may install this key automatically.",
		)
	}
}

func (r *sshKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sshKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	isDefault := !plan.Default.IsNull() && !plan.Default.IsUnknown() && plan.Default.ValueBool()
	created, err := r.client.SSHKeyAdd(ctx, plan.Name.ValueString(), plan.Key.ValueString(), isDefault)
	if err != nil {
		resp.Diagnostics.AddError("Create SSH key failed", err.Error())
		return
	}

	state := sshKeyModel{
		ID:      types.StringValue(strconv.Itoa(created.ID)),
		Name:    types.StringValue(created.Name),
		Key:     types.StringValue(created.Key),
		Default: types.BoolValue(created.Default != 0),
		Created: types.StringValue(created.Created),
	}
	// Prefer configured key string if API strips the comment suffix.
	if !plan.Key.IsNull() {
		state.Key = plan.Key
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *sshKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sshKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid SSH key id", err.Error())
		return
	}

	key, err := r.client.SSHKeyGet(ctx, id)
	if err != nil {
		if errors.Is(err, invapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read SSH key failed", err.Error())
		return
	}

	state.Name = types.StringValue(key.Name)
	if state.Key.IsNull() || !sshPublicKeysEquivalent(state.Key.ValueString(), key.Key) {
		state.Key = types.StringValue(key.Key)
	}
	state.Default = types.BoolValue(key.Default != 0)
	if key.Created != "" {
		state.Created = types.StringValue(key.Created)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *sshKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// name/key/default are RequiresReplace — Update should not run for those.
	var plan sshKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sshKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sshKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid SSH key id", err.Error())
		return
	}
	if err := r.client.SSHKeyDelete(ctx, id); err != nil {
		resp.Diagnostics.AddError("Delete SSH key failed", err.Error())
		return
	}
}

func (r *sshKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(validateSSHKeyImportID(req.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
