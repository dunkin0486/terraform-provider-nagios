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
	_ datasource.DataSource              = &hostgroupDataSource{}
	_ datasource.DataSourceWithConfigure = &hostgroupDataSource{}
)

func NewHostgroupDataSource() datasource.DataSource {
	return &hostgroupDataSource{}
}

type hostgroupDataSource struct {
	client *client.Client
}

func (d *hostgroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hostgroup"
}

func (d *hostgroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Nagios XI hostgroup.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the hostgroup. It can be up to 255 characters long.",
			},
			"alias":             schema.StringAttribute{Computed: true, Description: "The description of the hostgroup"},
			"members":           schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "Members of this hostgroup"},
			"hostgroup_members": schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "Nested hostgroups whose member hosts are included in this group"},
			"notes":             schema.StringAttribute{Computed: true, Description: "Notes about the hostgroup that may assist with troubleshooting"},
			"notes_url":         schema.StringAttribute{Computed: true, Description: "URL to a third-party documentation repository containing more information about the hostgroup"},
			"action_url":        schema.StringAttribute{Computed: true, Description: "URL to a third-party documentation repository containing actions to take in the event the hostgroup goes down"},
		},
	}
}

func (d *hostgroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *hostgroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config hostgroupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.client.GetHostgroup(ctx, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading hostgroup", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Hostgroup not found", fmt.Sprintf("No hostgroup named %q exists in Nagios.", config.Name.ValueString()))
		return
	}

	diags := modelFromHostgroup(ctx, &config, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
