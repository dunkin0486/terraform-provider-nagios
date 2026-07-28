package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ resource.Resource                = &contactResource{}
	_ resource.ResourceWithConfigure   = &contactResource{}
	_ resource.ResourceWithImportState = &contactResource{}
)

func NewContactResource() resource.Resource {
	return &contactResource{}
}

type contactResource struct {
	client *client.Client
}

type contactModel struct {
	ContactName                 types.String `tfsdk:"contact_name"`
	HostNotificationsEnabled    types.Bool   `tfsdk:"host_notifications_enabled"`
	ServiceNotificationsEnabled types.Bool   `tfsdk:"service_notifications_enabled"`
	HostNotificationPeriod      types.String `tfsdk:"host_notification_period"`
	ServiceNotificationPeriod   types.String `tfsdk:"service_notification_period"`
	HostNotificationOptions     types.String `tfsdk:"host_notification_options"`
	ServiceNotificationOptions  types.String `tfsdk:"service_notification_options"`
	HostNotificationCommands    types.Set    `tfsdk:"host_notification_commands"`
	ServiceNotificationCommands types.Set    `tfsdk:"service_notification_commands"`
	Alias                       types.String `tfsdk:"alias"`
	ContactGroups               types.Set    `tfsdk:"contact_groups"`
	Templates                   types.Set    `tfsdk:"templates"`
	Email                       types.String `tfsdk:"email"`
	Pager                       types.String `tfsdk:"pager"`
	Address1                    types.String `tfsdk:"address1"`
	Address2                    types.String `tfsdk:"address2"`
	Address3                    types.String `tfsdk:"address3"`
	CanSubmitCommands           types.Bool   `tfsdk:"can_submit_commands"`
	RetainStatusInformation     types.Bool   `tfsdk:"retain_status_information"`
	RetainNonstatusInformation  types.Bool   `tfsdk:"retain_nonstatus_information"`
}

func (r *contactResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contact"
}

func (r *contactResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI contact.",
		Attributes: map[string]schema.Attribute{
			"contact_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the contact",
			},
			"host_notifications_enabled": schema.BoolAttribute{
				Required:    true,
				Description: "Determines whether or not the contact will receive notifications about host problems and recoveries",
			},
			"service_notifications_enabled": schema.BoolAttribute{
				Required:    true,
				Description: "Determines whether or not the contact will receive notifications about service problems and recoveries",
			},
			"host_notification_period": schema.StringAttribute{
				Required:    true,
				Description: "The short name of the time period during which the contact can be notified about host problems or recoveries",
			},
			"service_notification_period": schema.StringAttribute{
				Required:    true,
				Description: "The short name of the time period during which the contact can be notified about service problems or recoveries",
			},
			"host_notification_options": schema.StringAttribute{
				Required:    true,
				Description: "The host states for which notifications can be sent out to this contact. Valid options are a combination of one or more of the following: d = notify on DOWN host states, u = notify on UNREACHABLE host states, r = notify on host recoveries (UP states), f = notify when the host starts and stops flapping, and s = send notifications",
			},
			"service_notification_options": schema.StringAttribute{
				Required:    true,
				Description: "The service states for which notifications can be sent out to this contact. Valid options are a combination of one or more of the following: w = notify on WARNING service states, u = notify on UNKNOWN service states, c = notify on CRITICAL service states, r = notify on service recoveries (OK states), and f = notify when the service starts and stops flapping.",
			},
			"host_notification_commands": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "A list of the short names of the commands used to notify the contact of a host problem or recovery",
			},
			"service_notification_commands": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "A list of the short names of the commands used to notify the contact of a service problem or recovery",
			},
			"alias": schema.StringAttribute{
				Optional:    true,
				Description: "A longer name or description for the contact",
			},
			"contact_groups": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The short name(s) of the contactgroup(s) that the contact belongs to",
			},
			"templates": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The contact templates to apply to the contact",
			},
			"email": schema.StringAttribute{
				Optional:    true,
				Description: "Defines an email address for the contact",
			},
			"pager": schema.StringAttribute{
				Optional:    true,
				Description: "Defines a pager number for the contact",
			},
			"address1": schema.StringAttribute{
				Optional:    true,
				Description: "Defines additional 'addresses' for the contact",
			},
			"address2": schema.StringAttribute{
				Optional:    true,
				Description: "Defines additional 'addresses' for the contact",
			},
			"address3": schema.StringAttribute{
				Optional:    true,
				Description: "Defines additional 'addresses' for the contact",
			},
			"can_submit_commands": schema.BoolAttribute{
				Optional:    true,
				Description: "Determines whether or not the contact can submit external commands to Nagios from the CGIs",
			},
			"retain_status_information": schema.BoolAttribute{
				Optional:    true,
				Description: "Determines whether or not status-related information about the contact is retained across program restarts",
			},
			"retain_nonstatus_information": schema.BoolAttribute{
				Optional:    true,
				Description: "Determines whether or not non-status information about the contact is retained across program restarts.",
			},
		},
	}
}

