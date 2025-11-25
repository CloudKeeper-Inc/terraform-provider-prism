package provider

import (
	"context"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure CloudKeeperProvider satisfies various provider interfaces.
var _ provider.Provider = &CloudKeeperProvider{}

// CloudKeeperProvider defines the provider implementation.
type CloudKeeperProvider struct {
	version string
}

// CloudKeeperProviderModel describes the provider data model.
type CloudKeeperProviderModel struct {
	PrismSubdomain types.String `tfsdk:"prism_subdomain"`
	APIToken       types.String `tfsdk:"api_token"`
	BaseURL        types.String `tfsdk:"base_url"`
}

// New creates a new provider instance
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CloudKeeperProvider{
			version: version,
		}
	}
}

// Metadata returns the provider type name.
func (p *CloudKeeperProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "prism"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *CloudKeeperProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The CloudKeeper provider is used to interact with CloudKeeper Prism resources. " +
			"The provider needs to be configured with the proper credentials before it can be used.",
		Attributes: map[string]schema.Attribute{
			"prism_subdomain": schema.StringAttribute{
				MarkdownDescription: "The Prism subdomain for CloudKeeper API paths (e.g., `https://sso.prism.cloudkeeper.com`). Can also be set via the `PRISM_SUBDOMAIN` environment variable.",
				Optional:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "The API token for authentication with CloudKeeper. Can also be set via the `PRISM_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"base_url": schema.StringAttribute{
				MarkdownDescription: "The base URL for the Prism API endpoint (e.g., `https://prism.cloudkeeper.com`). The port 8090 is automatically appended. Can also be set via the `PRISM_BASE_URL` environment variable.",
				Optional:            true,
			},
		},
	}
}

// Configure prepares a CloudKeeper API client for data sources and resources.
func (p *CloudKeeperProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data CloudKeeperProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Configuration values are now available in data

	// If practitioner provided a configuration value for any of the
	// attributes, it must be a known value.

	if data.PrismSubdomain.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("prism_subdomain"),
			"Unknown CloudKeeper Prism Subdomain",
			"The provider cannot create the CloudKeeper API client as there is an unknown configuration value for the CloudKeeper Prism subdomain. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the PRISM_SUBDOMAIN environment variable.",
		)
	}

	if data.APIToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Unknown CloudKeeper API Token",
			"The provider cannot create the CloudKeeper API client as there is an unknown configuration value for the CloudKeeper API token. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the PRISM_API_TOKEN environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	prismSubdomain := os.Getenv("PRISM_SUBDOMAIN")
	apiToken := os.Getenv("PRISM_API_TOKEN")
	baseURL := os.Getenv("PRISM_BASE_URL")

	if !data.PrismSubdomain.IsNull() {
		prismSubdomain = data.PrismSubdomain.ValueString()
	}

	if !data.APIToken.IsNull() {
		apiToken = data.APIToken.ValueString()
	}

	if !data.BaseURL.IsNull() {
		baseURL = data.BaseURL.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.

	if prismSubdomain == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("prism_subdomain"),
			"Missing CloudKeeper Prism Subdomain",
			"The provider cannot create the CloudKeeper API client as there is a missing or empty value for the CloudKeeper Prism subdomain. "+
				"Set the prism_subdomain value in the configuration or use the PRISM_SUBDOMAIN environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if apiToken == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Missing CloudKeeper API Token",
			"The provider cannot create the CloudKeeper API client as there is a missing or empty value for the CloudKeeper API token. "+
				"Set the api_token value in the configuration or use the PRISM_API_TOKEN environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if baseURL == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("base_url"),
			"Missing CloudKeeper Prism Base URL",
			"The provider cannot create the CloudKeeper API client as there is a missing or empty value for the CloudKeeper Prism base URL. "+
				"Set the base_url value in the configuration or use the PRISM_BASE_URL environment variable. "+
				"Example: https://prism.cloudkeeper.com",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Ensure base URL doesn't have trailing slash and append port
	baseURL = strings.TrimSuffix(baseURL, "/")
	finalBaseURL := baseURL + ":8090"

	// Create a new CloudKeeper client using the configuration values
	client := NewClient(finalBaseURL, prismSubdomain, apiToken)

	// Make the CloudKeeper client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

// Resources defines the resources implemented in the provider.
func (p *CloudKeeperProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAWSAccountResource,
		NewPermissionSetResource,
		NewPermissionSetAssignmentResource,
		NewUserResource,
		NewGroupResource,
		NewGroupMembershipResource,
		NewIdentityProviderResource,
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *CloudKeeperProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAWSAccountDataSource,
		NewPermissionSetDataSource,
		NewUserDataSource,
		NewGroupDataSource,
	}
}
