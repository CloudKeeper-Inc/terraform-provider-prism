package provider

import (
	"context"
	"fmt"
	"strings"

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
	AccountIDs      types.List   `tfsdk:"account_ids"`
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
			"account_ids": schema.ListAttribute{
				ElementType:         types.StringType,
				Required:            true,
				MarkdownDescription: "List of AWS account IDs to grant access to",
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

	// Extract account IDs from the list
	var accountIDs []string
	resp.Diagnostics.Append(data.AccountIDs.ElementsAs(ctx, &accountIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	assignment := &PermissionSetAssignment{
		PermissionSetID: data.PermissionSetID.ValueString(),
		PrincipalType:   data.PrincipalType.ValueString(),
		AccountIDs:      accountIDs,
	}

	// Set principal name based on type
	if data.PrincipalType.ValueString() == "USER" {
		assignment.Username = data.PrincipalID.ValueString()
	} else if data.PrincipalType.ValueString() == "GROUP" {
		assignment.GroupName = data.PrincipalID.ValueString()
	}

	_, err := r.client.CreatePermissionSetAssignment(assignment)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create permission set assignment, got error: %s", err))
		return
	}

	// Generate a composite ID representing this assignment configuration
	// Format: permissionSetId:principalType:principalId:accountId1,accountId2,...
	compositeID := fmt.Sprintf("%s:%s:%s:%s",
		data.PermissionSetID.ValueString(),
		data.PrincipalType.ValueString(),
		data.PrincipalID.ValueString(),
		strings.Join(accountIDs, ","))

	data.ID = types.StringValue(compositeID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PermissionSetAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PermissionSetAssignmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Parse the composite ID to get account IDs
	var accountIDs []string
	resp.Diagnostics.Append(data.AccountIDs.ElementsAs(ctx, &accountIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List all assignments and verify our assignments still exist
	assignments, err := r.client.ListPermissionSetAssignments()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list permission set assignments, got error: %s", err))
		return
	}

	// Check if assignments for our permission set + principal + accounts still exist
	principalID := data.PrincipalID.ValueString()
	permSetID := data.PermissionSetID.ValueString()
	principalType := data.PrincipalType.ValueString()

	foundCount := 0
	for _, assignment := range assignments {
		if assignment.PermissionSetID != permSetID {
			continue
		}
		if assignment.PrincipalType != principalType {
			continue
		}

		// Check principal ID matches (could be username or group name)
		principalMatches := false
		if principalType == "USER" && assignment.Username == principalID {
			principalMatches = true
		} else if principalType == "GROUP" && assignment.GroupName == principalID {
			principalMatches = true
		}

		if !principalMatches {
			continue
		}

		// Check if this assignment is for one of our accounts
		for _, accID := range accountIDs {
			if assignment.AccountID == accID {
				foundCount++
				break
			}
		}
	}

	// If none of the assignments exist, remove from state
	if foundCount == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	// Keep the state as is - we don't update individual fields
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

	// Parse the composite ID to get account IDs
	var accountIDs []string
	resp.Diagnostics.Append(data.AccountIDs.ElementsAs(ctx, &accountIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List all assignments to find the ones we need to delete
	assignments, err := r.client.ListPermissionSetAssignments()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list permission set assignments, got error: %s", err))
		return
	}

	// Find and delete assignments matching our configuration
	principalID := data.PrincipalID.ValueString()
	permSetID := data.PermissionSetID.ValueString()
	principalType := data.PrincipalType.ValueString()

	var deleteErrors []string
	for _, assignment := range assignments {
		if assignment.PermissionSetID != permSetID {
			continue
		}
		if assignment.PrincipalType != principalType {
			continue
		}

		// Check principal ID matches
		principalMatches := false
		if principalType == "USER" && assignment.Username == principalID {
			principalMatches = true
		} else if principalType == "GROUP" && assignment.GroupName == principalID {
			principalMatches = true
		}

		if !principalMatches {
			continue
		}

		// Check if this assignment is for one of our accounts
		for _, accID := range accountIDs {
			if assignment.AccountID == accID {
				// Delete this assignment
				err := r.client.DeletePermissionSetAssignment(assignment.ID)
				if err != nil {
					deleteErrors = append(deleteErrors, fmt.Sprintf("account %s: %s", accID, err.Error()))
				}
				break
			}
		}
	}

	if len(deleteErrors) > 0 {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Failed to delete some permission set assignments: %s", strings.Join(deleteErrors, "; ")))
	}
}

func (r *PermissionSetAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
