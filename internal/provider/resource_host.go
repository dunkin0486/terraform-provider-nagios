package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ resource.Resource                = &hostResource{}
	_ resource.ResourceWithConfigure   = &hostResource{}
	_ resource.ResourceWithImportState = &hostResource{}
)

func NewHostResource() resource.Resource {
	return &hostResource{}
}

type hostResource struct {
	client *client.Client
}

// hostModel is the Terraform-side representation of a nagios_host resource,
// 1:1 with the schema attributes below.
type hostModel struct {
	HostName                   types.String `tfsdk:"host_name"`
	Address                    types.String `tfsdk:"address"`
	DisplayName                types.String `tfsdk:"display_name"`
	MaxCheckAttempts           types.String `tfsdk:"max_check_attempts"`
	CheckPeriod                types.String `tfsdk:"check_period"`
	NotificationInterval       types.String `tfsdk:"notification_interval"`
	NotificationPeriod         types.String `tfsdk:"notification_period"`
	Contacts                   types.Set    `tfsdk:"contacts"`
	Alias                      types.String `tfsdk:"alias"`
	Templates                  types.Set    `tfsdk:"templates"`
	CheckCommand               types.String `tfsdk:"check_command"`
	ContactGroups              types.Set    `tfsdk:"contact_groups"`
	Parents                    types.Set    `tfsdk:"parents"`
	Hostgroups                 types.Set    `tfsdk:"hostgroups"`
	Notes                      types.String `tfsdk:"notes"`
	NotesURL                   types.String `tfsdk:"notes_url"`
	ActionURL                  types.String `tfsdk:"action_url"`
	InitialState               types.String `tfsdk:"initial_state"`
	RetryInterval              types.String `tfsdk:"retry_interval"`
	PassiveChecksEnabled       types.Bool   `tfsdk:"passive_checks_enabled"`
	ActiveChecksEnabled        types.Bool   `tfsdk:"active_checks_enabled"`
	ObsessOverHost             types.Bool   `tfsdk:"obsess_over_host"`
	EventHandler               types.String `tfsdk:"event_handler"`
	EventHandlerEnabled        types.Bool   `tfsdk:"event_handler_enabled"`
	FlapDetectionEnabled       types.Bool   `tfsdk:"flap_detection_enabled"`
	FlapDetectionOptions       types.Set    `tfsdk:"flap_detection_options"`
	LowFlapThreshold           types.String `tfsdk:"low_flap_threshold"`
	HighFlapThreshold          types.String `tfsdk:"high_flap_threshold"`
	ProcessPerfData            types.Bool   `tfsdk:"process_perf_data"`
	RetainStatusInformation    types.Bool   `tfsdk:"retain_status_information"`
	RetainNonstatusInformation types.Bool   `tfsdk:"retain_nonstatus_information"`
	CheckFreshness             types.Bool   `tfsdk:"check_freshness"`
	FreshnessThreshold         types.String `tfsdk:"freshness_threshold"`
	FirstNotificationDelay     types.String `tfsdk:"first_notification_delay"`
	NotificationOptions        types.String `tfsdk:"notification_options"`
	NotificationsEnabled       types.Bool   `tfsdk:"notifications_enabled"`
	StalkingOptions            types.String `tfsdk:"stalking_options"`
	IconImage                  types.String `tfsdk:"icon_image"`
	IconImageAlt               types.String `tfsdk:"icon_image_alt"`
	VRMLImage                  types.String `tfsdk:"vrml_image"`
	StatusMapImage             types.String `tfsdk:"statusmap_image"`
	// Nagios's wire field names are "2d_coords"/"3d_coords" (see client.Host),
	// but Terraform attribute names can't start with a digit, so these are
	// exposed to HCL as coords_2d/coords_3d instead.
	TwoDCoords    types.String `tfsdk:"coords_2d"`
	ThreeDCoords  types.String `tfsdk:"coords_3d"`
	Register      types.Bool   `tfsdk:"register"`
	FreeVariables types.Map    `tfsdk:"free_variables"`
}

