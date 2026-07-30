package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &OrgPoliciesDataSource{}

func NewOrgPoliciesDataSource() datasource.DataSource {
	return &OrgPoliciesDataSource{}
}

type OrgPoliciesDataSource struct {
	client *Client
}

type OrgPoliciesDataSourceModel struct {
	PolicyType types.String            `tfsdk:"policy_type"`
	Policies   []OrgPolicySummaryModel `tfsdk:"policies"`
}

type OrgPolicySummaryModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	AWSManaged  types.Bool   `tfsdk:"aws_managed"`
}

func (d *OrgPoliciesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_policies"
}

func (d *OrgPoliciesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists AWS Organizations policies for a management account through CloudKeeper Prism.",

		Attributes: map[string]schema.Attribute{
			"policy_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The type of policy to filter by. Defaults to SERVICE_CONTROL_POLICY if not specified. Valid values: `SERVICE_CONTROL_POLICY`, `TAG_POLICY`, `BACKUP_POLICY`, `AISERVICES_OPT_OUT_POLICY`, `RESOURCE_CONTROL_POLICY`.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						"SERVICE_CONTROL_POLICY",
						"TAG_POLICY",
						"BACKUP_POLICY",
						"AISERVICES_OPT_OUT_POLICY",
						"RESOURCE_CONTROL_POLICY",
					),
				},
			},
			"policies": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of policies",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The unique identifier of the policy",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The friendly name of the policy",
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "A description of the policy",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The type of policy",
						},
						"aws_managed": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the policy is an AWS-managed policy",
						},
					},
				},
			},
		},
	}
}

func (d *OrgPoliciesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *OrgPoliciesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrgPoliciesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policyType := data.PolicyType.ValueString()

	policies, err := d.client.ListOrgPolicies("", policyType)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list org policies, got error: %s", err))
		return
	}

	data.Policies = make([]OrgPolicySummaryModel, len(policies))
	for i, p := range policies {
		data.Policies[i] = OrgPolicySummaryModel{
			ID:          types.StringValue(p.ID),
			Name:        types.StringValue(p.Name),
			Description: types.StringValue(p.Description),
			Type:        types.StringValue(p.Type),
			AWSManaged:  types.BoolValue(p.AWSManaged),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
