package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

var (
	_ resource.Resource                = &nnaUserResource{}
	_ resource.ResourceWithConfigure   = &nnaUserResource{}
	_ resource.ResourceWithImportState = &nnaUserResource{}
)

func NewNNAUserResource() resource.Resource {
	return &nnaUserResource{}
}

type nnaUserResource struct {
	client *nna.Client
}

// nnaUserModel's Password is write-only: NNA's /api/v1/users never returns
// it (or the confirm_password NewUser/UpdateUser derive from it) from a GET,
// the same permanently-write-only shape CLAUDE.md quirk 15 documents for
// nagios_user. modelFromNNAUser below never assigns it, so it survives a
// refresh unchanged - only an explicit config change is ever applied.
type nnaUserModel struct {
	ID                 types.Int64  `tfsdk:"id"`
	Username           types.String `tfsdk:"username"`
	Password           types.String `tfsdk:"password"`
	Email              types.String `tfsdk:"email"`
	RoleID             types.Int64  `tfsdk:"role_id"`
	APIAccess          types.Bool   `tfsdk:"apiaccess"`
	Theme              types.String `tfsdk:"theme"`
	Lang               types.String `tfsdk:"lang"`
	Type               types.String `tfsdk:"type"`
	ForcePasswordReset types.Bool   `tfsdk:"force_password_reset"`
	FirstName          types.String `tfsdk:"first_name"`
	LastName           types.String `tfsdk:"last_name"`
	Company            types.String `tfsdk:"company"`
	Phone              types.String `tfsdk:"phone"`
	Active             types.Bool   `tfsdk:"active"`
	AuthServerID       types.Int64  `tfsdk:"auth_server_id"`
	APIKey             types.String `tfsdk:"apikey"`
	APIKeyID           types.Int64  `tfsdk:"apikey_id"`
}

func (r *nnaUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nna_user"
}

func (r *nnaUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios Network Analyzer login account (/api/v1/users) - distinct from a Nagios XI nagios_user, which is a separate application with its own accounts.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:      true,
				Description:   "The numeric ID Network Analyzer assigns this user.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"username": schema.StringAttribute{
				Required:    true,
				Description: "The login username. Must be unique.",
			},
			"password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "The user's password. Write-only: Network Analyzer never returns this from a GET, so it is not detected as drifted if changed outside Terraform - only an explicit config change is applied.",
			},
			"email": schema.StringAttribute{
				Required:    true,
				Description: "The user's email address.",
			},
			"role_id": schema.Int64Attribute{
				Required:    true,
				Description: "The numeric ID of the Network Analyzer role (e.g. the built-in Admin/User roles, ids 1 and 2 on a fresh instance) that grants this user's permissions.",
			},
			"apiaccess": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Whether this user's apikey (see the computed apikey attribute) may be used to authenticate API requests.",
			},
			"theme": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("default"),
				Description: "The UI theme for this user.",
			},
			"lang": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("en_US"),
				Description: "The user's language/locale, e.g. \"en_US\". Confirmed live: sending a bare language code like \"en\" is accepted but normalizes server-side to its full locale form (\"en_US\") on read-back, which would otherwise show as a permanent diff - always use the full locale form here.",
			},
			"type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("Local"),
				Description: "The account type, e.g. \"Local\" for a standalone account with its own password.",
			},
			"force_password_reset": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Forces the user to change their password at next login.",
			},
			"first_name": schema.StringAttribute{
				Optional:      true,
				Description:   "The user's first name. Once set, cannot be cleared back to unset through an update - confirmed live (#154), Network Analyzer's PATCH endpoint silently ignores an attempt to clear it (sending \"\", null, or omitting the key all leave the prior value in place). Removing this attribute from config after it's been set forces a replace (destroy+recreate) instead of a no-op update.",
				PlanModifiers: []planmodifier.String{nnaUserClearRequiresReplace{}},
			},
			"last_name": schema.StringAttribute{
				Optional:      true,
				Description:   "The user's last name. Same clear-forces-replace behavior as first_name - see its description.",
				PlanModifiers: []planmodifier.String{nnaUserClearRequiresReplace{}},
			},
			"company": schema.StringAttribute{
				Optional:      true,
				Description:   "The user's company. Same clear-forces-replace behavior as first_name - see its description.",
				PlanModifiers: []planmodifier.String{nnaUserClearRequiresReplace{}},
			},
			"phone": schema.StringAttribute{
				Optional:      true,
				Description:   "The user's phone number. Same clear-forces-replace behavior as first_name - see its description.",
				PlanModifiers: []planmodifier.String{nnaUserClearRequiresReplace{}},
			},
			"active": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether this user account is active. A disabled (inactive) user cannot log in.",
			},
			"auth_server_id": schema.Int64Attribute{
				Optional:      true,
				Description:   "The numeric ID of the Network Analyzer auth server this user authenticates against, for a non-local account. Omit for a local account. Likely shares first_name's clear-forces-replace behavior (inferred, not separately confirmed live: it's nullable through the identical PATCH partial-update code path) - removing it from config after it's been set forces a replace.",
				PlanModifiers: []planmodifier.Int64{nnaUserClearRequiresReplaceInt64{}},
			},
			"apikey": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "The API key Network Analyzer generates for this user, usable for API authentication when apiaccess is true. Read-only: cannot be set through this resource.",
			},
			"apikey_id": schema.Int64Attribute{
				Computed:    true,
				Description: "The numeric ID of this user's generated API key.",
			},
		},
	}
}

