package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	Region         types.String `tfsdk:"region"`
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
		MarkdownDescription: "The CloudKeeper provider is used to interact with CloudKeeper-Auth resources. " +
			"The provider needs to be configured with the proper credentials before it can be used.",
		Attributes: map[string]schema.Attribute{
			"prism_subdomain": schema.StringAttribute{
				MarkdownDescription: "The Prism subdomain for CloudKeeper API paths. Can also be set via the `PRISM_SUBDOMAIN` environment variable.",
				Optional:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "The API token for authentication with CloudKeeper. Can also be set via the `PRISM_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "The region for the Prism API endpoint. Must be either `prism` (default) or `prism-eu`. Can also be set via the `PRISM_REGION` environment variable.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("prism", "prism-eu"),
				},
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
	region := os.Getenv("PRISM_REGION")

	if !data.PrismSubdomain.IsNull() {
		prismSubdomain = data.PrismSubdomain.ValueString()
	}

	if !data.APIToken.IsNull() {
		apiToken = data.APIToken.ValueString()
	}

	if !data.Region.IsNull() {
		region = data.Region.ValueString()
	}

	// Default region to "prism" if not set
	if region == "" {
		region = "prism"
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

	if resp.Diagnostics.HasError() {
		return
	}

	// Build the base URL from the region
	baseUrl := fmt.Sprintf("https://%s.cloudkeeper.com:8090", region)

	// Create a new CloudKeeper client using the configuration values
	client := NewClient(baseUrl, prismSubdomain, apiToken)

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
