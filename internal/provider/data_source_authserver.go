package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ datasource.DataSource              = &authServerDataSource{}
	_ datasource.DataSourceWithConfigure = &authServerDataSource{}
)

func NewAuthServerDataSource() datasource.DataSource {
	return &authServerDataSource{}
}

type authServerDataSource struct {
	client *client.Client
}

func (d *authServerDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_authserver"
}

func (d *authServerDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Nagios XI authentication server.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the authentication server",
			},
			"enabled":               schema.BoolAttribute{Computed: true, Description: "Determines whether or not this authentication server is enabled"},
			"connection_method":     schema.StringAttribute{Computed: true, Description: "The connection method used for authentication. This value can be either 'ad' or 'ldap'"},
			"ad_account_suffix":     schema.StringAttribute{Computed: true, Description: "The account suffix used when the connection method is 'ad'"},
			"ad_domain_controllers": schema.StringAttribute{Computed: true, Description: "A comma separated list of domain controllers used for Active Directory authentication"},
			"base_dn":               schema.StringAttribute{Computed: true, Description: "The Base DN where the user accounts exist in AD or LDAP that will be authenticating to Nagios"},
			"security_level":        schema.StringAttribute{Computed: true, Description: "The security level used to encrypt the connection. It can be either 'none', 'ssl' or 'tls'"},
			"ldap_port":             schema.StringAttribute{Computed: true, Description: "The TCP port used when connecting with LDAP"},
			"ldap_host":             schema.StringAttribute{Computed: true, Description: "The LDAP host name or IP address connected to"},
		},
	}
}

func (d *authServerDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *authServerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config authServerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.client.GetAuthServer(ctx, config.ServerID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading authentication server", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Authentication server not found", fmt.Sprintf("No authentication server with ID %q exists in Nagios.", config.ServerID.ValueString()))
		return
	}

	modelFromAuthServer(&config, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
