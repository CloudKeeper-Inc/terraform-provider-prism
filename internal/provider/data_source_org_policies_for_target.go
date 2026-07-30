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

var _ datasource.DataSource = &OrgPoliciesForTargetDataSource{}

func NewOrgPoliciesForTargetDataSource() datasource.DataSource {
	return &OrgPoliciesForTargetDataSource{}
}

type OrgPoliciesForTargetDataSource struct {
	client *Client
}

type OrgPoliciesForTargetDataSourceModel struct {
	TargetID   types.String            `tfsdk:"target_id"`
	PolicyType types.String            `tfsdk:"policy_type"`
	Policies   []OrgPolicySummaryModel `tfsdk:"policies"`
}

func (d *OrgPoliciesForTargetDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_policies_for_target"
}

func (d *OrgPoliciesForTargetDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists AWS Organizations policies attached to a specific target (root, OU, or account) through CloudKeeper Prism.",

		Attributes: map[string]schema.Attribute{
			"target_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the root, organizational unit, or account to list policies for",
			},
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
				MarkdownDescription: "List of policies attached to the target",
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

func (d *OrgPoliciesForTargetDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *OrgPoliciesForTargetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrgPoliciesForTargetDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policyType := data.PolicyType.ValueString()

	policies, err := d.client.ListOrgPoliciesForTarget(
		"",
		data.TargetID.ValueString(),
		policyType,
	)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list org policies for target, got error: %s", err))
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
