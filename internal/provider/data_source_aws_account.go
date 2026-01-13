package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &AWSAccountDataSource{}

func NewAWSAccountDataSource() datasource.DataSource {
	return &AWSAccountDataSource{}
}

type AWSAccountDataSource struct {
	client *Client
}

type AWSAccountDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	AccountID   types.String `tfsdk:"account_id"`
	AccountName types.String `tfsdk:"account_name"`
	Region      types.String `tfsdk:"region"`
	RoleArn     types.String `tfsdk:"role_arn"`
	OwnerEmails types.List   `tfsdk:"owner_emails"`
}

func (d *AWSAccountDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_account"
}

func (d *AWSAccountDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches information about an AWS account onboarded to CloudKeeper.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The internal identifier for this AWS account configuration",
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The AWS account ID (12-digit number)",
			},
			"account_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A friendly name for the AWS account",
			},
			"region": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The primary AWS region for this account",
			},
			"role_arn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ARN of the IAM role used for cross-account access",
			},
			"owner_emails": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of owner email addresses for JIT (Just-In-Time) access approvals",
			},
		},
	}
}

func (d *AWSAccountDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *AWSAccountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AWSAccountDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	account, err := d.client.GetAWSAccount(data.AccountID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read AWS account, got error: %s", err))
		return
	}

	data.ID = types.StringValue(account.ID)
	data.AccountName = types.StringValue(account.AccountName)
	if account.Region != "" {
		data.Region = types.StringValue(account.Region)
	}
	if account.RoleArn != "" {
		data.RoleArn = types.StringValue(account.RoleArn)
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
