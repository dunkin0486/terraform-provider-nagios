package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ datasource.DataSource              = &hostDataSource{}
	_ datasource.DataSourceWithConfigure = &hostDataSource{}
)

func NewHostDataSource() datasource.DataSource {
	return &hostDataSource{}
}

type hostDataSource struct {
	client *client.Client
}

func (d *hostDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_host"
}

func (d *hostDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Nagios XI host.",
		Attributes: map[string]schema.Attribute{
			"host_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the host",
			},
			"address":                      schema.StringAttribute{Computed: true, Description: "The IP address of the host"},
			"display_name":                 schema.StringAttribute{Computed: true},
			"max_check_attempts":           schema.StringAttribute{Computed: true},
			"check_period":                 schema.StringAttribute{Computed: true},
			"notification_interval":        schema.StringAttribute{Computed: true},
			"notification_period":          schema.StringAttribute{Computed: true},
			"contacts":                     schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"alias":                        schema.StringAttribute{Computed: true},
			"templates":                    schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"check_command":                schema.StringAttribute{Computed: true},
			"contact_groups":               schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"notes":                        schema.StringAttribute{Computed: true},
			"notes_url":                    schema.StringAttribute{Computed: true},
			"action_url":                   schema.StringAttribute{Computed: true},
			"initial_state":                schema.StringAttribute{Computed: true},
			"retry_interval":               schema.StringAttribute{Computed: true},
			"passive_checks_enabled":       schema.BoolAttribute{Computed: true},
			"active_checks_enabled":        schema.BoolAttribute{Computed: true},
			"obsess_over_host":             schema.BoolAttribute{Computed: true},
			"event_handler":                schema.StringAttribute{Computed: true},
			"event_handler_enabled":        schema.BoolAttribute{Computed: true},
			"flap_detection_enabled":       schema.BoolAttribute{Computed: true},
			"flap_detection_options":       schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"low_flap_threshold":           schema.StringAttribute{Computed: true},
			"high_flap_threshold":          schema.StringAttribute{Computed: true},
			"process_perf_data":            schema.BoolAttribute{Computed: true},
			"retain_status_information":    schema.BoolAttribute{Computed: true},
			"retain_nonstatus_information": schema.BoolAttribute{Computed: true},
			"check_freshness":              schema.BoolAttribute{Computed: true},
			"freshness_threshold":          schema.StringAttribute{Computed: true},
			"first_notification_delay":     schema.StringAttribute{Computed: true},
			"notification_options":         schema.StringAttribute{Computed: true},
			"notifications_enabled":        schema.BoolAttribute{Computed: true},
			"stalking_options":             schema.StringAttribute{Computed: true},
			"icon_image":                   schema.StringAttribute{Computed: true},
			"icon_image_alt":               schema.StringAttribute{Computed: true},
			"vrml_image":                   schema.StringAttribute{Computed: true},
			"statusmap_image":              schema.StringAttribute{Computed: true},
			"coords_2d":                    schema.StringAttribute{Computed: true},
			"coords_3d":                    schema.StringAttribute{Computed: true},
			"register":                     schema.BoolAttribute{Computed: true},
			"free_variables":               schema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *hostDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *hostDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config hostModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.client.GetHost(ctx, config.HostName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading host", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Host not found", fmt.Sprintf("No host named %q exists in Nagios.", config.HostName.ValueString()))
		return
	}

	diags := modelFromHost(ctx, &config, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
