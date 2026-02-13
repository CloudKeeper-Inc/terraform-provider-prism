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

	// Wait for dependencies to become available before creating
	permSetID := data.PermissionSetID.ValueString()
	if err := waitForDependency(ctx, "permission_set", permSetID, func() error {
		_, err := r.client.GetPermissionSet(permSetID)
		return err
	}); err != nil {
		resp.Diagnostics.AddError("Dependency Error", fmt.Sprintf("Permission set dependency not satisfied: %s", err))
		return
	}

	for _, acctID := range accountIDs {
		if err := waitForDependency(ctx, "aws_account", acctID, func() error {
			_, err := r.client.GetAWSAccount(acctID)
			return err
		}); err != nil {
			resp.Diagnostics.AddError("Dependency Error", fmt.Sprintf("AWS account dependency not satisfied: %s", err))
			return
		}
	}

	principalID := data.PrincipalID.ValueString()
	principalType := data.PrincipalType.ValueString()
	if principalType == "USER" {
		if err := waitForDependency(ctx, "user", principalID, func() error {
			_, err := r.client.GetUser(principalID)
			return err
		}); err != nil {
			resp.Diagnostics.AddError("Dependency Error", fmt.Sprintf("User dependency not satisfied: %s", err))
			return
		}
	} else if principalType == "GROUP" {
		if err := waitForDependency(ctx, "group", principalID, func() error {
			_, err := r.client.GetGroup(principalID)
			return err
		}); err != nil {
			resp.Diagnostics.AddError("Dependency Error", fmt.Sprintf("Group dependency not satisfied: %s", err))
			return
		}
	}

	_, err := r.client.CreatePermissionSetAssignment(assignment)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create permission set assignment, got error: %s", err))
		return
	}

	// After creating, we need to find the actual assignment IDs that were created
	// The backend creates one assignment per account, but only returns the first one
	// So we need to list all assignments and find the ones we just created
	assignments, err := r.client.ListPermissionSetAssignments()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list permission set assignments after create, got error: %s", err))
		return
	}

	// Find the assignments we just created by matching all criteria

	var createdAssignmentIDs []string
	for _, acctID := range accountIDs {
		found := false
		for _, apiAssignment := range assignments {
			if apiAssignment.PermissionSetID != permSetID {
				continue
			}
			if apiAssignment.PrincipalType != principalType {
				continue
			}
			if apiAssignment.AccountID != acctID {
				continue
			}

			// Check principal ID matches
			principalMatches := false
			if principalType == "USER" && apiAssignment.Username == principalID {
				principalMatches = true
			} else if principalType == "GROUP" && apiAssignment.GroupName == principalID {
				principalMatches = true
			}

			if principalMatches {
				createdAssignmentIDs = append(createdAssignmentIDs, apiAssignment.ID)
				found = true
				break
			}
		}

		if !found {
			resp.Diagnostics.AddWarning(
				"Assignment Not Found",
				fmt.Sprintf("Could not find assignment for account %s after creation. It may have been created but not immediately visible.", acctID),
			)
		}
	}

	if len(createdAssignmentIDs) == 0 {
		resp.Diagnostics.AddError(
			"No Assignments Found",
			"Failed to locate any of the created assignments. They may have been created but are not yet visible in the API.",
		)
		return
	}

	// Store the actual API assignment IDs in the composite ID
	// Format: assignmentId1,assignmentId2,assignmentId3,...
	compositeID := strings.Join(createdAssignmentIDs, ",")
	data.ID = types.StringValue(compositeID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PermissionSetAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PermissionSetAssignmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Parse the composite ID to get the actual assignment IDs
	assignmentIDs := strings.Split(data.ID.ValueString(), ",")
	if len(assignmentIDs) == 0 {
		resp.Diagnostics.AddError(
			"Invalid State",
			"No assignment IDs found in state. The resource may be corrupted.",
		)
		return
	}

	// Track existing assignments and collect account IDs
	var existingAssignments []*PermissionSetAssignment
	var accountIDs []string

	for _, assignmentID := range assignmentIDs {
		assignment, err := r.client.GetPermissionSetAssignment(assignmentID)
		if err != nil {
			// If 404 or not found, skip it
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
				continue
			}
			// Other errors should be reported
			resp.Diagnostics.AddWarning(
				"API Error",
				fmt.Sprintf("Unable to read assignment %s: %s", assignmentID, err),
			)
			continue
		}
		existingAssignments = append(existingAssignments, assignment)
		accountIDs = append(accountIDs, assignment.AccountID)
	}

	// If none of the assignments exist, remove from state
	if len(existingAssignments) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	// If some but not all assignments exist, that's unusual but we keep the resource
	if len(existingAssignments) < len(assignmentIDs) {
		resp.Diagnostics.AddWarning(
			"Partial Assignment Drift",
			fmt.Sprintf("Only %d of %d assignments still exist. Some may have been deleted outside Terraform.", len(existingAssignments), len(assignmentIDs)),
		)
	}

	// Populate state from the first existing assignment (they should all have same permission_set, principal)
	firstAssignment := existingAssignments[0]
	data.PermissionSetID = types.StringValue(firstAssignment.PermissionSetID)
	data.PrincipalType = types.StringValue(firstAssignment.PrincipalType)

	// Set principal_id based on type
	if firstAssignment.PrincipalType == "USER" {
		data.PrincipalID = types.StringValue(firstAssignment.Username)
	} else {
		data.PrincipalID = types.StringValue(firstAssignment.GroupName)
	}

	// Set account_ids from all existing assignments
	accountIDsValues, diags := types.ListValueFrom(ctx, types.StringType, accountIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.AccountIDs = accountIDsValues

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

	// Parse the composite ID to get the actual assignment IDs we created
	assignmentIDs := strings.Split(data.ID.ValueString(), ",")
	if len(assignmentIDs) == 0 {
		// Nothing to delete
		return
	}

	// Delete each assignment by its actual API ID
	// This ensures we only delete the assignments we created
	var deleteErrors []string
	for _, assignmentID := range assignmentIDs {
		err := r.client.DeletePermissionSetAssignment(assignmentID)
		if err != nil {
			// If already deleted (404), that's OK
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
				continue
			}
			deleteErrors = append(deleteErrors, fmt.Sprintf("assignment %s: %s", assignmentID, err.Error()))
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
