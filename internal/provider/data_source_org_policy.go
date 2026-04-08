package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &OrgPolicyDataSource{}

func NewOrgPolicyDataSource() datasource.DataSource {
	return &OrgPolicyDataSource{}
}

type OrgPolicyDataSource struct {
	client *Client
}

type OrgPolicyDataSourceModel struct {
	ID                  types.String `tfsdk:"id"`
	ManagementAccountID types.String `tfsdk:"management_account_id"`
	Name                types.String `tfsdk:"name"`
	Description         types.String `tfsdk:"description"`
	Content             types.String `tfsdk:"content"`
	PolicyType          types.String `tfsdk:"policy_type"`
	AWSManaged          types.Bool   `tfsdk:"aws_managed"`
	Tags                types.Map    `tfsdk:"tags"`
}

func (d *OrgPolicyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_policy"
}

func (d *OrgPolicyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches information about an AWS Organizations policy through CloudKeeper Prism.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the policy",
			},
			"management_account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The AWS management (payer) account ID that owns the organization",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The friendly name of the policy",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A description of the policy",
			},
			"content": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The policy content as a JSON string",
			},
			"policy_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of policy",
			},
			"aws_managed": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the policy is an AWS-managed policy",
			},
			"tags": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Key-value tags on the policy",
			},
		},
	}
}

func (d *OrgPolicyDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *OrgPolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrgPolicyDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	detail, err := d.client.GetOrgPolicy(data.ManagementAccountID.ValueString(), data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read org policy, got error: %s", err))
		return
	}

	data.Name = types.StringValue(detail.Name)
	data.Description = types.StringValue(detail.Description)
	data.Content = types.StringValue(detail.Content)
	data.PolicyType = types.StringValue(detail.Type)
	data.AWSManaged = types.BoolValue(detail.AWSManaged)

	tags, err := d.client.ListTagsForOrgPolicy(data.ManagementAccountID.ValueString(), data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read tags for org policy, got error: %s", err))
		return
	}
	if len(tags) > 0 {
		tagMap := make(map[string]string, len(tags))
		for _, t := range tags {
			tagMap[t.Key] = t.Value
		}
		tagsValue, diags := types.MapValueFrom(ctx, types.StringType, tagMap)
		resp.Diagnostics.Append(diags...)
		data.Tags = tagsValue
	} else {
		data.Tags = types.MapNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