func (r *contactResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *contactResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan contactModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	c, diags := contactFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.NewContact(ctx, c); err != nil {
		resp.Diagnostics.AddError("Error creating contact", err.Error())
		return
	}

	got, err := client.RetryUntilFound(ctx, defaultCreateRetryAttempts, defaultCreateRetryBackoff, func() (*client.Contact, error) {
		return r.client.GetContact(ctx, c.ContactName)
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading contact after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Contact not found after create", fmt.Sprintf("Contact %q was created but not visible on read-back after retries.", c.ContactName))
		return
	}

	diags = modelFromContact(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *contactResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state contactModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetContact(ctx, state.ContactName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading contact", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	diags := modelFromContact(ctx, &state, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *contactResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state contactModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	c, diags := contactFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateContact(ctx, c, state.ContactName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating contact", err.Error())
		return
	}

	got, err := r.client.GetContact(ctx, c.ContactName)
	if err != nil {
		resp.Diagnostics.AddError("Error reading contact after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Contact not found after update", fmt.Sprintf("Contact %q not found after update.", c.ContactName))
		return
	}

	diags = modelFromContact(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *contactResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state contactModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteContact(ctx, state.ContactName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting contact", err.Error())
	}
}

func (r *contactResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("contact_name"), req, resp)
}

func contactFromModel(ctx context.Context, m *contactModel) (*client.Contact, diag.Diagnostics) {
	var diags diag.Diagnostics

	hostNotificationCommands, d := setToStrings(ctx, m.HostNotificationCommands)
	diags.Append(d...)
	serviceNotificationCommands, d := setToStrings(ctx, m.ServiceNotificationCommands)
	diags.Append(d...)
	contactGroups, d := setToStrings(ctx, m.ContactGroups)
	diags.Append(d...)
	templates, d := setToStrings(ctx, m.Templates)
	diags.Append(d...)

	if diags.HasError() {
		return nil, diags
	}

	return &client.Contact{
		ContactName:                 m.ContactName.ValueString(),
		HostNotificationsEnabled:    optionalBoolToNagios(m.HostNotificationsEnabled),
		ServiceNotificationsEnabled: optionalBoolToNagios(m.ServiceNotificationsEnabled),
		HostNotificationPeriod:      m.HostNotificationPeriod.ValueString(),
		ServiceNotificationPeriod:   m.ServiceNotificationPeriod.ValueString(),
		HostNotificationOptions:     m.HostNotificationOptions.ValueString(),
		ServiceNotificationOptions:  m.ServiceNotificationOptions.ValueString(),
		HostNotificationCommands:    hostNotificationCommands,
		ServiceNotificationCommands: serviceNotificationCommands,
		Alias:                       m.Alias.ValueString(),
		ContactGroups:               contactGroups,
		Templates:                   templates,
		Email:                       m.Email.ValueString(),
		Pager:                       m.Pager.ValueString(),
		Address1:                    m.Address1.ValueString(),
		Address2:                    m.Address2.ValueString(),
		Address3:                    m.Address3.ValueString(),
		CanSubmitCommands:           optionalBoolToNagios(m.CanSubmitCommands),
		RetainStatusInformation:     optionalBoolToNagios(m.RetainStatusInformation),
		RetainNonstatusInformation:  optionalBoolToNagios(m.RetainNonstatusInformation),
	}, diags
}

func modelFromContact(ctx context.Context, m *contactModel, c *client.Contact) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ContactName = types.StringValue(c.ContactName)
	m.HostNotificationsEnabled = nagiosToOptionalBool(c.HostNotificationsEnabled)
	m.ServiceNotificationsEnabled = nagiosToOptionalBool(c.ServiceNotificationsEnabled)
	m.HostNotificationPeriod = types.StringValue(c.HostNotificationPeriod)
	m.ServiceNotificationPeriod = types.StringValue(c.ServiceNotificationPeriod)
	m.HostNotificationOptions = types.StringValue(c.HostNotificationOptions)
	m.ServiceNotificationOptions = types.StringValue(c.ServiceNotificationOptions)

	hostNotificationCommands, d := stringsToSet(ctx, c.HostNotificationCommands)
	diags.Append(d...)
	m.HostNotificationCommands = hostNotificationCommands

	serviceNotificationCommands, d := stringsToSet(ctx, c.ServiceNotificationCommands)
	diags.Append(d...)
	m.ServiceNotificationCommands = serviceNotificationCommands

	m.Alias = stringOrNull(c.Alias)

	contactGroups, d := stringsToSet(ctx, c.ContactGroups)
	diags.Append(d...)
	m.ContactGroups = contactGroups

	templates, d := stringsToSet(ctx, c.Templates)
	diags.Append(d...)
	m.Templates = templates

	m.Email = stringOrNull(c.Email)
	m.Pager = stringOrNull(c.Pager)
	m.Address1 = stringOrNull(c.Address1)
	m.Address2 = stringOrNull(c.Address2)
	m.Address3 = stringOrNull(c.Address3)
	m.CanSubmitCommands = nagiosToOptionalBool(c.CanSubmitCommands)
	m.RetainStatusInformation = nagiosToOptionalBool(c.RetainStatusInformation)
	m.RetainNonstatusInformation = nagiosToOptionalBool(c.RetainNonstatusInformation)

	return diags
}
