package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &CustomerDataSource{}

func NewCustomerDataSource() datasource.DataSource {
	return &CustomerDataSource{}
}

type CustomerDataSource struct {
	client *Client
}

type CustomerDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Domain      types.String `tfsdk:"domain"`
}

func (d *CustomerDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer"
}

func (d *CustomerDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches information about a CloudKeeper customer.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The unique identifier for the customer",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the customer",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A description of the customer",
			},
			"domain": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The domain associated with the customer",
			},
		},
	}
}

func (d *CustomerDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *CustomerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CustomerDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customer, err := d.client.GetCustomer(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read customer, got error: %s", err))
		return
	}

	data.Name = types.StringValue(customer.Name)
	data.Description = types.StringValue(customer.Description)
	data.Domain = types.StringValue(customer.Domain)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