func (r *hostResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_host"
}

func (r *hostResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI host.",
		Attributes: map[string]schema.Attribute{
			"host_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the host",
			},
			"address": schema.StringAttribute{
				Required:    true,
				Description: "The IP address of the host",
			},
			"display_name": schema.StringAttribute{
				Optional:    true,
				Description: "Another name for the host that will be displayed in the web interface. If left blank, the value from `host_name` will be displayed",
			},
			"max_check_attempts": schema.StringAttribute{
				Required:    true,
				Description: "How many times to retry the host check before alerting when the state is anything other than OK",
			},
			"check_period": schema.StringAttribute{
				Required:    true,
				Description: "The time period during which active checks of the host can be made",
			},
			"notification_interval": schema.StringAttribute{
				Required:    true,
				Description: "How long to wait before sending another notification to a contact that the host is down",
			},
			"notification_period": schema.StringAttribute{
				Required:    true,
				Description: "The time period during which notifications can be sent for a host alert",
			},
			"contacts": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "The list of users that Nagios should alert when a host is down",
			},
			"alias": schema.StringAttribute{
				Optional:    true,
				Description: "A longer name to describe the host",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"templates": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of Nagios templates to apply to the host",
			},
			"check_command": schema.StringAttribute{
				Optional:    true,
				Description: "The name of the command that should be used to check if the host is up or down",
			},
			"contact_groups": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of the contact groups that should be notified if the host goes down",
			},
			"parents": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of hosts that are used to determine the network reachability of this host - if a parent host is down or unreachable, this host is marked unreachable rather than down",
			},
			"hostgroups": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of hostgroups this host should be a member of, assigned from the host side (complements a hostgroup's own `members` attribute)",
			},
			"notes": schema.StringAttribute{
				Optional:    true,
				Description: "Notes about the host that may assist with troubleshooting",
			},
			"notes_url": schema.StringAttribute{
				Optional:    true,
				Description: "URL to a third-party documentation repository containing more information about the host",
			},
			"action_url": schema.StringAttribute{
				Optional:    true,
				Description: "URL to a third-party documentation repository containing actions to take in the event the host goes down",
			},
			"initial_state": schema.StringAttribute{
				Optional:    true,
				Description: "The state of the host when it is first added to Nagios. Valid options are: 'd' down, 's' up or 'u' unreachable",
			},
			"retry_interval": schema.StringAttribute{
				Optional:    true,
				Description: "How often should Nagios try to check the host after the initial down alert",
			},
			"passive_checks_enabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not passive checks are enabled for the host",
			},
			"active_checks_enabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not active checks are enabled for the host",
			},
			"obsess_over_host": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not Nagios 'obsesses' over the host using the ochp_command",
			},
			"event_handler": schema.StringAttribute{
				Optional:    true,
				Description: "The command that should be run whenever a change in the state of the host is detected",
			},
			"event_handler_enabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not event handlers should be enabled for the host",
			},
			"flap_detection_enabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not flap detection is enabled for the host",
			},
			"flap_detection_options": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Determines what flap detection logic will be used for the host. One or more of the following valid options can be provided: 'd' down, 'o' up, or 'u' unreachable.",
			},
			"low_flap_threshold": schema.StringAttribute{
				Optional:    true,
				Description: "The minimum threshold that should be used when detecting if flapping is occurring",
			},
			"high_flap_threshold": schema.StringAttribute{
				Optional:    true,
				Description: "The maximum threshold that should be used when detecting if flapping is occurring",
			},
			"process_perf_data": schema.BoolAttribute{
				Optional:    true,
				Description: "Determines if Nagios should process performance data",
			},
			"retain_status_information": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not status related information should be kept for the host",
			},
			"retain_nonstatus_information": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not non-status related information should be kept for the host",
			},
			"check_freshness": schema.BoolAttribute{
				Optional:    true,
				Description: "Sets whether or not freshness checks are enabled for the host",
			},
			"freshness_threshold": schema.StringAttribute{
				Optional:    true,
				Description: "The freshness threshold used for the host",
			},
			"first_notification_delay": schema.StringAttribute{
				Optional:    true,
				Description: "The amount of time to wait to send out the first notification when a host enters a non-UP state",
			},
			"notification_options": schema.StringAttribute{
				Optional:    true,
				Description: "Determines when Nagios should alert if a host is one or more of the following option: 'o' up, 'd' down, 'u' unreachable, 'r' recovery, 'f' flapping or 's' scheduled downtime",
			},
			"notifications_enabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Determines if Nagios should send notifications",
			},
			"stalking_options": schema.StringAttribute{
				Optional:    true,
				Description: "A list of options to determine which states, if any, should be stalked by Nagios",
			},
			"icon_image": schema.StringAttribute{
				Optional:    true,
				Description: "The icon to display in Nagios",
			},
			"icon_image_alt": schema.StringAttribute{
				Optional:    true,
				Description: "The text to display when hovering over the icon",
			},
			"vrml_image": schema.StringAttribute{
				Optional:    true,
				Description: "The image that will be used as a texture map for the specified host",
			},
			"statusmap_image": schema.StringAttribute{
				Optional:    true,
				Description: "The name of the image that should be used in the statusmap CGI in Nagios",
			},
			"coords_2d": schema.StringAttribute{
				Optional:    true,
				Description: "The coordinates to use when drawing the host in the statusmap CGI (Nagios's own API field name is 2d_coords, but Terraform attribute names can't start with a digit)",
			},
			"coords_3d": schema.StringAttribute{
				Optional:    true,
				Description: "The coordinates to use when drawing the host in the statuswrl CGI (Nagios's own API field name is 3d_coords, but Terraform attribute names can't start with a digit)",
			},
			"register": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Determines if the host will be marked as active or inactive",
			},
			"free_variables": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A key/value pair of free variables to add to the host. The key must begin with an underscore.",
			},
		},
	}
}

