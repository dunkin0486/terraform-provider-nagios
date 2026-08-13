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
	_ datasource.DataSource              = &userDataSource{}
	_ datasource.DataSourceWithConfigure = &userDataSource{}
)

func NewUserDataSource() datasource.DataSource {
	return &userDataSource{}
}

type userDataSource struct {
	client *client.Client
}

// userDataSourceModel is deliberately its own type rather than a reuse of
// userModel: terraform-plugin-framework's reflection requires an exact 1:1
// match between a struct's tfsdk tags and its schema's attributes in both
// directions, and this data source's schema is a strict subset of the
// resource's - it has no field to expose for any of the five write-only
// attributes (password/auth_level/force_pw_change/auth_type/
// auth_server_id), which have nothing readable to report (#174). Every
// other data source in this provider mirrors its resource's schema 1:1, so
// reusing the resource model works there but not here.
type userDataSourceModel struct {
	UserID   types.String `tfsdk:"user_id"`
	Username types.String `tfsdk:"username"`
	Name     types.String `tfsdk:"name"`
	Email    types.String `tfsdk:"email"`
	Enabled  types.Bool   `tfsdk:"enabled"`
}

func (d *userDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

// Schema exposes only the fields Nagios's GET actually returns (#174) -
// password/auth_level/force_pw_change/auth_type/auth_server_id are
// write-only on the resource and have no readable value to expose here.
func (d *userDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Nagios XI login/admin panel user account by username.",
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				Required:    true,
				Description: "The login username to look up",
			},
			"user_id": schema.StringAttribute{Computed: true, Description: "The ID of the user"},
			"name":    schema.StringAttribute{Computed: true, Description: "The user's full name"},
			"email":   schema.StringAttribute{Computed: true, Description: "The user's email address"},
			"enabled": schema.BoolAttribute{Computed: true, Description: "Determines whether or not this user account is enabled"},
		},
	}
}

func (d *userDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config userDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.client.GetUser(ctx, config.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("User not found", fmt.Sprintf("No user with username %q exists in Nagios.", config.Username.ValueString()))
		return
	}

	config.UserID = types.StringValue(got.ID)
	config.Username = types.StringValue(got.Username)
	config.Name = types.StringValue(got.Name)
	config.Email = types.StringValue(got.Email)
	config.Enabled = nagiosToOptionalBool(got.Enabled)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
