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

var _ resource.Resource = &OrgPolicyAttachmentResource{}
var _ resource.ResourceWithImportState = &OrgPolicyAttachmentResource{}

func NewOrgPolicyAttachmentResource() resource.Resource {
	return &OrgPolicyAttachmentResource{}
}

type OrgPolicyAttachmentResource struct {
	client *Client
}

type OrgPolicyAttachmentResourceModel struct {
	ID       types.String `tfsdk:"id"`
	PolicyID types.String `tfsdk:"policy_id"`
	TargetID types.String `tfsdk:"target_id"`
}

func (r *OrgPolicyAttachmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_policy_attachment"
}

func (r *OrgPolicyAttachmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Attaches an AWS Organizations policy to a target (root, organizational unit, or account) through CloudKeeper Prism.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier for the attachment (format: policy_id/target_id)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"policy_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the policy to attach. Changing this forces a new resource to be created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the root, organizational unit, or account to attach the policy to. Changing this forces a new resource to be created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *OrgPolicyAttachmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrgPolicyAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrgPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policyID := data.PolicyID.ValueString()
	targetID := data.TargetID.ValueString()

	// Wait for the policy to exist before attaching
	if err := waitForDependency(ctx, "org_policy", policyID, func() error {
		_, err := r.client.GetOrgPolicy("", policyID)
		return err
	}); err != nil {
		resp.Diagnostics.AddError("Dependency Error", fmt.Sprintf("Org policy dependency not satisfied: %s", err))
		return
	}

	err := r.client.AttachOrgPolicy("", policyID, targetID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to attach org policy, got error: %s", err))
		return
	}

	data.ID = types.StringValue(policyID + "/" + targetID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgPolicyAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrgPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policyID := data.PolicyID.ValueString()
	targetID := data.TargetID.ValueString()

	targets, err := r.client.ListOrgPolicyTargets("", policyID)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read org policy targets, got error: %s", err))
		return
	}

	found := false
	for _, t := range targets {
		if t.TargetID == targetID {
			found = true
			break
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgPolicyAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Unexpected Update",
		"All attributes of org_policy_attachment require replacement. This is a provider bug.",
	)
}

func (r *OrgPolicyAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrgPolicyAttachmentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DetachOrgPolicy("", data.PolicyID.ValueString(), data.TargetID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to detach org policy, got error: %s", err))
		return
	}
}

func (r *OrgPolicyAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			"Expected import ID format: policy_id/target_id",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("policy_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