func (r *hostResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *hostResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	h, diags := hostFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.NewHost(ctx, h); err != nil {
		resp.Diagnostics.AddError("Error creating host", err.Error())
		return
	}

	// Bounded retry to tolerate Nagios XI's read-after-write eventual
	// consistency - see internal/client/retry.go.
	got, err := client.RetryUntilFound(ctx, defaultCreateRetryAttempts, defaultCreateRetryBackoff, func() (*client.Host, error) {
		return r.client.GetHost(ctx, h.HostName)
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading host after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError(
			"Host not found after create",
			fmt.Sprintf("Host %q was created but not visible on read-back after retries. This may indicate a Nagios XI config-apply delay.", h.HostName),
		)
		return
	}

	diags = modelFromHost(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetHost(ctx, state.HostName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading host", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	diags := modelFromHost(ctx, &state, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *hostResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state hostModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	h, diags := hostFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateHost(ctx, h, state.HostName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating host", err.Error())
		return
	}

	got, err := r.client.GetHost(ctx, h.HostName)
	if err != nil {
		resp.Diagnostics.AddError("Error reading host after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Host not found after update", fmt.Sprintf("Host %q not found after update.", h.HostName))
		return
	}

	diags = modelFromHost(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state hostModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteHost(ctx, state.HostName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting host", err.Error())
	}
}

func (r *hostResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("host_name"), req, resp)
}

// hostFromModel converts a Terraform plan/state model into the client.Host
// the API expects. Optional bools are only sent when explicitly set (not
// null) - the old provider's bug was silently sending "0" for unset optional
// bools by reading Go's bool zero-value, which is indistinguishable from an
// explicit false.
func hostFromModel(ctx context.Context, m *hostModel) (*client.Host, diag.Diagnostics) {
	var diags diag.Diagnostics

	contacts, d := setToStrings(ctx, m.Contacts)
	diags.Append(d...)
	templates, d := setToStrings(ctx, m.Templates)
	diags.Append(d...)
	contactGroups, d := setToStrings(ctx, m.ContactGroups)
	diags.Append(d...)
	parents, d := setToStrings(ctx, m.Parents)
	diags.Append(d...)
	hostgroups, d := setToStrings(ctx, m.Hostgroups)
	diags.Append(d...)
	flapDetectionOptions, d := setToStrings(ctx, m.FlapDetectionOptions)
	diags.Append(d...)
	freeVariables, d := mapToStrings(ctx, m.FreeVariables)
	diags.Append(d...)

	if diags.HasError() {
		return nil, diags
	}

	return &client.Host{
		HostName:                   m.HostName.ValueString(),
		Address:                    m.Address.ValueString(),
		DisplayName:                m.DisplayName.ValueString(),
		MaxCheckAttempts:           m.MaxCheckAttempts.ValueString(),
		CheckPeriod:                m.CheckPeriod.ValueString(),
		NotificationInterval:       m.NotificationInterval.ValueString(),
		NotificationPeriod:         m.NotificationPeriod.ValueString(),
		Contacts:                   contacts,
		Alias:                      m.Alias.ValueString(),
		Templates:                  templates,
		CheckCommand:               m.CheckCommand.ValueString(),
		ContactGroups:              contactGroups,
		Parents:                    parents,
		Hostgroups:                 hostgroups,
		Notes:                      m.Notes.ValueString(),
		NotesURL:                   m.NotesURL.ValueString(),
		ActionURL:                  m.ActionURL.ValueString(),
		InitialState:               m.InitialState.ValueString(),
		RetryInterval:              m.RetryInterval.ValueString(),
		PassiveChecksEnabled:       optionalBoolToNagios(m.PassiveChecksEnabled),
		ActiveChecksEnabled:        optionalBoolToNagios(m.ActiveChecksEnabled),
		ObsessOverHost:             optionalBoolToNagios(m.ObsessOverHost),
		EventHandler:               m.EventHandler.ValueString(),
		EventHandlerEnabled:        optionalBoolToNagios(m.EventHandlerEnabled),
		FlapDetectionEnabled:       optionalBoolToNagios(m.FlapDetectionEnabled),
		FlapDetectionOptions:       flapDetectionOptions,
		LowFlapThreshold:           m.LowFlapThreshold.ValueString(),
		HighFlapThreshold:          m.HighFlapThreshold.ValueString(),
		ProcessPerfData:            optionalBoolToNagios(m.ProcessPerfData),
		RetainStatusInformation:    optionalBoolToNagios(m.RetainStatusInformation),
		RetainNonstatusInformation: optionalBoolToNagios(m.RetainNonstatusInformation),
		CheckFreshness:             optionalBoolToNagios(m.CheckFreshness),
		FreshnessThreshold:         m.FreshnessThreshold.ValueString(),
		FirstNotificationDelay:     m.FirstNotificationDelay.ValueString(),
		NotificationOptions:        m.NotificationOptions.ValueString(),
		NotificationsEnabled:       optionalBoolToNagios(m.NotificationsEnabled),
		StalkingOptions:            m.StalkingOptions.ValueString(),
		IconImage:                  m.IconImage.ValueString(),
		IconImageAlt:               m.IconImageAlt.ValueString(),
		VRMLImage:                  m.VRMLImage.ValueString(),
		StatusMapImage:             m.StatusMapImage.ValueString(),
		TwoDCoords:                 m.TwoDCoords.ValueString(),
		ThreeDCoords:               m.ThreeDCoords.ValueString(),
		Register:                   optionalBoolToNagios(m.Register),
		FreeVariables:              freeVariables,
	}, diags
}

// modelFromHost is the inverse of hostFromModel, populating Terraform state
// from what the API returned after a create/read/update.
func modelFromHost(ctx context.Context, m *hostModel, h *client.Host) diag.Diagnostics {
	var diags diag.Diagnostics

	m.HostName = types.StringValue(h.HostName)
	m.Address = types.StringValue(h.Address)
	m.DisplayName = stringOrNull(h.DisplayName)
	m.MaxCheckAttempts = types.StringValue(h.MaxCheckAttempts)
	m.CheckPeriod = types.StringValue(h.CheckPeriod)
	m.NotificationInterval = types.StringValue(h.NotificationInterval)
	m.NotificationPeriod = types.StringValue(h.NotificationPeriod)

	contacts, d := stringsToSet(ctx, h.Contacts)
	diags.Append(d...)
	m.Contacts = contacts

	m.Alias = stringOrNull(h.Alias)

	templates, d := stringsToSet(ctx, h.Templates)
	diags.Append(d...)
	m.Templates = templates

	m.CheckCommand = stringOrNull(h.CheckCommand)

	contactGroups, d := stringsToSet(ctx, h.ContactGroups)
	diags.Append(d...)
	m.ContactGroups = contactGroups

	parents, d := stringsToSet(ctx, h.Parents)
	diags.Append(d...)
	m.Parents = parents

	hostgroups, d := stringsToSet(ctx, h.Hostgroups)
	diags.Append(d...)
	m.Hostgroups = hostgroups

	m.Notes = stringOrNull(h.Notes)
	m.NotesURL = stringOrNull(h.NotesURL)
	m.ActionURL = stringOrNull(h.ActionURL)
	m.InitialState = stringOrNull(h.InitialState)
	m.RetryInterval = stringOrNull(h.RetryInterval)
	m.PassiveChecksEnabled = nagiosToOptionalBool(h.PassiveChecksEnabled)
	m.ActiveChecksEnabled = nagiosToOptionalBool(h.ActiveChecksEnabled)
	m.ObsessOverHost = nagiosToOptionalBool(h.ObsessOverHost)
	m.EventHandler = stringOrNull(h.EventHandler)
	m.EventHandlerEnabled = nagiosToOptionalBool(h.EventHandlerEnabled)
	m.FlapDetectionEnabled = nagiosToOptionalBool(h.FlapDetectionEnabled)

	flapDetectionOptions, d := stringsToSet(ctx, h.FlapDetectionOptions)
	diags.Append(d...)
	m.FlapDetectionOptions = flapDetectionOptions

	m.LowFlapThreshold = stringOrNull(h.LowFlapThreshold)
	m.HighFlapThreshold = stringOrNull(h.HighFlapThreshold)
	m.ProcessPerfData = nagiosToOptionalBool(h.ProcessPerfData)
	m.RetainStatusInformation = nagiosToOptionalBool(h.RetainStatusInformation)
	m.RetainNonstatusInformation = nagiosToOptionalBool(h.RetainNonstatusInformation)
	m.CheckFreshness = nagiosToOptionalBool(h.CheckFreshness)
	m.FreshnessThreshold = stringOrNull(h.FreshnessThreshold)
	m.FirstNotificationDelay = stringOrNull(h.FirstNotificationDelay)
	m.NotificationOptions = stringOrNull(h.NotificationOptions)
	m.NotificationsEnabled = nagiosToOptionalBool(h.NotificationsEnabled)
	m.StalkingOptions = stringOrNull(h.StalkingOptions)
	m.IconImage = stringOrNull(h.IconImage)
	m.IconImageAlt = stringOrNull(h.IconImageAlt)
	m.VRMLImage = stringOrNull(h.VRMLImage)
	m.StatusMapImage = stringOrNull(h.StatusMapImage)
	m.TwoDCoords = stringOrNull(h.TwoDCoords)
	m.ThreeDCoords = stringOrNull(h.ThreeDCoords)
	m.Register = nagiosToOptionalBool(h.Register)

	freeVariables, d := stringsMapToMap(ctx, h.FreeVariables)
	diags.Append(d...)
	m.FreeVariables = freeVariables

	return diags
}