func (r *nnaUserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	nnaClient := nnaClientFrom(req.ProviderData, &resp.Diagnostics)
	if nnaClient == nil {
		return
	}
	r.client = nnaClient
}

func (r *nnaUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nnaUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u := nnaUserFromModel(&plan)

	created, err := r.client.NewUser(ctx, u)
	if err != nil {
		resp.Diagnostics.AddError("Error creating NNA user", err.Error())
		return
	}
	if created == nil {
		resp.Diagnostics.AddError("NNA user not found after create", fmt.Sprintf("User %q was created but not found by id on read-back.", u.Username))
		return
	}

	modelFromNNAUser(&plan, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nnaUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetUser(ctx, state.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA user", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	modelFromNNAUser(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nnaUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nnaUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u := nnaUserFromModel(&plan)
	// nna.User.MarshalJSON derives confirm_password from Password and omits
	// both when Password is empty - clearing it here whenever the plan
	// matches prior state means an unrelated field edit (theme, active,
	// etc.) no longer resends the real password on every apply; it's only
	// sent again on an apply that actually rotates it. Mirrors
	// resource_user.go's identical mitigation for nagios_user.
	if plan.Password.ValueString() == state.Password.ValueString() {
		u.Password = ""
	}

	updated, err := r.client.UpdateUser(ctx, state.ID.ValueInt64(), u)
	if err != nil {
		resp.Diagnostics.AddError("Error updating NNA user", err.Error())
		return
	}
	if updated == nil {
		resp.Diagnostics.AddError("NNA user not found after update", fmt.Sprintf("User id %d was updated but not found on read-back.", state.ID.ValueInt64()))
		return
	}

	modelFromNNAUser(&plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nnaUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteUser(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting NNA user", err.Error())
	}
}

// ImportState parses the import ID as a numeric user id, mirroring
// nagios_nna_source/nagios_nna_source_group - resource.ImportStatePassthroughID
// can't be used directly since it writes the raw string import ID into the
// target attribute without converting it to the schema's int64 type.
func (r *nnaUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected a numeric NNA user id, got %q: %s", req.ID, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func nnaUserFromModel(m *nnaUserModel) *nna.User {
	authServerID := int64(0)
	if !m.AuthServerID.IsNull() && !m.AuthServerID.IsUnknown() {
		authServerID = m.AuthServerID.ValueInt64()
	}

	forcePasswordReset := 0
	if m.ForcePasswordReset.ValueBool() {
		forcePasswordReset = 1
	}

	return &nna.User{
		Username:           m.Username.ValueString(),
		Password:           m.Password.ValueString(),
		Email:              m.Email.ValueString(),
		RoleID:             m.RoleID.ValueInt64(),
		APIAccess:          m.APIAccess.ValueBool(),
		Theme:              m.Theme.ValueString(),
		Lang:               m.Lang.ValueString(),
		Type:               m.Type.ValueString(),
		ForcePasswordReset: forcePasswordReset,
		FirstName:          m.FirstName.ValueString(),
		LastName:           m.LastName.ValueString(),
		Company:            m.Company.ValueString(),
		Phone:              m.Phone.ValueString(),
		Active:             m.Active.ValueBool(),
		AuthServerID:       authServerID,
	}
}

func modelFromNNAUser(m *nnaUserModel, u *nna.User) {
	m.ID = types.Int64Value(u.ID)
	m.Username = types.StringValue(u.Username)
	m.Email = types.StringValue(u.Email)
	m.RoleID = types.Int64Value(u.RoleID)
	m.APIAccess = types.BoolValue(u.APIAccess)
	m.Theme = types.StringValue(u.Theme)
	m.Lang = types.StringValue(u.Lang)
	m.Type = types.StringValue(u.Type)
	m.ForcePasswordReset = types.BoolValue(u.ForcePasswordReset != 0)
	m.FirstName = stringOrNull(u.FirstName)
	m.LastName = stringOrNull(u.LastName)
	m.Company = stringOrNull(u.Company)
	m.Phone = stringOrNull(u.Phone)
	m.Active = types.BoolValue(u.Active)
	if u.AuthServerID == 0 {
		m.AuthServerID = types.Int64Null()
	} else {
		m.AuthServerID = types.Int64Value(u.AuthServerID)
	}
	m.APIKey = types.StringValue(u.APIKey)
	m.APIKeyID = types.Int64Value(u.APIKeyID)
}

// nnaUserClearRequiresReplace forces resource replacement when a
// previously-set string value is being cleared to null/empty. Network
// Analyzer's PATCH /api/v1/users endpoint silently ignores an attempt to
// clear first_name/last_name/company/phone back to empty once they've been
// set - confirmed live (#154): sending "", null, or omitting the key
// entirely all leave the prior value in place. A destroy+recreate is the
// only way this provider can actually reach that state, so RequiresReplace
// substitutes for an update Network Analyzer's API can't perform - the same
// role CLAUDE.md quirk 7's authserver RequiresReplace fields play for an
// update path confirmed unreachable through the live API.
type nnaUserClearRequiresReplace struct{}

func (m nnaUserClearRequiresReplace) Description(ctx context.Context) string {
	return "Requires replacement when clearing this attribute, since Network Analyzer's API can't clear it via update once set."
}

func (m nnaUserClearRequiresReplace) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m nnaUserClearRequiresReplace) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	wasSet := !req.StateValue.IsNull() && req.StateValue.ValueString() != ""
	clearing := req.PlanValue.IsNull() || req.PlanValue.ValueString() == ""
	if wasSet && clearing {
		resp.RequiresReplace = true
	}
}

// nnaUserClearRequiresReplaceInt64 is nnaUserClearRequiresReplace's Int64
// counterpart, for auth_server_id.
type nnaUserClearRequiresReplaceInt64 struct{}

func (m nnaUserClearRequiresReplaceInt64) Description(ctx context.Context) string {
	return "Requires replacement when clearing this attribute, since Network Analyzer's API can't clear it via update once set."
}

func (m nnaUserClearRequiresReplaceInt64) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m nnaUserClearRequiresReplaceInt64) PlanModifyInt64(ctx context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	wasSet := !req.StateValue.IsNull() && req.StateValue.ValueInt64() != 0
	clearing := req.PlanValue.IsNull()
	if wasSet && clearing {
		resp.RequiresReplace = true
	}
}
