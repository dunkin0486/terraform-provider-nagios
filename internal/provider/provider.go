package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

var _ provider.Provider = &nagiosProvider{}

type nagiosProvider struct {
	version string
}

// providerData is what Configure hands to every resource/data source as
// ProviderData. XI is always non-nil (the provider's url/token are
// required eagerly, see Configure below). NNA is nil unless nna_url/
// nna_api_key were actually set - those credentials are Optional and
// unvalidated here so that existing XI-only configs keep working
// unchanged; it's each nna_* resource's own Configure that errors if it's
// declared without NNA nil.
type providerData struct {
	XI  *client.Client
	NNA *nna.Client
}

// New returns a factory for a fresh instance of the Nagios provider.
// version is injected by GoReleaser's ldflags at build time (see main.go)
// and reported back to Terraform via Metadata.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &nagiosProvider{version: version}
	}
}

type nagiosProviderModel struct {
	URL       types.String `tfsdk:"url"`
	Token     types.String `tfsdk:"token"`
	NNAURL    types.String `tfsdk:"nna_url"`
	NNAAPIKey types.String `tfsdk:"nna_api_key"`
}

func (p *nagiosProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "nagios"
	resp.Version = p.version
}

func (p *nagiosProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interacts with a Nagios XI instance's REST API.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:    true,
				Description: "The URL of the Nagios XI application. Defaults to the NAGIOS_URL environment variable if not set.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "API token to authenticate to Nagios XI. Defaults to the API_TOKEN environment variable if not set.",
			},
			"nna_url": schema.StringAttribute{
				Optional:    true,
				Description: "The URL of the Nagios Network Analyzer application. Only required if nna_* resources/data sources are used. Defaults to the NNA_URL environment variable if not set.",
			},
			"nna_api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "API token to authenticate to Nagios Network Analyzer. Only required if nna_* resources/data sources are used. Defaults to the NNA_API_KEY environment variable if not set.",
			},
		},
	}
}

// Configure mirrors the old SDKv1 provider's schema.EnvDefaultFunc behavior:
// url/token are Optional in the schema (Terraform won't force them in HCL)
// but Configure enforces that one of {config value, env var} is present.
func (p *nagiosProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config nagiosProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	url := config.URL.ValueString()
	if config.URL.IsNull() {
		url = os.Getenv("NAGIOS_URL")
	}

	token := config.Token.ValueString()
	if config.Token.IsNull() {
		token = os.Getenv("API_TOKEN")
	}

	if url == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("url"),
			"Missing Nagios XI URL",
			"The provider requires a URL, set either via the url attribute or the NAGIOS_URL environment variable.",
		)
	}

	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing Nagios XI API Token",
			"The provider requires an API token, set either via the token attribute or the API_TOKEN environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Unlike url/token above, nna_url/nna_api_key are never hard-required
	// here - most configs don't use any nna_* resource, and forcing every
	// existing XI-only user to also set these would be a breaking change.
	// A missing value just leaves pd.NNA nil; each nna_* resource's own
	// Configure errors on that if it's actually declared.
	nnaURL := config.NNAURL.ValueString()
	if config.NNAURL.IsNull() {
		nnaURL = os.Getenv("NNA_URL")
	}

	nnaAPIKey := config.NNAAPIKey.ValueString()
	if config.NNAAPIKey.IsNull() {
		nnaAPIKey = os.Getenv("NNA_API_KEY")
	}

	pd := &providerData{XI: client.NewClient(url, token)}
	if nnaURL != "" && nnaAPIKey != "" {
		pd.NNA = nna.NewClient(nnaURL, nnaAPIKey)
	}

	resp.ResourceData = pd
	resp.DataSourceData = pd
}

func (p *nagiosProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewHostResource,
		NewHostgroupResource,
		NewContactResource,
		NewContactgroupResource,
		NewServiceResource,
		NewServicegroupResource,
		NewAuthServerResource,
		NewTimeperiodResource,
		NewCommandResource,
		NewNNASourceResource,
		NewNNASourceGroupResource,
		NewNNAUserResource,
		NewUserResource,
	}
}

func (p *nagiosProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewHostDataSource,
		NewHostgroupDataSource,
		NewServiceDataSource,
		NewContactDataSource,
		NewContactgroupDataSource,
		NewServicegroupDataSource,
		NewAuthServerDataSource,
		NewUserDataSource,
	}
}
