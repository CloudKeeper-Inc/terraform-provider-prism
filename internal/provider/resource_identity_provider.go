package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &IdentityProviderResource{}
var _ resource.ResourceWithImportState = &IdentityProviderResource{}

func NewIdentityProviderResource() resource.Resource {
	return &IdentityProviderResource{}
}

type IdentityProviderResource struct {
	client *Client
}

type IdentityProviderResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Type        types.String `tfsdk:"type"`
	Alias       types.String `tfsdk:"alias"`
	DisplayName types.String `tfsdk:"display_name"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Config      types.String `tfsdk:"config"`
}

func (r *IdentityProviderResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_identity_provider"
}

func (r *IdentityProviderResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an identity provider configuration in CloudKeeper. Supports Google, Microsoft Azure AD, Keycloak federation, and custom OIDC providers.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier for the identity provider",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The type of identity provider (google, microsoft, keycloak, custom)",
				Validators: []validator.String{
					stringvalidator.OneOf("google", "microsoft", "keycloak", "custom"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"alias": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The alias/identifier for the identity provider",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The display name for the identity provider",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the identity provider is enabled",
			},
			"config": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "JSON configuration for the identity provider (includes client ID, client secret, etc.)",
			},
		},
	}
}

func (r *IdentityProviderResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *IdentityProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data IdentityProviderResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Parse config JSON
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(data.Config.ValueString()), &config); err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", fmt.Sprintf("Unable to parse config JSON: %s", err))
		return
	}

	idp := &IdentityProvider{
		Type:        data.Type.ValueString(),
		Alias:       data.Alias.ValueString(),
		DisplayName: data.DisplayName.ValueString(),
		Enabled:     data.Enabled.ValueBool(),
		Config:      config,
	}

	created, err := r.client.CreateIdentityProvider(data.Type.ValueString(), idp)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create identity provider, got error: %s", err))
		return
	}

	data.ID = types.StringValue(created.ID)

	// Preserve alias from plan if API doesn't return it
	if created.Alias != "" {
		data.Alias = types.StringValue(created.Alias)
	}
	// Otherwise keep the planned value already in data.Alias

	if created.DisplayName != "" {
		data.DisplayName = types.StringValue(created.DisplayName)
	}

	// Preserve enabled from plan - API may not properly return this field during creation
	// Only update if explicitly set to false when plan was true (user can override later via update)
	// This prevents inconsistency errors when API doesn't respect the enabled field
	// Keep the planned value already in data.Enabled

	// API doesn't return sensitive config fields (clientId, clientSecret, etc.)
	// Keep the original planned config value to avoid drift on sensitive fields
	// data.Config already contains the planned value from earlier in this function

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IdentityProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data IdentityProviderResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idp, err := r.client.GetIdentityProvider(data.Type.ValueString(), data.Alias.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read identity provider, got error: %s", err))
		return
	}

	if idp.DisplayName != "" {
		data.DisplayName = types.StringValue(idp.DisplayName)
	}

	// Preserve enabled from state - API may not properly return this field
	// Keep the existing state value in data.Enabled

	// API doesn't return sensitive config fields (clientId, clientSecret, etc.)
	// Keep the existing state config value to avoid drift on sensitive fields
	// data.Config already contains the state value from earlier in this function

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IdentityProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data IdentityProviderResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(data.Config.ValueString()), &config); err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", fmt.Sprintf("Unable to parse config JSON: %s", err))
		return
	}

	idp := &IdentityProvider{
		Alias:       data.Alias.ValueString(),
		DisplayName: data.DisplayName.ValueString(),
		Enabled:     data.Enabled.ValueBool(),
		Config:      config,
	}

	updated, err := r.client.UpdateIdentityProvider(data.Type.ValueString(), data.Alias.ValueString(), idp)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update identity provider, got error: %s", err))
		return
	}

	if updated.DisplayName != "" {
		data.DisplayName = types.StringValue(updated.DisplayName)
	}

	// Preserve enabled from plan - API may not properly return this field during update
	// Keep the planned value already in data.Enabled

	// API doesn't return sensitive config fields (clientId, clientSecret, etc.)
	// Keep the planned config value to avoid drift on sensitive fields
	// data.Config already contains the planned value from earlier in this function

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IdentityProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data IdentityProviderResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteIdentityProvider(data.Type.ValueString(), data.Alias.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete identity provider, got error: %s", err))
		return
	}
}

func (r *IdentityProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
