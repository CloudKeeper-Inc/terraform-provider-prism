package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

type UserResource struct {
	client *Client
}

type UserResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Username   types.String `tfsdk:"username"`
	Email      types.String `tfsdk:"email"`
	FirstName  types.String `tfsdk:"first_name"`
	LastName   types.String `tfsdk:"last_name"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	Attributes types.Map    `tfsdk:"attributes"`
}

func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a CloudKeeper user in a customer realm.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier for the user",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"username": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The username for the user",
			},
			"email": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The email address of the user",
			},
			"first_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The first name of the user",
			},
			"last_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The last name of the user",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the user account is enabled",
			},
			"attributes": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Custom attributes for the user",
			},
		},
	}
}

func (r *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tfAttributes map[string]string
	var apiAttributes map[string][]string
	if !data.Attributes.IsNull() {
		resp.Diagnostics.Append(data.Attributes.ElementsAs(ctx, &tfAttributes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		// Convert map[string]string to map[string][]string for API
		apiAttributes = make(map[string][]string)
		for k, v := range tfAttributes {
			apiAttributes[k] = []string{v}
		}
	}

	user := &User{
		Username:   data.Username.ValueString(),
		Email:      data.Email.ValueString(),
		FirstName:  data.FirstName.ValueString(),
		LastName:   data.LastName.ValueString(),
		Enabled:    data.Enabled.ValueBool(),
		Attributes: apiAttributes,
	}

	created, err := r.client.CreateUser(user)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create user, got error: %s", err))
		return
	}

	data.ID = types.StringValue(created.ID)
	data.Username = types.StringValue(created.Username)
	// Only update email if API returned a non-empty value
	if created.Email != "" {
		data.Email = types.StringValue(created.Email)
	}
	if created.FirstName != "" {
		data.FirstName = types.StringValue(created.FirstName)
	}
	if created.LastName != "" {
		data.LastName = types.StringValue(created.LastName)
	}
	// Only update enabled if it's explicitly true (preserve plan value if API returns false/default)
	if created.Enabled {
		data.Enabled = types.BoolValue(created.Enabled)
	}

	if len(created.Attributes) > 0 {
		// Convert map[string][]string from API to map[string]string for Terraform
		tfAttributesMap := make(map[string]string)
		for k, v := range created.Attributes {
			if len(v) > 0 {
				tfAttributesMap[k] = v[0] // Take first value
			}
		}
		attributesMap, diags := types.MapValueFrom(ctx, types.StringType, tfAttributesMap)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Attributes = attributesMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := r.client.GetUser(data.Username.ValueString())
	if err != nil {
		// If the resource is not found (404), remove it from state
		if strings.Contains(err.Error(), "404") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
		return
	}

	data.Username = types.StringValue(user.Username)
	// Only update email if API returned a non-empty value
	if user.Email != "" {
		data.Email = types.StringValue(user.Email)
	}
	if user.FirstName != "" {
		data.FirstName = types.StringValue(user.FirstName)
	}
	if user.LastName != "" {
		data.LastName = types.StringValue(user.LastName)
	}
	// Only update enabled if it's explicitly true (preserve state value if API returns false/default)
	if user.Enabled {
		data.Enabled = types.BoolValue(user.Enabled)
	}

	if len(user.Attributes) > 0 {
		// Convert map[string][]string from API to map[string]string for Terraform
		tfAttributesMap := make(map[string]string)
		for k, v := range user.Attributes {
			if len(v) > 0 {
				tfAttributesMap[k] = v[0] // Take first value
			}
		}
		attributesMap, diags := types.MapValueFrom(ctx, types.StringType, tfAttributesMap)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Attributes = attributesMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data UserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tfAttributes map[string]string
	var apiAttributes map[string][]string
	if !data.Attributes.IsNull() {
		resp.Diagnostics.Append(data.Attributes.ElementsAs(ctx, &tfAttributes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		// Convert map[string]string to map[string][]string for API
		apiAttributes = make(map[string][]string)
		for k, v := range tfAttributes {
			apiAttributes[k] = []string{v}
		}
	}

	user := &User{
		Username:   data.Username.ValueString(),
		Email:      data.Email.ValueString(),
		FirstName:  data.FirstName.ValueString(),
		LastName:   data.LastName.ValueString(),
		Enabled:    data.Enabled.ValueBool(),
		Attributes: apiAttributes,
	}

	updated, err := r.client.UpdateUser(data.Username.ValueString(), user)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update user, got error: %s", err))
		return
	}

	data.Username = types.StringValue(updated.Username)
	// Only update email if API returned a non-empty value
	if updated.Email != "" {
		data.Email = types.StringValue(updated.Email)
	}
	if updated.FirstName != "" {
		data.FirstName = types.StringValue(updated.FirstName)
	}
	if updated.LastName != "" {
		data.LastName = types.StringValue(updated.LastName)
	}
	// Only update enabled if it's explicitly true (preserve plan value if API returns false/default)
	if updated.Enabled {
		data.Enabled = types.BoolValue(updated.Enabled)
	}

	if len(updated.Attributes) > 0 {
		// Convert map[string][]string from API to map[string]string for Terraform
		tfAttributesMap := make(map[string]string)
		for k, v := range updated.Attributes {
			if len(v) > 0 {
				tfAttributesMap[k] = v[0] // Take first value
			}
		}
		attributesMap, diags := types.MapValueFrom(ctx, types.StringType, tfAttributesMap)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Attributes = attributesMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteUser(data.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete user, got error: %s", err))
		return
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
