package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &PermissionSetResource{}
var _ resource.ResourceWithImportState = &PermissionSetResource{}

func NewPermissionSetResource() resource.Resource {
	return &PermissionSetResource{}
}

type PermissionSetResource struct {
	client *Client
}

type PermissionSetResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	SessionDuration types.String `tfsdk:"session_duration"`
	ManagedPolicies types.List   `tfsdk:"managed_policies"`
	InlinePolicies  types.Map    `tfsdk:"inline_policies"`
}

func (r *PermissionSetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_set"
}

func (r *PermissionSetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a CloudKeeper permission set. Permission sets define the IAM policies and session duration for AWS access.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier for the permission set",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the permission set",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description of the permission set",
			},
			"session_duration": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The session duration in ISO 8601 format (e.g., PT4H for 4 hours)",
			},
			"managed_policies": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "List of AWS managed policy ARNs to attach",
			},
			"inline_policies": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Map of inline IAM policy documents in JSON format. The key is the policy name, and the value is the policy document.",
			},
		},
	}
}

func (r *PermissionSetResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PermissionSetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PermissionSetResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert managed policies list to string slice
	var managedPolicies []string
	if !data.ManagedPolicies.IsNull() {
		resp.Diagnostics.Append(data.ManagedPolicies.ElementsAs(ctx, &managedPolicies, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Convert inline policies map
	var inlinePolicies map[string]string
	if !data.InlinePolicies.IsNull() {
		resp.Diagnostics.Append(data.InlinePolicies.ElementsAs(ctx, &inlinePolicies, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	permSet := &PermissionSet{
		Name:            data.Name.ValueString(),
		Description:     data.Description.ValueString(),
		SessionDuration: data.SessionDuration.ValueString(),
		ManagedPolicies: managedPolicies,
		InlinePolicies:  inlinePolicies,
	}

	created, err := r.client.CreatePermissionSet(permSet)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create permission set, got error: %s", err))
		return
	}

	data.ID = types.StringValue(created.ID)
	data.Name = types.StringValue(created.Name)
	data.Description = types.StringValue(created.Description)
	if created.SessionDuration != "" {
		data.SessionDuration = types.StringValue(created.SessionDuration)
	}

	// Convert managed policies back to list
	if len(created.ManagedPolicies) > 0 {
		managedPoliciesList, diags := types.ListValueFrom(ctx, types.StringType, created.ManagedPolicies)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.ManagedPolicies = managedPoliciesList
	}

	// Convert inline policies back to map
	if len(created.InlinePolicies) > 0 {
		inlinePoliciesMap, diags := types.MapValueFrom(ctx, types.StringType, created.InlinePolicies)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.InlinePolicies = inlinePoliciesMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PermissionSetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PermissionSetResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	permSet, err := r.client.GetPermissionSet(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read permission set, got error: %s", err))
		return
	}

	data.Name = types.StringValue(permSet.Name)
	data.Description = types.StringValue(permSet.Description)
	if permSet.SessionDuration != "" {
		data.SessionDuration = types.StringValue(permSet.SessionDuration)
	}

	if len(permSet.ManagedPolicies) > 0 {
		managedPoliciesList, diags := types.ListValueFrom(ctx, types.StringType, permSet.ManagedPolicies)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.ManagedPolicies = managedPoliciesList
	}

	if len(permSet.InlinePolicies) > 0 {
		inlinePoliciesMap, diags := types.MapValueFrom(ctx, types.StringType, permSet.InlinePolicies)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.InlinePolicies = inlinePoliciesMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PermissionSetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PermissionSetResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var managedPolicies []string
	if !data.ManagedPolicies.IsNull() {
		resp.Diagnostics.Append(data.ManagedPolicies.ElementsAs(ctx, &managedPolicies, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Convert inline policies map
	var inlinePolicies map[string]string
	if !data.InlinePolicies.IsNull() {
		resp.Diagnostics.Append(data.InlinePolicies.ElementsAs(ctx, &inlinePolicies, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	permSet := &PermissionSet{
		Name:            data.Name.ValueString(),
		Description:     data.Description.ValueString(),
		SessionDuration: data.SessionDuration.ValueString(),
		ManagedPolicies: managedPolicies,
		InlinePolicies:  inlinePolicies,
	}

	updated, err := r.client.UpdatePermissionSet(data.ID.ValueString(), permSet)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update permission set, got error: %s", err))
		return
	}

	data.Name = types.StringValue(updated.Name)
	data.Description = types.StringValue(updated.Description)
	if updated.SessionDuration != "" {
		data.SessionDuration = types.StringValue(updated.SessionDuration)
	}

	if len(updated.ManagedPolicies) > 0 {
		managedPoliciesList, diags := types.ListValueFrom(ctx, types.StringType, updated.ManagedPolicies)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.ManagedPolicies = managedPoliciesList
	}

	if len(updated.InlinePolicies) > 0 {
		inlinePoliciesMap, diags := types.MapValueFrom(ctx, types.StringType, updated.InlinePolicies)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.InlinePolicies = inlinePoliciesMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PermissionSetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PermissionSetResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	permissionSetID := data.ID.ValueString()

	// Before deleting the permission set, delete all assignments that use it
	// This prevents the "permission set has active assignments" error
	assignments, err := r.client.ListPermissionSetAssignments()
	if err != nil {
		// Log warning but continue - if we can't list assignments, try to delete anyway
		resp.Diagnostics.AddWarning(
			"Unable to List Assignments",
			fmt.Sprintf("Could not list permission set assignments before deleting permission set. If assignments exist, deletion may fail: %s", err),
		)
	} else {
		// Find and delete all assignments for this permission set
		var deleteErrors []string
		var deletedIDs []string

		for _, assignment := range assignments {
			if assignment.PermissionSetID == permissionSetID {
				err := r.client.DeletePermissionSetAssignment(assignment.ID)
				if err != nil {
					// Collect errors but continue trying to delete other assignments
					deleteErrors = append(deleteErrors, fmt.Sprintf("assignment %s: %s", assignment.ID, err.Error()))
				} else {
					deletedIDs = append(deletedIDs, assignment.ID)
				}
			}
		}

		if len(deleteErrors) > 0 {
			resp.Diagnostics.AddWarning(
				"Failed to Delete Some Assignments",
				fmt.Sprintf("Could not delete all assignments for permission set. Deletion may fail. Errors: %s",
					strings.Join(deleteErrors, "; ")),
			)
		}

		if len(deletedIDs) > 0 {
			resp.Diagnostics.AddWarning(
				"Automatic Assignment Cleanup",
				fmt.Sprintf("Automatically deleted %d permission set assignment(s) before deleting the permission set. This may affect other Terraform resources if they manage these assignments.",
					len(deletedIDs)),
			)

			// Wait for assignments to be fully deleted (backend processes asynchronously)
			// Poll for up to 30 seconds to verify assignments are gone
			maxWaitTime := 30 * time.Second
			pollInterval := 2 * time.Second
			startTime := time.Now()

			for time.Since(startTime) < maxWaitTime {
				// Check if assignments still exist
				stillExists := false
				for _, deletedID := range deletedIDs {
					_, err := r.client.GetPermissionSetAssignment(deletedID)
					if err == nil {
						// Assignment still exists
						stillExists = true
						break
					}
					// 404 means it's gone, which is what we want
					if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
						// Some other error - log it but continue
						resp.Diagnostics.AddWarning(
							"Error Checking Assignment Status",
							fmt.Sprintf("Could not verify assignment %s was deleted: %s", deletedID, err),
						)
					}
				}

				if !stillExists {
					// All assignments are deleted
					break
				}

				// Wait before next poll
				time.Sleep(pollInterval)
			}

			// Final check - if assignments still exist after waiting, warn the user
			if time.Since(startTime) >= maxWaitTime {
				resp.Diagnostics.AddWarning(
					"Assignment Deletion Timeout",
					fmt.Sprintf("Waited %v for assignments to be deleted but they may still be processing. Permission set deletion may fail.", maxWaitTime),
				)
			}
		}
	}

	// Now delete the permission set
	err = r.client.DeletePermissionSet(permissionSetID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete permission set, got error: %s", err))
		return
	}
}

func (r *PermissionSetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
