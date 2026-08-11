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
	_ datasource.DataSource              = &contactDataSource{}
	_ datasource.DataSourceWithConfigure = &contactDataSource{}
)

func NewContactDataSource() datasource.DataSource {
	return &contactDataSource{}
}

type contactDataSource struct {
	client *client.Client
}

func (d *contactDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contact"
}

func (d *contactDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Nagios XI contact.",
		Attributes: map[string]schema.Attribute{
			"contact_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the contact",
			},
			"host_notifications_enabled":    schema.BoolAttribute{Computed: true, Description: "Determines whether or not the contact will receive notifications about host problems and recoveries"},
			"service_notifications_enabled": schema.BoolAttribute{Computed: true, Description: "Determines whether or not the contact will receive notifications about service problems and recoveries"},
			"host_notification_period":      schema.StringAttribute{Computed: true, Description: "The short name of the time period during which the contact can be notified about host problems or recoveries"},
			"service_notification_period":   schema.StringAttribute{Computed: true, Description: "The short name of the time period during which the contact can be notified about service problems or recoveries"},
			"host_notification_options":     schema.StringAttribute{Computed: true, Description: "The host states for which notifications can be sent out to this contact"},
			"service_notification_options":  schema.StringAttribute{Computed: true, Description: "The service states for which notifications can be sent out to this contact"},
			"host_notification_commands":    schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "A list of the short names of the commands used to notify the contact of a host problem or recovery"},
			"service_notification_commands": schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "A list of the short names of the commands used to notify the contact of a service problem or recovery"},
			"alias":                         schema.StringAttribute{Computed: true, Description: "A longer name or description for the contact"},
			"contact_groups":                schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "The short name(s) of the contactgroup(s) that the contact belongs to"},
			"templates":                     schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "The contact templates to apply to the contact"},
			"email":                         schema.StringAttribute{Computed: true, Description: "Defines an email address for the contact"},
			"pager":                         schema.StringAttribute{Computed: true, Description: "Defines a pager number for the contact"},
			"address1":                      schema.StringAttribute{Computed: true, Description: "Defines additional 'addresses' for the contact"},
			"address2":                      schema.StringAttribute{Computed: true, Description: "Defines additional 'addresses' for the contact"},
			"address3":                      schema.StringAttribute{Computed: true, Description: "Defines additional 'addresses' for the contact"},
			"can_submit_commands":           schema.BoolAttribute{Computed: true, Description: "Determines whether or not the contact can submit external commands to Nagios from the CGIs"},
			"retain_status_information":     schema.BoolAttribute{Computed: true, Description: "Determines whether or not status-related information about the contact is retained across program restarts"},
			"retain_nonstatus_information":  schema.BoolAttribute{Computed: true, Description: "Determines whether or not non-status information about the contact is retained across program restarts."},
		},
	}
}

func (d *contactDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *providerData, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	d.client = pd.XI
}

func (d *contactDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config contactModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.client.GetContact(ctx, config.ContactName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading contact", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Contact not found", fmt.Sprintf("No contact named %q exists in Nagios.", config.ContactName.ValueString()))
		return
	}

	diags := modelFromContact(ctx, &config, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
