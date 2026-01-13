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

var _ resource.Resource = &AWSAccountResource{}
var _ resource.ResourceWithImportState = &AWSAccountResource{}

func NewAWSAccountResource() resource.Resource {
	return &AWSAccountResource{}
}

type AWSAccountResource struct {
	client *Client
}

type AWSAccountResourceModel struct {
	ID          types.String `tfsdk:"id"`
	AccountID   types.String `tfsdk:"account_id"`
	AccountName types.String `tfsdk:"account_name"`
	Region      types.String `tfsdk:"region"`
	RoleArn     types.String `tfsdk:"role_arn"`
	OwnerEmails types.List   `tfsdk:"owner_emails"`
}

func (r *AWSAccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_account"
}

func (r *AWSAccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an AWS account onboarded to CloudKeeper. This resource sets up SAML/OIDC providers and configures cross-account access.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The internal identifier for this AWS account configuration",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The AWS account ID (12-digit number)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A friendly name for the AWS account",
			},
			"region": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The primary AWS region for this account",
			},
			"role_arn": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The ARN of the IAM role used for cross-account access",
			},
			"owner_emails": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of owner email addresses for JIT (Just-In-Time) access approvals",
			},
		},
	}
}

func (r *AWSAccountResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AWSAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AWSAccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert owner_emails from types.List to []string
	var ownerEmails []string
	if !data.OwnerEmails.IsNull() && !data.OwnerEmails.IsUnknown() {
		diags := data.OwnerEmails.ElementsAs(ctx, &ownerEmails, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	account := &AWSAccount{
		AccountID:   data.AccountID.ValueString(),
		AccountName: data.AccountName.ValueString(),
		Region:      data.Region.ValueString(),
		RoleArn:     data.RoleArn.ValueString(),
		OwnerEmails: ownerEmails,
	}

	created, err := r.client.CreateAWSAccount(account)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create AWS account, got error: %s", err))
		return
	}

	// Set ID from API response
	data.ID = types.StringValue(created.ID)

	// Only update account_id if API returned a non-empty value, otherwise preserve plan value
	if created.AccountID != "" {
		data.AccountID = types.StringValue(created.AccountID)
	}

	// Only update account_name if API returned a non-empty value, otherwise preserve plan value
	if created.AccountName != "" {
		data.AccountName = types.StringValue(created.AccountName)
	}

	// Only update region if API returned a non-empty value
	if created.Region != "" {
		data.Region = types.StringValue(created.Region)
	}

	// Set role_arn: use API value if provided, otherwise compute default
	if created.RoleArn != "" {
		data.RoleArn = types.StringValue(created.RoleArn)
	} else if data.RoleArn.IsNull() || data.RoleArn.ValueString() == "" {
		// Compute default role ARN if not provided
		defaultRoleArn := fmt.Sprintf("arn:aws:iam::%s:role/CloudKeeper-SSO-Role", data.AccountID.ValueString())
		data.RoleArn = types.StringValue(defaultRoleArn)
	}

	// Set owner_emails from API response
	if len(created.OwnerEmails) > 0 {
		ownerEmailsList, diags := types.ListValueFrom(ctx, types.StringType, created.OwnerEmails)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.OwnerEmails = ownerEmailsList
	} else {
		data.OwnerEmails = types.ListNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AWSAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AWSAccountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	account, err := r.client.GetAWSAccount(data.AccountID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read AWS account, got error: %s", err))
		return
	}

	// Only update account_name if API returned a non-empty value, otherwise preserve state value
	if account.AccountName != "" {
		data.AccountName = types.StringValue(account.AccountName)
	}

	// Only update region if API returned a non-empty value
	if account.Region != "" {
		data.Region = types.StringValue(account.Region)
	}

	// Set role_arn: use API value if provided, otherwise compute default
	if account.RoleArn != "" {
		data.RoleArn = types.StringValue(account.RoleArn)
	} else if data.RoleArn.IsNull() || data.RoleArn.ValueString() == "" {
		// Compute default role ARN
		defaultRoleArn := fmt.Sprintf("arn:aws:iam::%s:role/CloudKeeper-SSO-Role", data.AccountID.ValueString())
		data.RoleArn = types.StringValue(defaultRoleArn)
	}

	// Set owner_emails from API response
	if len(account.OwnerEmails) > 0 {
		ownerEmailsList, diags := types.ListValueFrom(ctx, types.StringType, account.OwnerEmails)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.OwnerEmails = ownerEmailsList
	} else {
		data.OwnerEmails = types.ListNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AWSAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AWSAccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert owner_emails from types.List to []string
	var ownerEmails []string
	if !data.OwnerEmails.IsNull() && !data.OwnerEmails.IsUnknown() {
		diags := data.OwnerEmails.ElementsAs(ctx, &ownerEmails, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	account := &AWSAccount{
		AccountID:   data.AccountID.ValueString(),
		AccountName: data.AccountName.ValueString(),
		Region:      data.Region.ValueString(),
		RoleArn:     data.RoleArn.ValueString(),
		OwnerEmails: ownerEmails,
	}

	updated, err := r.client.UpdateAWSAccount(data.AccountID.ValueString(), account)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update AWS account, got error: %s", err))
		return
	}

	// Only update account_name if API returned a non-empty value, otherwise preserve plan value
	if updated.AccountName != "" {
		data.AccountName = types.StringValue(updated.AccountName)
	}

	// Only update region if API returned a non-empty value
	if updated.Region != "" {
		data.Region = types.StringValue(updated.Region)
	}

	// Set role_arn: use API value if provided, otherwise compute default
	if updated.RoleArn != "" {
		data.RoleArn = types.StringValue(updated.RoleArn)
	} else if data.RoleArn.IsNull() || data.RoleArn.ValueString() == "" {
		// Compute default role ARN
		defaultRoleArn := fmt.Sprintf("arn:aws:iam::%s:role/CloudKeeper-SSO-Role", data.AccountID.ValueString())
		data.RoleArn = types.StringValue(defaultRoleArn)
	}

	// Set owner_emails from API response
	if len(updated.OwnerEmails) > 0 {
		ownerEmailsList, diags := types.ListValueFrom(ctx, types.StringType, updated.OwnerEmails)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.OwnerEmails = ownerEmailsList
	} else {
		data.OwnerEmails = types.ListNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AWSAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AWSAccountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	accountID := data.AccountID.ValueString()

	// Before deleting the account, we need to delete all permission set assignments
	// that reference this account. This handles cases where Terraform's dependency
	// graph doesn't capture the relationship (e.g., hardcoded account IDs)
	assignments, err := r.client.ListPermissionSetAssignments()
	if err != nil {
		// Log warning but continue - if we can't list assignments, try to delete anyway
		resp.Diagnostics.AddWarning(
			"Unable to List Assignments",
			fmt.Sprintf("Could not list permission set assignments before deleting account. If assignments exist, account deletion may fail: %s", err),
		)
	} else {
		// Find and delete all assignments for this account
		var deleteErrors []string
		var deletedIDs []string

		for _, assignment := range assignments {
			if assignment.AccountID == accountID {
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
				fmt.Sprintf("Could not delete all permission set assignments for account %s. Account deletion may fail. Errors: %s",
					accountID, strings.Join(deleteErrors, "; ")),
			)
		}

		if len(deletedIDs) > 0 {
			resp.Diagnostics.AddWarning(
				"Automatic Assignment Cleanup",
				fmt.Sprintf("Automatically deleted %d permission set assignment(s) for account %s before deleting the account. This may affect other Terraform resources if they manage these assignments.",
					len(deletedIDs), accountID),
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
					fmt.Sprintf("Waited %v for assignments to be deleted but they may still be processing. Account deletion may fail.", maxWaitTime),
				)
			}
		}
	}

	// Now delete the account
	err = r.client.DeleteAWSAccount(accountID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete AWS account, got error: %s", err))
		return
	}
}

func (r *AWSAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import using account_id (AWS account ID) since that's what Read() uses to fetch the account
	resource.ImportStatePassthroughID(ctx, path.Root("account_id"), req, resp)
}
