package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &OrgPolicyResource{}
var _ resource.ResourceWithImportState = &OrgPolicyResource{}

func NewOrgPolicyResource() resource.Resource {
	return &OrgPolicyResource{}
}

type OrgPolicyResource struct {
	client *Client
}

type OrgPolicyResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Content     types.String `tfsdk:"content"`
	PolicyType  types.String `tfsdk:"policy_type"`
	AWSManaged  types.Bool   `tfsdk:"aws_managed"`
	Tags        types.Map    `tfsdk:"tags"`
}

// normalizeJSON re-marshals a JSON string through map[string]interface{} so that
// keys are sorted alphabetically, matching the output of Terraform's jsonencode().
func normalizeJSON(input string) string {
	var parsed interface{}
	if err := json.Unmarshal([]byte(input), &parsed); err != nil {
		return input
	}
	normalized, err := json.Marshal(parsed)
	if err != nil {
		return input
	}
	return string(normalized)
}

func (r *OrgPolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_policy"
}

func (r *OrgPolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an AWS Organizations policy (SCP, tag policy, backup policy, etc.) through CloudKeeper Prism.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the policy",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The friendly name of the policy",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description of the policy",
			},
			"content": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The policy content as a JSON string",
			},
			"policy_type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("SERVICE_CONTROL_POLICY"),
				MarkdownDescription: "The type of policy. Valid values: `SERVICE_CONTROL_POLICY`, `TAG_POLICY`, `BACKUP_POLICY`, `AISERVICES_OPT_OUT_POLICY`, `RESOURCE_CONTROL_POLICY`. Defaults to `SERVICE_CONTROL_POLICY`. Changing this forces a new resource to be created.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						"SERVICE_CONTROL_POLICY",
						"TAG_POLICY",
						"BACKUP_POLICY",
						"AISERVICES_OPT_OUT_POLICY",
						"RESOURCE_CONTROL_POLICY",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"aws_managed": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the policy is an AWS-managed policy",
			},
			"tags": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Key-value tags to attach to the policy",
			},
		},
	}
}

func (r *OrgPolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrgPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrgPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy := &OrgPolicy{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Content:     data.Content.ValueString(),
		Type:        data.PolicyType.ValueString(),
	}

	created, err := r.client.CreateOrgPolicy("", policy)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create org policy, got error: %s", err))
		return
	}

	// Create response doesn't include content, fetch full details
	detail, err := r.client.GetOrgPolicy("", created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read org policy after creation, got error: %s", err))
		return
	}

	data.ID = types.StringValue(detail.ID)
	data.Name = types.StringValue(detail.Name)
	data.Description = types.StringValue(detail.Description)
	data.Content = types.StringValue(normalizeJSON(detail.Content))
	data.PolicyType = types.StringValue(detail.Type)
	data.AWSManaged = types.BoolValue(detail.AWSManaged)

	// Apply tags if any
	if !data.Tags.IsNull() && !data.Tags.IsUnknown() {
		tags := make(map[string]string)
		resp.Diagnostics.Append(data.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(tags) > 0 {
			var tagList []OrgPolicyTag
			for k, v := range tags {
				tagList = append(tagList, OrgPolicyTag{Key: k, Value: v})
			}
			if err := r.client.TagOrgPolicy("", detail.ID, tagList); err != nil {
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to tag org policy, got error: %s", err))
				return
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrgPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	detail, err := r.client.GetOrgPolicy("", data.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read org policy, got error: %s", err))
		return
	}

	data.Name = types.StringValue(detail.Name)
	data.Description = types.StringValue(detail.Description)
	data.Content = types.StringValue(normalizeJSON(detail.Content))
	data.PolicyType = types.StringValue(detail.Type)
	data.AWSManaged = types.BoolValue(detail.AWSManaged)

	// Fetch tags
	tags, err := r.client.ListTagsForOrgPolicy("", data.ID.ValueString())
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

func (r *OrgPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrgPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy := &OrgPolicy{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Content:     data.Content.ValueString(),
	}

	updated, err := r.client.UpdateOrgPolicy("", data.ID.ValueString(), policy)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update org policy, got error: %s", err))
		return
	}

	data.Name = types.StringValue(updated.Name)
	data.Description = types.StringValue(updated.Description)
	data.Content = types.StringValue(normalizeJSON(updated.Content))
	data.PolicyType = types.StringValue(updated.Type)
	data.AWSManaged = types.BoolValue(updated.AWSManaged)

	// Handle tag changes
	var oldData OrgPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &oldData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desiredTags := make(map[string]string)
	if !data.Tags.IsNull() && !data.Tags.IsUnknown() {
		resp.Diagnostics.Append(data.Tags.ElementsAs(ctx, &desiredTags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	oldTags := make(map[string]string)
	if !oldData.Tags.IsNull() && !oldData.Tags.IsUnknown() {
		resp.Diagnostics.Append(oldData.Tags.ElementsAs(ctx, &oldTags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	mgmtID := ""
	policyID := data.ID.ValueString()

	// Remove deleted tags
	var keysToRemove []string
	for k := range oldTags {
		if _, exists := desiredTags[k]; !exists {
			keysToRemove = append(keysToRemove, k)
		}
	}
	if len(keysToRemove) > 0 {
		if err := r.client.UntagOrgPolicy(mgmtID, policyID, keysToRemove); err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to remove tags from org policy, got error: %s", err))
			return
		}
	}

	// Add/update tags
	var tagsToSet []OrgPolicyTag
	for k, v := range desiredTags {
		if oldVal, exists := oldTags[k]; !exists || oldVal != v {
			tagsToSet = append(tagsToSet, OrgPolicyTag{Key: k, Value: v})
		}
	}
	if len(tagsToSet) > 0 {
		if err := r.client.TagOrgPolicy(mgmtID, policyID, tagsToSet); err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to tag org policy, got error: %s", err))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrgPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteOrgPolicy("", data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete org policy, got error: %s", err))
		return
	}
}

func (r *OrgPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
