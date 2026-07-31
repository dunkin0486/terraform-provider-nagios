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
	_ datasource.DataSource              = &serviceDataSource{}
	_ datasource.DataSourceWithConfigure = &serviceDataSource{}
)

func NewServiceDataSource() datasource.DataSource {
	return &serviceDataSource{}
}

type serviceDataSource struct {
	client *client.Client
}

func (d *serviceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

func (d *serviceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Nagios XI service.",
		Attributes: map[string]schema.Attribute{
			"service_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the service",
			},
			"host_name":                    schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "The hosts that the service runs on"},
			"hostgroup_name":               schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "The hostgroups whose member hosts the service also runs on"},
			"display_name":                 schema.StringAttribute{Computed: true},
			"description":                  schema.StringAttribute{Computed: true, Description: "The description of the service"},
			"check_command":                schema.StringAttribute{Computed: true},
			"max_check_attempts":           schema.StringAttribute{Computed: true},
			"check_interval":               schema.StringAttribute{Computed: true},
			"retry_interval":               schema.StringAttribute{Computed: true},
			"check_period":                 schema.StringAttribute{Computed: true},
			"notification_interval":        schema.StringAttribute{Computed: true},
			"notification_period":          schema.StringAttribute{Computed: true},
			"contacts":                     schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"templates":                    schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"is_volatile":                  schema.BoolAttribute{Computed: true},
			"initial_state":                schema.StringAttribute{Computed: true},
			"active_checks_enabled":        schema.BoolAttribute{Computed: true},
			"passive_checks_enabled":       schema.BoolAttribute{Computed: true},
			"obsess_over_service":          schema.BoolAttribute{Computed: true},
			"check_freshness":              schema.BoolAttribute{Computed: true},
			"freshness_threshold":          schema.StringAttribute{Computed: true},
			"event_handler":                schema.StringAttribute{Computed: true},
			"event_handler_enabled":        schema.BoolAttribute{Computed: true},
			"low_flap_threshold":           schema.StringAttribute{Computed: true},
			"high_flap_threshold":          schema.StringAttribute{Computed: true},
			"flap_detection_enabled":       schema.BoolAttribute{Computed: true},
			"flap_detection_options":       schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"process_perf_data":            schema.BoolAttribute{Computed: true},
			"retain_status_information":    schema.BoolAttribute{Computed: true},
			"retain_nonstatus_information": schema.BoolAttribute{Computed: true},
			"first_notification_delay":     schema.StringAttribute{Computed: true},
			"notification_options":         schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"notifications_enabled":        schema.BoolAttribute{Computed: true},
			"contact_groups":               schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"servicegroups":                schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"notes":                        schema.StringAttribute{Computed: true},
			"notes_url":                    schema.StringAttribute{Computed: true},
			"action_url":                   schema.StringAttribute{Computed: true},
			"icon_image":                   schema.StringAttribute{Computed: true},
			"icon_image_alt":               schema.StringAttribute{Computed: true},
			"register":                     schema.BoolAttribute{Computed: true},
			"free_variables":               schema.MapAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (d *serviceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *serviceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config serviceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.client.GetService(ctx, config.ServiceName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading service", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Service not found", fmt.Sprintf("No service named %q exists in Nagios.", config.ServiceName.ValueString()))
		return
	}

	diags := modelFromService(ctx, &config, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
