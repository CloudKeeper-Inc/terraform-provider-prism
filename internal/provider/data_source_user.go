package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &UserDataSource{}

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

type UserDataSource struct {
	client *Client
}

type UserDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Username   types.String `tfsdk:"username"`
	Email      types.String `tfsdk:"email"`
	FirstName  types.String `tfsdk:"first_name"`
	LastName   types.String `tfsdk:"last_name"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	Attributes types.Map    `tfsdk:"attributes"`
}

func (d *UserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *UserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches information about a CloudKeeper user.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The unique identifier for the user",
			},
			"username": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The username for the user",
			},
			"email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email address of the user",
			},
			"first_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The first name of the user",
			},
			"last_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The last name of the user",
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the user account is enabled",
			},
			"attributes": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Custom attributes for the user",
			},
		},
	}
}

func (d *UserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data UserDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := d.client.GetUser(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read user, got error: %s", err))
		return
	}

	data.Username = types.StringValue(user.Username)
	data.Email = types.StringValue(user.Email)
	if user.FirstName != "" {
		data.FirstName = types.StringValue(user.FirstName)
	}
	if user.LastName != "" {
		data.LastName = types.StringValue(user.LastName)
	}
	data.Enabled = types.BoolValue(user.Enabled)

	if len(user.Attributes) > 0 {
		// Convert map[string][]string from API to map[string]string for Terraform
		tfAttributesMap := make(map[string]string)
		for k, v := range user.Attributes {
			if len(v) > 0 {
				tfAttributesMap[k] = v[0] // Take first value
			}
		}
		attributesMap, diags := types.MapValueFrom(ctx, types.StringType, tfAttributesMap)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Attributes = attributesMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
