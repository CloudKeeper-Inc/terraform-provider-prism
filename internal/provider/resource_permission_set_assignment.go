package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &PermissionSetAssignmentResource{}
var _ resource.ResourceWithImportState = &PermissionSetAssignmentResource{}

func NewPermissionSetAssignmentResource() resource.Resource {
	return &PermissionSetAssignmentResource{}
}

type PermissionSetAssignmentResource struct {
	client *Client
}

type PermissionSetAssignmentResourceModel struct {
	ID              types.String `tfsdk:"id"`
	PermissionSetID types.String `tfsdk:"permission_set_id"`
	PrincipalType   types.String `tfsdk:"principal_type"`
	PrincipalID     types.String `tfsdk:"principal_id"`
	AccountID       types.String `tfsdk:"account_id"`
}

func (r *PermissionSetAssignmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_set_assignment"
}

func (r *PermissionSetAssignmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a permission set to a user or group for a specific AWS account.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier for the assignment",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"permission_set_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the permission set to assign",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"principal_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The type of principal (USER or GROUP)",
				Validators: []validator.String{
					stringvalidator.OneOf("USER", "GROUP"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"principal_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID or email of the user/group",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The AWS account ID to grant access to",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *PermissionSetAssignmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PermissionSetAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PermissionSetAssignmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	assignment := &PermissionSetAssignment{
		PermissionSetID: data.PermissionSetID.ValueString(),
		PrincipalType:   data.PrincipalType.ValueString(),
		PrincipalID:     data.PrincipalID.ValueString(),
		AccountID:       data.AccountID.ValueString(),
	}

	created, err := r.client.CreatePermissionSetAssignment(assignment)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create permission set assignment, got error: %s", err))
		return
	}

	data.ID = types.StringValue(created.ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PermissionSetAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PermissionSetAssignmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	assignment, err := r.client.GetPermissionSetAssignment(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read permission set assignment, got error: %s", err))
		return
	}

	data.PermissionSetID = types.StringValue(assignment.PermissionSetID)
	data.PrincipalType = types.StringValue(assignment.PrincipalType)
	data.PrincipalID = types.StringValue(assignment.PrincipalID)
	data.AccountID = types.StringValue(assignment.AccountID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PermissionSetAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Permission set assignments cannot be updated; they must be replaced
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"Permission set assignments cannot be updated. They must be destroyed and recreated.",
	)
}

func (r *PermissionSetAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PermissionSetAssignmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeletePermissionSetAssignment(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete permission set assignment, got error: %s", err))
		return
	}
}

func (r *PermissionSetAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
