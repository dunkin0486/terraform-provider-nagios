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
	_ datasource.DataSource              = &contactgroupDataSource{}
	_ datasource.DataSourceWithConfigure = &contactgroupDataSource{}
)

func NewContactgroupDataSource() datasource.DataSource {
	return &contactgroupDataSource{}
}

type contactgroupDataSource struct {
	client *client.Client
}

func (d *contactgroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contactgroup"
}

func (d *contactgroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Nagios XI contactgroup.",
		Attributes: map[string]schema.Attribute{
			"contactgroup_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the contact group",
			},
			"alias":                schema.StringAttribute{Computed: true, Description: "A longer description of the contact group"},
			"members":              schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "A list of the short names of contacts that are members of this group"},
			"contactgroup_members": schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "A list of other contactgroups whose members should be included in this group"},
		},
	}
}

func (d *contactgroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *contactgroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config contactgroupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.client.GetContactgroup(ctx, config.ContactgroupName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading contactgroup", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Contactgroup not found", fmt.Sprintf("No contactgroup named %q exists in Nagios.", config.ContactgroupName.ValueString()))
		return
	}

	diags := modelFromContactgroup(ctx, &config, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
