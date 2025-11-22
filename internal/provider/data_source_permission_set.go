package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PermissionSetDataSource{}

func NewPermissionSetDataSource() datasource.DataSource {
	return &PermissionSetDataSource{}
}

type PermissionSetDataSource struct {
	client *Client
}

type PermissionSetDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	SessionDuration types.String `tfsdk:"session_duration"`
	ManagedPolicies types.List   `tfsdk:"managed_policies"`
	InlinePolicies  types.Map    `tfsdk:"inline_policies"`
}

func (d *PermissionSetDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_set"
}

func (d *PermissionSetDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches information about a CloudKeeper permission set.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The unique identifier for the permission set",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the permission set",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A description of the permission set",
			},
			"session_duration": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The session duration in ISO 8601 format",
			},
			"managed_policies": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "List of AWS managed policy ARNs",
			},
			"inline_policies": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Map of inline IAM policy documents in JSON format. The key is the policy name, and the value is the policy document.",
			},
		},
	}
}

func (d *PermissionSetDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PermissionSetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PermissionSetDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	permSet, err := d.client.GetPermissionSet(data.ID.ValueString())
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
