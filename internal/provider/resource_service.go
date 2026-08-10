package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ resource.Resource                = &serviceResource{}
	_ resource.ResourceWithConfigure   = &serviceResource{}
	_ resource.ResourceWithImportState = &serviceResource{}
)

func NewServiceResource() resource.Resource {
	return &serviceResource{}
}

type serviceResource struct {
	client *client.Client
}

// serviceModel is the Terraform-side representation of a nagios_service
// resource. Note: unlike nagios_host, this resource is keyed differently
// across verbs at the client layer - see internal/client/service.go.
type serviceModel struct {
	ServiceName                types.String `tfsdk:"service_name"`
	HostName                   types.Set    `tfsdk:"host_name"`
	HostgroupName              types.Set    `tfsdk:"hostgroup_name"`
	DisplayName                types.String `tfsdk:"display_name"`
	Description                types.String `tfsdk:"description"`
	CheckCommand               types.String `tfsdk:"check_command"`
	MaxCheckAttempts           types.String `tfsdk:"max_check_attempts"`
	CheckInterval              types.String `tfsdk:"check_interval"`
	RetryInterval              types.String `tfsdk:"retry_interval"`
	CheckPeriod                types.String `tfsdk:"check_period"`
	NotificationInterval       types.String `tfsdk:"notification_interval"`
	NotificationPeriod         types.String `tfsdk:"notification_period"`
	Contacts                   types.Set    `tfsdk:"contacts"`
	Templates                  types.Set    `tfsdk:"templates"`
	IsVolatile                 types.Bool   `tfsdk:"is_volatile"`
	InitialState               types.String `tfsdk:"initial_state"`
	ActiveChecksEnabled        types.Bool   `tfsdk:"active_checks_enabled"`
	PassiveChecksEnabled       types.Bool   `tfsdk:"passive_checks_enabled"`
	ObsessOverService          types.Bool   `tfsdk:"obsess_over_service"`
	CheckFreshness             types.Bool   `tfsdk:"check_freshness"`
	FreshnessThreshold         types.String `tfsdk:"freshness_threshold"`
	EventHandler               types.String `tfsdk:"event_handler"`
	EventHandlerEnabled        types.Bool   `tfsdk:"event_handler_enabled"`
	LowFlapThreshold           types.String `tfsdk:"low_flap_threshold"`
	HighFlapThreshold          types.String `tfsdk:"high_flap_threshold"`
	FlapDetectionEnabled       types.Bool   `tfsdk:"flap_detection_enabled"`
	FlapDetectionOptions       types.Set    `tfsdk:"flap_detection_options"`
	ProcessPerfData            types.Bool   `tfsdk:"process_perf_data"`
	RetainStatusInformation    types.Bool   `tfsdk:"retain_status_information"`
	RetainNonStatusInformation types.Bool   `tfsdk:"retain_nonstatus_information"`
	FirstNotificationDelay     types.String `tfsdk:"first_notification_delay"`
	NotificationOptions        types.Set    `tfsdk:"notification_options"`
	NotificationsEnabled       types.Bool   `tfsdk:"notifications_enabled"`
	ContactGroups              types.Set    `tfsdk:"contact_groups"`
	Servicegroups              types.Set    `tfsdk:"servicegroups"`
	Notes                      types.String `tfsdk:"notes"`
	NotesURL                   types.String `tfsdk:"notes_url"`
	ActionURL                  types.String `tfsdk:"action_url"`
	IconImage                  types.String `tfsdk:"icon_image"`
	IconImageAlt               types.String `tfsdk:"icon_image_alt"`
	Register                   types.Bool   `tfsdk:"register"`
	FreeVariables              types.Map    `tfsdk:"free_variables"`
}

