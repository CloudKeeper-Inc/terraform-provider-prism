package provider

import (
	"context"
	"fmt"

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

	account := &AWSAccount{
		AccountID:   data.AccountID.ValueString(),
		AccountName: data.AccountName.ValueString(),
		Region:      data.Region.ValueString(),
		RoleArn:     data.RoleArn.ValueString(),
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AWSAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AWSAccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	account := &AWSAccount{
		AccountID:   data.AccountID.ValueString(),
		AccountName: data.AccountName.ValueString(),
		Region:      data.Region.ValueString(),
		RoleArn:     data.RoleArn.ValueString(),
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AWSAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AWSAccountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteAWSAccount(data.AccountID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete AWS account, got error: %s", err))
		return
	}
}

func (r *AWSAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
