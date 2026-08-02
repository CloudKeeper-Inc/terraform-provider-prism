package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &CustomerManagedPolicyResource{}
var _ resource.ResourceWithImportState = &CustomerManagedPolicyResource{}

func NewCustomerManagedPolicyResource() resource.Resource {
	return &CustomerManagedPolicyResource{}
}

type CustomerManagedPolicyResource struct {
	client *Client
}

type CustomerManagedPolicyResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	PolicyDocument types.String `tfsdk:"policy_document"`
}

func (r *CustomerManagedPolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer_managed_policy"
}

func (r *CustomerManagedPolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a CloudKeeper customer-managed policy (an IAM policy in the CMP library that can be attached to permission sets).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier for the customer-managed policy.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The policy name (unique within the tenant).",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description of the policy.",
			},
			"policy_document": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The IAM policy document as a JSON string. Keep it canonical — formatting-only differences show as a diff.",
			},
		},
	}
}

func (r *CustomerManagedPolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *Client, got: %T. Please report this issue to the provider developers.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *CustomerManagedPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CustomerManagedPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateCustomerManagedPolicy(&CustomerManagedPolicy{
		Name: data.Name.ValueString(), Description: data.Description.ValueString(), PolicyDocument: data.PolicyDocument.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create customer-managed policy: %s", err))
		return
	}
	data.ID = types.StringValue(created.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomerManagedPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CustomerManagedPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cmp, err := r.client.GetCustomerManagedPolicy(data.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read customer-managed policy: %s", err))
		return
	}
	data.Name = types.StringValue(cmp.Name)
	if cmp.Description != "" {
		data.Description = types.StringValue(cmp.Description)
	}
	if cmp.PolicyDocument != "" {
		data.PolicyDocument = types.StringValue(cmp.PolicyDocument)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomerManagedPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CustomerManagedPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateCustomerManagedPolicy(data.ID.ValueString(), &CustomerManagedPolicy{
		Name: data.Name.ValueString(), Description: data.Description.ValueString(), PolicyDocument: data.PolicyDocument.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update customer-managed policy: %s", err))
		return
	}
	data.Name = types.StringValue(updated.Name)
	if updated.Description != "" {
		data.Description = types.StringValue(updated.Description)
	}
	if updated.PolicyDocument != "" {
		data.PolicyDocument = types.StringValue(updated.PolicyDocument)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CustomerManagedPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CustomerManagedPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCustomerManagedPolicy(data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete customer-managed policy: %s", err))
		return
	}
}

func (r *CustomerManagedPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
