package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// crossAccountRoleName is the role Prism assumes in a target account to create
// the SAML/OIDC providers. It is fixed deployment-wide configuration; it appears
// here only so the preflight can name it in its remediation message.
const crossAccountRoleName = "CloudKeeper-SSO-Role"

var _ resource.Resource = &AWSAccountResource{}
var _ resource.ResourceWithImportState = &AWSAccountResource{}
var _ resource.ResourceWithValidateConfig = &AWSAccountResource{}

func NewAWSAccountResource() resource.Resource {
	return &AWSAccountResource{}
}

type AWSAccountResource struct {
	client *Client
}

type AWSAccountResourceModel struct {
	ID            types.String `tfsdk:"id"`
	CreateNew     types.Bool   `tfsdk:"create_new"`
	AccountID     types.String `tfsdk:"account_id"`
	AccountName   types.String `tfsdk:"account_name"`
	RootEmail     types.String `tfsdk:"root_email"`
	AdminRoleName types.String `tfsdk:"admin_role_name"`
	MasterAccount types.String `tfsdk:"master_account"`
	OwnerEmails   types.List   `tfsdk:"owner_emails"`
	CreateTimeout types.String `tfsdk:"create_timeout"`
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
			"create_new": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "When true, Prism creates a brand-new AWS account via the Account Factory instead of onboarding an existing one. Requires root_email, admin_role_name and master_account, and management-account/Org-Services must be enabled for the customer.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The AWS account ID (12-digit). Required in onboard mode; computed by the factory in create mode.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A friendly name for the AWS account",
			},
			"root_email": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Create mode only: the root-user email for the new AWS account. Required when create_new = true.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"admin_role_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Create mode only: name of an IAM role created in the new account with AdministratorAccess, assumable from master_account. Required when create_new = true.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"master_account": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Create mode only: 12-digit account whose root the admin role trusts. Required when create_new = true.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"owner_emails": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of owner email addresses for JIT (Just-In-Time) access approvals",
			},
			"create_timeout": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("45m"),
				MarkdownDescription: "Create mode only: maximum time to wait for the factory + onboarding to finish (Go duration, e.g. \"45m\").",
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

	if data.CreateNew.ValueBool() {
		r.createViaFactory(ctx, &data, resp)
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
		OwnerEmails: ownerEmails,
	}

	// Preflight: prove Prism can assume its cross-account role before attempting
	// the onboard itself, so a target account that cannot be onboarded is never
	// half-processed server-side. Onboard path only -- createViaFactory returned
	// above, and the factory provisions this role as part of creating the
	// account, so probing there would be a guaranteed-pass extra call.
	if err := r.client.PreflightAWSAccount(account.AccountID, account.AccountName); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Cannot assume %s in AWS account %s", crossAccountRoleName, account.AccountID),
			fmt.Sprintf("Prism could not assume its cross-account role in %s, so onboarding was not attempted and nothing was changed.\n\n"+
				"AWS reported: %s\n\n"+
				"The role is %s. It is deployment-wide configuration rather than something this resource sets, so the fix is in the target account, not in Terraform. "+
				"The usual causes are that the role is absent from %s -- a StackSet that provisioned it may have been removed, which is what happens when an account leaves its previous AWS organization -- "+
				"or that its trust policy no longer matches the Prism principal or its ExternalId.",
				account.AccountID, err, crossAccountRoleName, account.AccountID),
		)
		return
	}

	created, err := r.client.CreateAWSAccount(account)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create AWS account, got error: %s", err))
		return
	}

	// Guard the one attribute Terraform can never recover from. CreateAWSAccount
	// already rejects a response with no account record, so this only fires if a
	// future code path stops doing that -- an empty id in state means a resource
	// Terraform believes it owns but that does not exist in Prism, and every
	// later plan/refresh/destroy for it is wrong.
	if created.ID == "" {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Prism returned no account record id for AWS account %s; the account was not onboarded.", data.AccountID.ValueString()))
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

// createViaFactory drives the async Account Factory: launch, poll to completion,
// then poll until Prism onboarding lands, all within create_timeout (default 45m).
func (r *AWSAccountResource) createViaFactory(ctx context.Context, data *AWSAccountResourceModel, resp *resource.CreateResponse) {
	var ownerEmails []string
	if !data.OwnerEmails.IsNull() && !data.OwnerEmails.IsUnknown() {
		resp.Diagnostics.Append(data.OwnerEmails.ElementsAs(ctx, &ownerEmails, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	timeout := 45 * time.Minute
	if !data.CreateTimeout.IsNull() && data.CreateTimeout.ValueString() != "" {
		d, err := time.ParseDuration(data.CreateTimeout.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("create_timeout"), "Invalid create_timeout", err.Error())
			return
		}
		timeout = d
	}
	deadline := time.Now().Add(timeout)

	opID, err := r.client.CreateAccountViaFactory(FactoryCreateInput{
		AccountName:       data.AccountName.ValueString(),
		RootEmail:         data.RootEmail.ValueString(),
		OwnerEmails:       ownerEmails,
		AdminRoleName:     data.AdminRoleName.ValueString(),
		AdminTrustAccount: data.MasterAccount.ValueString(),
	})
	if err != nil {
		if errors.Is(err, ErrFeatureNotEnabled) {
			resp.Diagnostics.AddError("Account creation not enabled",
				"Account creation is not enabled for this customer: no management account is configured. "+
					"Enable Org Services and configure a management account before using create_new = true.")
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to launch account factory: %s", err))
		return
	}

	// Phase A: factory pipeline -> completed (captures the new account id).
	accountID := ""
	for {
		st, sErr := r.client.GetFactoryStatus(opID)
		if sErr == nil {
			if st.Status == "completed" && st.AWSAccountID != "" {
				accountID = st.AWSAccountID
				break
			}
			if st.Status == "failed" {
				reason := strings.TrimSpace(st.FailureReason + " " + st.ErrorMessage)
				if reason == "" {
					reason = "unknown reason"
				}
				resp.Diagnostics.AddError("Account creation failed", fmt.Sprintf("Account factory failed (operation %s): %s", opID, reason))
				return
			}
		}
		if time.Now().After(deadline) {
			resp.Diagnostics.AddError("Account creation timed out",
				fmt.Sprintf("Account factory did not complete within %s (operation %s). It may still finish server-side; re-apply to resume.", timeout, opID))
			return
		}
		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError("Account creation canceled", ctx.Err().Error())
			return
		case <-time.After(10 * time.Second):
		}
	}

	// Phase B: wait for Prism onboarding (GET /aws-accounts/{id} succeeds).
	for {
		acct, aErr := r.client.GetAWSAccount(accountID)
		if aErr == nil && acct != nil {
			data.ID = types.StringValue(accountID)
			data.AccountID = types.StringValue(accountID)
			if acct.AccountName != "" {
				data.AccountName = types.StringValue(acct.AccountName)
			}
			if len(acct.OwnerEmails) > 0 {
				l, d := types.ListValueFrom(ctx, types.StringType, acct.OwnerEmails)
				resp.Diagnostics.Append(d...)
				if resp.Diagnostics.HasError() {
					return
				}
				data.OwnerEmails = l
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
			return
		}
		if time.Now().After(deadline) {
			resp.Diagnostics.AddError("Onboarding timed out",
				fmt.Sprintf("Account %s was created but did not finish Prism onboarding within %s. Recover by setting create_new = false and account_id = %q on the next apply.", accountID, timeout, accountID))
			return
		}
		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError("Account creation canceled", ctx.Err().Error())
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (r *AWSAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AWSAccountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	account, err := r.client.GetAWSAccount(data.AccountID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read AWS account, got error: %s", err))
		return
	}

	// Only update account_name if API returned a non-empty value, otherwise preserve state value
	if account.AccountName != "" {
		data.AccountName = types.StringValue(account.AccountName)
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

func (r *AWSAccountResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data AWSAccountResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Interpolated (unknown) values can't be validated at plan-config time — a
	// value referencing another resource is unknown until apply. Skip validation
	// when create_new or any mode-relevant attribute is unknown; the apply-time
	// logic and the backend still enforce correctness.
	if data.CreateNew.IsUnknown() || data.AccountID.IsUnknown() ||
		data.RootEmail.IsUnknown() || data.AdminRoleName.IsUnknown() ||
		data.MasterAccount.IsUnknown() {
		return
	}
	for _, is := range validateAccountMode(data) {
		resp.Diagnostics.AddAttributeError(path.Root(is.Attr), is.Summary, is.Detail)
	}
}

func (r *AWSAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import using account_id (AWS account ID) since that's what Read() uses to fetch the account
	resource.ImportStatePassthroughID(ctx, path.Root("account_id"), req, resp)
}