func (r *serviceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

func (r *serviceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI service.",
		Attributes: map[string]schema.Attribute{
			"service_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the service",
			},
			"host_name": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "The hosts that the service should run on",
			},
			"hostgroup_name": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of hostgroups whose member hosts the service should also run on, in addition to `host_name`",
			},
			"display_name": schema.StringAttribute{
				Optional:    true,
				Description: "Another name for the service that will be displayed in the web interface",
			},
			"description": schema.StringAttribute{
				Required:    true,
				Description: "Defines the description of the service. It may contain spaces, dashes and colons (avoid using semicolons, apostrophes and quotation marks)",
			},
			"check_command": schema.StringAttribute{
				Required:    true,
				Description: "The name of the command that should be used to check the status of the service",
			},
			"max_check_attempts": schema.StringAttribute{
				Required:    true,
				Description: "How many times to retry the service check before alerting when the state is anything other than OK",
			},
			"check_interval": schema.StringAttribute{
				Required:    true,
				Description: "The number of minutes to wait until the next regular check of the service",
			},
			"retry_interval": schema.StringAttribute{
				Required:    true,
				Description: "The number of minutes to wait until re-checking the service",
			},
			"check_period": schema.StringAttribute{
				Required:    true,
				Description: "The time period during which active checks of the service can be made",
			},
			"notification_interval": schema.StringAttribute{
				Required:    true,
				Description: "How long to wait before sending another notification to a contact that the service is down",
			},
			"notification_period": schema.StringAttribute{
				Required:    true,
				Description: "The time period during which notifications can be sent for a service alert",
			},
			"contacts": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "The list of users that Nagios should alert when a service is down",
			},
			"templates": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of service templates to apply to the service",
			},
			"is_volatile": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Determines if the service is 'volatile'. Services typically are not volatile and this should be disabled.",
			},
			"initial_state": schema.StringAttribute{
				Optional:    true,
				Description: "By default, Nagios will assume the service is in an OK state. Valid options are: 'd' down, 's' up or 'u' unreachable",
			},
			"active_checks_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Sets whether or not active checks are enabled for the service",
			},
			"passive_checks_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Sets whether or not passive checks are enabled for the service",
			},
			"obsess_over_service": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not Nagios 'obsesses' over the service using the ocsp_command",
			},
			"check_freshness": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not freshness checks are enabled for the service",
			},
			"freshness_threshold": schema.StringAttribute{
				Optional:    true,
				Description: "The freshness threshold used for the service",
			},
			"event_handler": schema.StringAttribute{
				Optional:    true,
				Description: "The command that should be run whenever a change in the state of the service is detected",
			},
			"event_handler_enabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not event handlers should be enabled for the service",
			},
			"low_flap_threshold": schema.StringAttribute{
				Optional:    true,
				Description: "The minimum threshold that should be used when detecting if flapping is occurring",
			},
			"high_flap_threshold": schema.StringAttribute{
				Optional:    true,
				Description: "The maximum threshold that should be used when detecting if flapping is occurring",
			},
			"flap_detection_enabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not flap detection is enabled for the service",
			},
			"flap_detection_options": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Determines what flap detection logic will be used for the service. One or more of the following valid options can be provided: 'd' down, 'o' up, or 'u' unreachable",
			},
			"process_perf_data": schema.BoolAttribute{
				Optional:    true,
				Description: "Determines if Nagios should process performance data",
			},
			"retain_status_information": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not status related information should be kept for the service",
			},
			"retain_nonstatus_information": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not non-status related information should be kept for the service",
			},
			"first_notification_delay": schema.StringAttribute{
				Optional:    true,
				Description: "The amount of time to wait to send out the first notification when a service enters a non-UP state",
			},
			"notification_options": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Determines when Nagios should alert if a service is one or more of the following options: 'o' up, 'd' down, 'u' unreachable, 'r' recovery, 'f' flapping or 's' scheduled downtime",
			},
			"notifications_enabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Determines if Nagios should send notifications",
			},
			"contact_groups": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of the contact groups that should be notified if the service goes down",
			},
			"servicegroups": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of servicegroups this service should be a member of, assigned from the service side",
			},
			"notes": schema.StringAttribute{
				Optional:    true,
				Description: "Notes about the service that may assist with troubleshooting",
			},
			"notes_url": schema.StringAttribute{
				Optional:    true,
				Description: "URL to a third-party documentation repository containing more information about the service",
			},
			"action_url": schema.StringAttribute{
				Optional:    true,
				Description: "URL to a third-party documentation repository containing actions to take in the event the service goes down",
			},
			"icon_image": schema.StringAttribute{
				Optional:    true,
				Description: "The icon to display in Nagios",
			},
			"icon_image_alt": schema.StringAttribute{
				Optional:    true,
				Description: "The text to display when hovering over the icon_image or the text to display if the icon_image is unavailable",
			},
			"register": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Determines if the service will be marked as active or inactive",
			},
			"free_variables": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A key/value pair of free variables to add to the service. The key must begin with an underscore.",
			},
		},
	}
}

