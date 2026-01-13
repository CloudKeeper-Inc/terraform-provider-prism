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

var _ resource.Resource = &GroupMembershipResource{}
var _ resource.ResourceWithImportState = &GroupMembershipResource{}

func NewGroupMembershipResource() resource.Resource {
	return &GroupMembershipResource{}
}

type GroupMembershipResource struct {
	client *Client
}

type GroupMembershipResourceModel struct {
	ID        types.String `tfsdk:"id"`
	GroupName types.String `tfsdk:"group_name"`
	Usernames types.List   `tfsdk:"usernames"`
}

func (r *GroupMembershipResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_membership"
}

func (r *GroupMembershipResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages group membership for CloudKeeper users. This resource adds users to a group and removes them when destroyed.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The identifier for this group membership resource (group_name)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the group",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"usernames": schema.ListAttribute{
				ElementType:         types.StringType,
				Required:            true,
				MarkdownDescription: "List of usernames to add to the group",
			},
		},
	}
}

func (r *GroupMembershipResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data GroupMembershipResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var usernames []string
	resp.Diagnostics.Append(data.Usernames.ElementsAs(ctx, &usernames, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.AddGroupMembers(data.GroupName.ValueString(), usernames)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to add group members, got error: %s", err))
		return
	}

	data.ID = types.StringValue(data.GroupName.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GroupMembershipResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members, err := r.client.GetGroupMembers(data.GroupName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read group members, got error: %s", err))
		return
	}

	usernamesList, diags := types.ListValueFrom(ctx, types.StringType, members)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Usernames = usernamesList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupMembershipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state GroupMembershipResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var planUsernames, stateUsernames []string
	resp.Diagnostics.Append(plan.Usernames.ElementsAs(ctx, &planUsernames, false)...)
	resp.Diagnostics.Append(state.Usernames.ElementsAs(ctx, &stateUsernames, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Find users to add (in plan but not in state)
	toAdd := []string{}
	for _, planUsername := range planUsernames {
		found := false
		for _, stateUsername := range stateUsernames {
			if planUsername == stateUsername {
				found = true
				break
			}
		}
		if !found {
			toAdd = append(toAdd, planUsername)
		}
	}

	// Find users to remove (in state but not in plan)
	toRemove := []string{}
	for _, stateUsername := range stateUsernames {
		found := false
		for _, planUsername := range planUsernames {
			if stateUsername == planUsername {
				found = true
				break
			}
		}
		if !found {
			toRemove = append(toRemove, stateUsername)
		}
	}

	// Add new members
	if len(toAdd) > 0 {
		err := r.client.AddGroupMembers(plan.GroupName.ValueString(), toAdd)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to add group members, got error: %s", err))
			return
		}
	}

	// Remove old members
	if len(toRemove) > 0 {
		err := r.client.RemoveGroupMembers(plan.GroupName.ValueString(), toRemove)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to remove group members, got error: %s", err))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *GroupMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data GroupMembershipResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var usernames []string
	resp.Diagnostics.Append(data.Usernames.ElementsAs(ctx, &usernames, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RemoveGroupMembers(data.GroupName.ValueString(), usernames)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to remove group members, got error: %s", err))
		return
	}
}

func (r *GroupMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import using group_name since that's what Read() uses to fetch the members
	resource.ImportStatePassthroughID(ctx, path.Root("group_name"), req, resp)
}
