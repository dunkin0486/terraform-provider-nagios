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
	_ datasource.DataSource              = &servicegroupDataSource{}
	_ datasource.DataSourceWithConfigure = &servicegroupDataSource{}
)

func NewServicegroupDataSource() datasource.DataSource {
	return &servicegroupDataSource{}
}

type servicegroupDataSource struct {
	client *client.Client
}

func (d *servicegroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_servicegroup"
}

func (d *servicegroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Nagios XI servicegroup.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the Nagios servicegroup",
			},
			"alias":                schema.StringAttribute{Computed: true, Description: "The description or other name that the servicegroup may be called"},
			"members":              schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "A list of services and/or service groups that are members of the service group"},
			"servicegroup_members": schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "A list of other servicegroups whose member services are included in this group (nested servicegroups)"},
			"notes":                schema.StringAttribute{Computed: true, Description: "Notes about the servicegroup that may assist with troubleshooting"},
			"notes_url":            schema.StringAttribute{Computed: true, Description: "URL to a third-party documentation repository containing more information about the servicegroup"},
			"action_url":           schema.StringAttribute{Computed: true, Description: "URL to a third-party documentation repository containing actions to take in the event the servicegroup goes down"},
		},
	}
}

func (d *servicegroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *servicegroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config servicegroupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.client.GetServicegroup(ctx, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading servicegroup", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Servicegroup not found", fmt.Sprintf("No servicegroup named %q exists in Nagios.", config.Name.ValueString()))
		return
	}

	diags := modelFromServicegroup(ctx, &config, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