func (r *serviceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serviceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, diags := serviceFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.NewService(ctx, s); err != nil {
		resp.Diagnostics.AddError("Error creating service", err.Error())
		return
	}

	got, err := client.RetryUntilFound(ctx, defaultCreateRetryAttempts, defaultCreateRetryBackoff, func() (*client.Service, error) {
		return r.client.GetService(ctx, s.ServiceName)
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading service after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Service not found after create", fmt.Sprintf("Service %q was created but not visible on read-back after retries.", s.ServiceName))
		return
	}

	diags = modelFromService(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetService(ctx, state.ServiceName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading service", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	diags := modelFromService(ctx, &state, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serviceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state serviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, diags := serviceFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// UpdateService is keyed by (config_name, description), unlike
	// DeleteService below which is keyed by (host_name, description) - see
	// internal/client/service.go's doc comment on Service.
	if err := r.client.UpdateService(ctx, s, state.ServiceName.ValueString(), state.Description.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating service", err.Error())
		return
	}

	got, err := r.client.GetService(ctx, s.ServiceName)
	if err != nil {
		resp.Diagnostics.AddError("Error reading service after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Service not found after update", fmt.Sprintf("Service %q not found after update.", s.ServiceName))
		return
	}

	diags = modelFromService(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hostNames, diags := setToStrings(ctx, state.HostName)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// DeleteService is keyed by (host_name, description), not by
	// service_name - a real Nagios API inconsistency versus Update above.
	if err := r.client.DeleteService(ctx, hostNames, state.Description.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting service", err.Error())
	}
}

func (r *serviceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("service_name"), req, resp)
}

func serviceFromModel(ctx context.Context, m *serviceModel) (*client.Service, diag.Diagnostics) {
	var diags diag.Diagnostics

	hostNames, d := setToStrings(ctx, m.HostName)
	diags.Append(d...)
	hostgroupNames, d := setToStrings(ctx, m.HostgroupName)
	diags.Append(d...)
	contacts, d := setToStrings(ctx, m.Contacts)
	diags.Append(d...)
	templates, d := setToStrings(ctx, m.Templates)
	diags.Append(d...)
	flapDetectionOptions, d := setToStrings(ctx, m.FlapDetectionOptions)
	diags.Append(d...)
	notificationOptions, d := setToStrings(ctx, m.NotificationOptions)
	diags.Append(d...)
	contactGroups, d := setToStrings(ctx, m.ContactGroups)
	diags.Append(d...)
	servicegroups, d := setToStrings(ctx, m.Servicegroups)
	diags.Append(d...)
	freeVariables, d := mapToStrings(ctx, m.FreeVariables)
	diags.Append(d...)

	if diags.HasError() {
		return nil, diags
	}

	return &client.Service{
		ServiceName:                m.ServiceName.ValueString(),
		HostName:                   hostNames,
		HostgroupName:              hostgroupNames,
		DisplayName:                m.DisplayName.ValueString(),
		Description:                m.Description.ValueString(),
		CheckCommand:               m.CheckCommand.ValueString(),
		MaxCheckAttempts:           m.MaxCheckAttempts.ValueString(),
		CheckInterval:              m.CheckInterval.ValueString(),
		RetryInterval:              m.RetryInterval.ValueString(),
		CheckPeriod:                m.CheckPeriod.ValueString(),
		NotificationInterval:       m.NotificationInterval.ValueString(),
		NotificationPeriod:         m.NotificationPeriod.ValueString(),
		Contacts:                   contacts,
		Templates:                  templates,
		IsVolatile:                 optionalBoolToNagios(m.IsVolatile),
		InitialState:               m.InitialState.ValueString(),
		ActiveChecksEnabled:        optionalBoolToNagios(m.ActiveChecksEnabled),
		PassiveChecksEnabled:       optionalBoolToNagios(m.PassiveChecksEnabled),
		ObsessOverService:          optionalBoolToNagios(m.ObsessOverService),
		CheckFreshness:             optionalBoolToNagios(m.CheckFreshness),
		FreshnessThreshold:         m.FreshnessThreshold.ValueString(),
		EventHandler:               m.EventHandler.ValueString(),
		EventHandlerEnabled:        optionalBoolToNagios(m.EventHandlerEnabled),
		LowFlapThreshold:           m.LowFlapThreshold.ValueString(),
		HighFlapThreshold:          m.HighFlapThreshold.ValueString(),
		FlapDetectionEnabled:       optionalBoolToNagios(m.FlapDetectionEnabled),
		FlapDetectionOptions:       flapDetectionOptions,
		ProcessPerfData:            optionalBoolToNagios(m.ProcessPerfData),
		RetainStatusInformation:    optionalBoolToNagios(m.RetainStatusInformation),
		RetainNonStatusInformation: optionalBoolToNagios(m.RetainNonStatusInformation),
		FirstNotificationDelay:     m.FirstNotificationDelay.ValueString(),
		NotificationOptions:        notificationOptions,
		NotificationsEnabled:       optionalBoolToNagios(m.NotificationsEnabled),
		ContactGroups:              contactGroups,
		Servicegroups:              servicegroups,
		Notes:                      m.Notes.ValueString(),
		NotesURL:                   m.NotesURL.ValueString(),
		ActionURL:                  m.ActionURL.ValueString(),
		IconImage:                  m.IconImage.ValueString(),
		IconImageAlt:               m.IconImageAlt.ValueString(),
		Register:                   optionalBoolToNagios(m.Register),
		FreeVariables:              freeVariables,
	}, diags
}

func modelFromService(ctx context.Context, m *serviceModel, s *client.Service) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ServiceName = types.StringValue(s.ServiceName)

	hostNames, d := stringsToSet(ctx, s.HostName)
	diags.Append(d...)
	m.HostName = hostNames

	hostgroupNames, d := stringsToSet(ctx, s.HostgroupName)
	diags.Append(d...)
	m.HostgroupName = hostgroupNames

	m.DisplayName = stringOrNull(s.DisplayName)
	m.Description = types.StringValue(s.Description)
	m.CheckCommand = types.StringValue(s.CheckCommand)
	m.MaxCheckAttempts = types.StringValue(s.MaxCheckAttempts)
	m.CheckInterval = types.StringValue(s.CheckInterval)
	m.RetryInterval = types.StringValue(s.RetryInterval)
	m.CheckPeriod = types.StringValue(s.CheckPeriod)
	m.NotificationInterval = types.StringValue(s.NotificationInterval)
	m.NotificationPeriod = types.StringValue(s.NotificationPeriod)

	contacts, d := stringsToSet(ctx, s.Contacts)
	diags.Append(d...)
	m.Contacts = contacts

	templates, d := stringsToSet(ctx, s.Templates)
	diags.Append(d...)
	m.Templates = templates

	m.IsVolatile = nagiosToOptionalBool(s.IsVolatile)
	m.InitialState = stringOrNull(s.InitialState)
	m.ActiveChecksEnabled = nagiosToOptionalBool(s.ActiveChecksEnabled)
	m.PassiveChecksEnabled = nagiosToOptionalBool(s.PassiveChecksEnabled)
	m.ObsessOverService = nagiosToOptionalBool(s.ObsessOverService)
	m.CheckFreshness = nagiosToOptionalBool(s.CheckFreshness)
	m.FreshnessThreshold = stringOrNull(s.FreshnessThreshold)
	m.EventHandler = stringOrNull(s.EventHandler)
	m.EventHandlerEnabled = nagiosToOptionalBool(s.EventHandlerEnabled)
	m.LowFlapThreshold = stringOrNull(s.LowFlapThreshold)
	m.HighFlapThreshold = stringOrNull(s.HighFlapThreshold)
	m.FlapDetectionEnabled = nagiosToOptionalBool(s.FlapDetectionEnabled)

	flapDetectionOptions, d := stringsToSet(ctx, s.FlapDetectionOptions)
	diags.Append(d...)
	m.FlapDetectionOptions = flapDetectionOptions

	m.ProcessPerfData = nagiosToOptionalBool(s.ProcessPerfData)
	m.RetainStatusInformation = nagiosToOptionalBool(s.RetainStatusInformation)
	m.RetainNonStatusInformation = nagiosToOptionalBool(s.RetainNonStatusInformation)
	m.FirstNotificationDelay = stringOrNull(s.FirstNotificationDelay)

	notificationOptions, d := stringsToSet(ctx, s.NotificationOptions)
	diags.Append(d...)
	m.NotificationOptions = notificationOptions

	m.NotificationsEnabled = nagiosToOptionalBool(s.NotificationsEnabled)

	contactGroups, d := stringsToSet(ctx, s.ContactGroups)
	diags.Append(d...)
	m.ContactGroups = contactGroups

	servicegroups, d := stringsToSet(ctx, s.Servicegroups)
	diags.Append(d...)
	m.Servicegroups = servicegroups

	m.Notes = stringOrNull(s.Notes)
	m.NotesURL = stringOrNull(s.NotesURL)
	m.ActionURL = stringOrNull(s.ActionURL)
	m.IconImage = stringOrNull(s.IconImage)
	m.IconImageAlt = stringOrNull(s.IconImageAlt)
	m.Register = nagiosToOptionalBool(s.Register)

	freeVariables, d := stringsMapToMap(ctx, s.FreeVariables)
	diags.Append(d...)
	m.FreeVariables = freeVariables

	return diags
}
