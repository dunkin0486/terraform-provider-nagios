package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ resource.Resource                = &userResource{}
	_ resource.ResourceWithConfigure   = &userResource{}
	_ resource.ResourceWithImportState = &userResource{}
)

func NewUserResource() resource.Resource {
	return &userResource{}
}

type userResource struct {
	client *client.Client
}

// userModel's password/auth_level/force_pw_change/auth_type/auth_server_id
// are write-only: Nagios's system/user API accepts them on create/update but
// never returns any of them from a GET under any field name (#174).
// modelFromUser below never assigns these five fields, and Create/Read/
// Update always start from a model already populated by req.Plan.Get/
// req.State.Get - so leaving them untouched here is what makes them survive
// a refresh unchanged: a one-way apply with no drift detection, rather than
// the usual GetX-populates-everything contract every other resource in this
// provider follows.
type userModel struct {
	UserID        types.String `tfsdk:"user_id"`
	Username      types.String `tfsdk:"username"`
	Name          types.String `tfsdk:"name"`
	Email         types.String `tfsdk:"email"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	Password      types.String `tfsdk:"password"`
	AuthLevel     types.String `tfsdk:"auth_level"`
	ForcePwChange types.Bool   `tfsdk:"force_pw_change"`
	AuthType      types.String `tfsdk:"auth_type"`
	AuthServerID  types.String `tfsdk:"auth_server_id"`
}

func (r *userResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI login/admin panel user account (system/user) - distinct from a monitoring nagios_contact, which only receives alerts and has no login capability. Note: Nagios's API has no server-side filter by username for this object type, so every lookup (Create's post-write confirmation, every Read/Update, and the nagios_user data source) fetches the entire unfiltered user list; this is expected to be low-cost for the small, admin-managed set of XI panel accounts most deployments have, but may be noticeably slower with a very large number of users.",
		Attributes: map[string]schema.Attribute{
			"user_id": schema.StringAttribute{
				Computed:      true,
				Description:   "The ID of the user. This value is computed by Nagios",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"username": schema.StringAttribute{
				Required:    true,
				Description: "The login username. Mutable - Nagios supports renaming an existing user in place",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The user's full name",
			},
			"email": schema.StringAttribute{
				Required:    true,
				Description: "The user's email address. Nagios requires a valid value here on every update, even when only unrelated fields are changing",
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Determines whether or not this user account is enabled",
			},
			"password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "The user's password. Write-only: Nagios never returns this from a GET, so it is not detected as drifted if changed outside Terraform - only an explicit config change is applied",
			},
			"auth_level": schema.StringAttribute{
				Required:    true,
				Description: "The user's permission level, either \"user\" or \"admin\". Write-only, see \"password\"",
				Validators:  []validator.String{stringvalidator.OneOf("user", "admin")},
			},
			"force_pw_change": schema.BoolAttribute{
				Optional:    true,
				Description: "Forces the user to change their password at next login. Write-only, see \"password\"",
			},
			"auth_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("local"),
				Description: "Either \"local\" for a standalone account with its own password, or \"sso\" for one authenticated via an existing nagios_authserver (see auth_server_id). Write-only, see \"password\"",
				Validators:  []validator.String{stringvalidator.OneOf("local", "sso")},
			},
			"auth_server_id": schema.StringAttribute{
				Optional:    true,
				Description: "The nagios_authserver.server_id this user authenticates against. Required when auth_type is \"sso\". Write-only, see \"password\"",
			},
		},
	}
}

func (r *userResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *providerData, got: %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	r.client = pd.XI
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u := userFromModel(&plan)

	if err := r.client.NewUser(ctx, u); err != nil {
		resp.Diagnostics.AddError("Error creating user", err.Error())
		return
	}

	got, err := client.RetryUntilFound(ctx, defaultCreateRetryAttempts, defaultCreateRetryBackoff, func() (*client.User, error) {
		return r.client.GetUser(ctx, plan.Username.ValueString())
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading user after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("User not found after create", "The user was created but not visible on read-back.")
		return
	}

	modelFromUser(&plan, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetUser(ctx, state.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	modelFromUser(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u := userFromModel(&plan)
	u.ID = state.UserID.ValueString()
	// UpdateUser's PUT sends every parameter via the URL query string, this
	// client's usual PUT convention (see UpdateUser's doc comment) - unlike
	// every other object type, that string can now carry a live account
	// password. Omitting it here whenever it's unchanged (setURLParams skips
	// empty strings) means an unrelated field edit - email, enabled, etc. -
	// no longer resends the real password on every apply; it's only sent
	// again on an apply that actually rotates it, which is unavoidable given
	// Nagios's own API only accepts PUT parameters this way.
	if plan.Password.ValueString() == state.Password.ValueString() {
		u.Password = ""
	}

	if err := r.client.UpdateUser(ctx, u, state.UserID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating user", err.Error())
		return
	}

	got, err := r.client.GetUser(ctx, plan.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading user after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("User not found after update", fmt.Sprintf("User %q not found after update.", plan.Username.ValueString()))
		return
	}

	modelFromUser(&plan, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteUser(ctx, state.UserID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting user", err.Error())
	}
}

func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("username"), req, resp)
}

func userFromModel(m *userModel) *client.User {
	return &client.User{
		Username:      m.Username.ValueString(),
		Name:          m.Name.ValueString(),
		Email:         m.Email.ValueString(),
		Enabled:       optionalBoolToNagios(m.Enabled),
		Password:      m.Password.ValueString(),
		AuthLevel:     m.AuthLevel.ValueString(),
		ForcePwChange: optionalBoolToNagios(m.ForcePwChange),
		AuthType:      m.AuthType.ValueString(),
		AuthServerID:  m.AuthServerID.ValueString(),
	}
}

// modelFromUser only ever assigns the fields Nagios's GET actually returns
// (#174): user_id, username, name, email, enabled. It deliberately never
// touches password/auth_level/force_pw_change/auth_type/auth_server_id - see
// the userModel doc comment above.
func modelFromUser(m *userModel, u *client.User) {
	m.UserID = types.StringValue(u.ID)
	m.Username = types.StringValue(u.Username)
	m.Name = types.StringValue(u.Name)
	m.Email = types.StringValue(u.Email)
	m.Enabled = nagiosToOptionalBool(u.Enabled)
}
