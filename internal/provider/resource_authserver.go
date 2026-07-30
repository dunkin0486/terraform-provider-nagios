package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ resource.Resource                = &authServerResource{}
	_ resource.ResourceWithConfigure   = &authServerResource{}
	_ resource.ResourceWithImportState = &authServerResource{}
)

func NewAuthServerResource() resource.Resource {
	return &authServerResource{}
}

type authServerResource struct {
	client *client.Client
}

type authServerModel struct {
	ServerID            types.String `tfsdk:"server_id"`
	Enabled             types.Bool   `tfsdk:"enabled"`
	ConnectionMethod    types.String `tfsdk:"connection_method"`
	ADAccountSuffix     types.String `tfsdk:"ad_account_suffix"`
	ADDomainControllers types.String `tfsdk:"ad_domain_controllers"`
	BaseDN              types.String `tfsdk:"base_dn"`
	SecurityLevel       types.String `tfsdk:"security_level"`
	LDAPPort            types.String `tfsdk:"ldap_port"`
	LDAPHost            types.String `tfsdk:"ldap_host"`
}

func (r *authServerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_authserver"
}

// The Nagios REST API has no update route for authentication servers at
// all - PUT system/authserver always returns "Unknown API endpoint.",
// confirmed against a live instance (see #104). Every attribute, including
// `enabled`, is RequiresReplace: changing any of them destroys and recreates
// the resource rather than attempting an update that can never succeed.
func (r *authServerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI authentication server.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				Computed:      true,
				Description:   "The ID of the authentication server. This value is computed by Nagios",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				Default:       booldefault.StaticBool(true),
				Description:   "Determines whether or not this authentication server is enabled",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"connection_method": schema.StringAttribute{
				Optional:      true,
				Description:   "The connection method used for authentication. This value can be either 'ad' or 'ldap'",
				Validators:    []validator.String{stringvalidator.OneOf("ad", "ldap")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ad_account_suffix": schema.StringAttribute{
				Optional:      true,
				Description:   "The account suffix that should be used. This value is required when the connection method is 'ad'",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ad_domain_controllers": schema.StringAttribute{
				Optional:      true,
				Description:   "A comma separated list of domain controllers to use for Active Directory authentication",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"base_dn": schema.StringAttribute{
				Optional:      true,
				Description:   "The Base DN where the user accounts exist in AD or LDAP that will be authenticating to Nagios",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"security_level": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString("none"),
				Description:   "The security level to be used to encrypt the connection. It can be either 'none', 'ssl' or 'tls'",
				Validators:    []validator.String{stringvalidator.OneOf("none", "ssl", "tls")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ldap_port": schema.StringAttribute{
				Optional:      true,
				Description:   "The TCP port to use when connecting with LDAP",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ldap_host": schema.StringAttribute{
				Optional:      true,
				Description:   "The LDAP host name or IP address to connect to",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *authServerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.Client, got: %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	r.client = c
}

func (r *authServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan authServerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a := authServerFromModel(&plan)

	if err := r.client.NewAuthServer(ctx, a); err != nil {
		resp.Diagnostics.AddError("Error creating authentication server", err.Error())
		return
	}

	// Nagios has no natural lookup key we know ahead of a create (server_id
	// is assigned by Nagios itself), so there's no way to retry-poll for a
	// specific ID the way the other resources do. The immediate GET-after-
	// create race is far less likely here in practice: LDAP/AD servers are
	// typically created one at a time, not fanned out in rapid succession
	// like the acceptance suite's host/hostgroup tests, so this is a plain
	// GET rather than client.RetryUntilFound.
	got, err := r.client.GetAuthServer(ctx, a.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading authentication server after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Authentication server not found after create", "The authentication server was created but not visible on read-back.")
		return
	}

	modelFromAuthServer(&plan, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *authServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state authServerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetAuthServer(ctx, state.ServerID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading authentication server", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	modelFromAuthServer(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable in practice: every attribute in the schema above is
// RequiresReplace, so the framework always plans a destroy+recreate instead
// of calling this. Kept implemented (rather than erroring outright) as a
// defensive fallback in case that ever changes - resource.Resource requires
// an Update method regardless.
func (r *authServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state authServerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a := authServerFromModel(&plan)
	a.ID = state.ServerID.ValueString()
	a.ServerID = state.ServerID.ValueString()

	if err := r.client.UpdateAuthServer(ctx, a, state.ServerID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating authentication server", err.Error())
		return
	}

	got, err := r.client.GetAuthServer(ctx, a.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading authentication server after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Authentication server not found after update", fmt.Sprintf("Authentication server %q not found after update.", a.ID))
		return
	}

	modelFromAuthServer(&plan, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *authServerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state authServerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteAuthServer(ctx, state.ServerID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting authentication server", err.Error())
	}
}

func (r *authServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("server_id"), req, resp)
}

func authServerFromModel(m *authServerModel) *client.AuthServer {
	return &client.AuthServer{
		Enabled:             optionalBoolToNagios(m.Enabled),
		ConnectionMethod:    m.ConnectionMethod.ValueString(),
		ADAccountSuffix:     m.ADAccountSuffix.ValueString(),
		ADDomainControllers: m.ADDomainControllers.ValueString(),
		BaseDN:              m.BaseDN.ValueString(),
		SecurityLevel:       m.SecurityLevel.ValueString(),
		LDAPPort:            m.LDAPPort.ValueString(),
		LDAPHost:            m.LDAPHost.ValueString(),
	}
}

func modelFromAuthServer(m *authServerModel, a *client.AuthServer) {
	m.ServerID = types.StringValue(a.ServerID)
	m.Enabled = nagiosToOptionalBool(a.Enabled)
	m.ConnectionMethod = stringOrNull(a.ConnectionMethod)

	if a.ConnectionMethod == "ad" {
		m.ADAccountSuffix = stringOrNull(a.ADAccountSuffix)
		m.ADDomainControllers = stringOrNull(a.ADDomainControllers)
	} else {
		m.LDAPPort = stringOrNull(a.LDAPPort)
		m.LDAPHost = stringOrNull(a.LDAPHost)
	}

	m.BaseDN = stringOrNull(a.BaseDN)
	m.SecurityLevel = stringOrNull(a.SecurityLevel)
}
