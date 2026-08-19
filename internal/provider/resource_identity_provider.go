package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
		MarkdownDescription: "Manages an identity provider configuration in CloudKeeper. Supports Google, Microsoft Azure AD, " +
			"Keycloak federation, custom OIDC providers, and SAML 2.0 providers such as Microsoft Entra ID, Okta and ADFS.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier for the identity provider",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The type of identity provider (google, microsoft, keycloak, custom, saml). " +
					"`custom` is a generic OIDC provider; `saml` is a SAML 2.0 provider.",
				Validators: []validator.String{
					stringvalidator.OneOf("google", "microsoft", "keycloak", "custom", "saml"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"alias": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The alias that identifies this identity provider in Prism. Assigned when the " +
					"provider is created and stable for as long as the resource exists. It forms part of the import " +
					"ID and of the provider's redirect URI; retrieve it from the Prism admin console or `terraform show`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "JSON-encoded configuration for the identity provider, normally written with " +
					"`jsonencode()`. The accepted keys depend on `type` - see the configuration reference below. Keys " +
					"that are not recognised for the given type are ignored, so a misspelled key produces a provider " +
					"that applies cleanly but does not work.",
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

	// Always use the alias returned by the API. It is generated from the display
	// name (GenerateURLSafeAlias) plus a uniqueness suffix, so it cannot be
	// predicted from the type alone.
	data.Alias = types.StringValue(created.Alias)

	// The API exposes no separate identifier for an IdP, so the alias doubles as
	// the resource ID. It is the key every other endpoint is addressed by.
	data.ID = types.StringValue(created.Alias)

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
		// Removed outside Terraform - drop it from state so it can be recreated
		// instead of wedging every subsequent plan behind a hard error.
		if isDependencyNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read identity provider, got error: %s", err))
		return
	}

	if idp.Alias != "" {
		data.Alias = types.StringValue(idp.Alias)
	}
	// The alias doubles as the resource ID (see Create).
	data.ID = data.Alias

	if idp.DisplayName != "" {
		data.DisplayName = types.StringValue(idp.DisplayName)
	}

	// Adopt enabled only when state has no value, i.e. straight after an import.
	// The backend hardcodes enabled=true for some IdP types, so taking the API
	// value unconditionally would fight a configured `enabled = false`.
	if data.Enabled.IsNull() {
		data.Enabled = types.BoolValue(idp.Enabled)
	}

	// API doesn't return sensitive config fields (clientId, clientSecret, etc.)
	// Keep the existing state config value to avoid drift on sensitive fields.
	// After an import it is null, so the first plan shows a diff for config.

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IdentityProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state IdentityProviderResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(data.Config.ValueString()), &config); err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", fmt.Sprintf("Unable to parse config JSON: %s", err))
		return
	}

	// The alias is backend-generated and never changes on update, so read it from
	// prior state rather than the plan.
	alias := state.Alias.ValueString()

	// display_name is Optional, but the backend PUT replaces the whole IdP and an
	// empty displayName clears it in Keycloak. Fall back to the value already in
	// state when the configuration omits it.
	displayName := data.DisplayName.ValueString()
	if displayName == "" {
		displayName = state.DisplayName.ValueString()
	}

	idp := &IdentityProvider{
		Alias:       alias,
		DisplayName: displayName,
		Enabled:     data.Enabled.ValueBool(),
		Config:      config,
	}

	if _, err := r.client.UpdateIdentityProvider(data.Type.ValueString(), alias, idp); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update identity provider, got error: %s", err))
		return
	}

	// display_name, enabled and config keep their planned values. The API does not
	// echo back sensitive config fields, and overwriting an Optional attribute with
	// a server value that differs from the plan is rejected by Terraform as an
	// inconsistent result.

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
		// Already gone (e.g. removed in the Prism UI) - treat as success so destroy
		// can complete instead of leaving the resource stranded in state.
		if isDependencyNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete identity provider, got error: %s", err))
		return
	}
}

func (r *IdentityProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Read() needs both the type and the alias to build the request path, and
	// neither can be derived from the other, so the import ID carries both.
	typeName, alias, found := strings.Cut(req.ID, ":")
	if !found || typeName == "" || alias == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import ID in the format \"<type>:<alias>\" (for example \"custom:okta\"), got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), typeName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("alias"), alias)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), alias)...)
}
