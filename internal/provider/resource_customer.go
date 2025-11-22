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

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &CustomerResource{}
var _ resource.ResourceWithImportState = &CustomerResource{}

func NewCustomerResource() resource.Resource {
	return &CustomerResource{}
}

// CustomerResource defines the resource implementation.
type CustomerResource struct {
	client *Client
}

// CustomerResourceModel describes the resource data model.
type CustomerResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Domain      types.String `tfsdk:"domain"`
}

func (r *CustomerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer"
}

func (r *CustomerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a CloudKeeper customer (tenant). Each customer represents an isolated realm in Keycloak with its own users, groups, and configurations.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier for the customer",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the customer",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description of the customer",
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The domain associated with the customer",
			},
		},
	}
}

func (r *CustomerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CustomerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CustomerResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	customer := &Customer{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Domain:      data.Domain.ValueString(),
	}

	created, err := r.client.CreateCustomer(customer)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create customer, got error: %s", err))
		return
	}

	data.ID = types.StringValue(created.ID)
	data.Name = types.StringValue(created.Name)
	data.Description = types.StringValue(created.Description)
	data.Domain = types.StringValue(created.Domain)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CustomerResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	customer, err := r.client.GetCustomer(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read customer, got error: %s", err))
		return
	}

	data.Name = types.StringValue(customer.Name)
	data.Description = types.StringValue(customer.Description)
	data.Domain = types.StringValue(customer.Domain)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CustomerResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	customer := &Customer{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Domain:      data.Domain.ValueString(),
	}

	updated, err := r.client.UpdateCustomer(data.ID.ValueString(), customer)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update customer, got error: %s", err))
		return
	}

	data.Name = types.StringValue(updated.Name)
	data.Description = types.StringValue(updated.Description)
	data.Domain = types.StringValue(updated.Domain)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CustomerResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteCustomer(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete customer, got error: %s", err))
		return
	}
}

func (r *CustomerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
