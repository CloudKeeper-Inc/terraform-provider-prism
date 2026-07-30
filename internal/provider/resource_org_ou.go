package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &OrgOUResource{}
var _ resource.ResourceWithImportState = &OrgOUResource{}

func NewOrgOUResource() resource.Resource {
	return &OrgOUResource{}
}

type OrgOUResource struct {
	client *Client
}

type OrgOUResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	ParentID types.String `tfsdk:"parent_id"`
	Accounts types.Set    `tfsdk:"accounts"`
}

func (r *OrgOUResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_ou"
}

func (r *OrgOUResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an AWS Organizations organizational unit (OU) through CloudKeeper Prism. Optionally manages which accounts are placed in the OU.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the OU (e.g., `ou-xxxx-xxxxxxxx`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the organizational unit. Changing this forces a new resource to be created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"parent_id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The ID of the parent node. Use the root ID (e.g., `r-xxxx`) or another OU ID " +
					"(e.g., `ou-xxxx-xxxxxxxx`). You can find these IDs in the **Accounts > Hierarchy** view in Prism. " +
					"Changing this forces a new resource to be created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"accounts": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "AWS account IDs to place in this OU. When accounts are added, they are moved " +
					"into the OU. When removed, they are moved back to the parent. On destroy, all accounts are moved " +
					"back to the parent before the OU is deleted.",
			},
		},
	}
}

func (r *OrgOUResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *Client, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *OrgOUResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrgOUResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ou, err := r.client.CreateOU(data.ParentID.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create OU, got error: %s", err))
		return
	}

	data.ID = types.StringValue(ou.ID)

	// Move accounts into the OU if specified
	if !data.Accounts.IsNull() && !data.Accounts.IsUnknown() {
		var accountIDs []string
		resp.Diagnostics.Append(data.Accounts.ElementsAs(ctx, &accountIDs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for _, acctID := range accountIDs {
			sourceParent := r.findAccountParent(acctID)
			if sourceParent == "" {
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Cannot find current parent for account %s in org tree", acctID))
				return
			}
			if sourceParent != ou.ID {
				if err := r.client.MoveAccount(acctID, sourceParent, ou.ID); err != nil {
					resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to move account %s into OU: %s", acctID, err))
					return
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgOUResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrgOUResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgTree, err := r.client.GetOrgStructure()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read org structure: %s", err))
		return
	}

	ouNode := findNodeInTree(orgTree.Tree, data.ID.ValueString())
	if ouNode == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	data.Name = types.StringValue(ouNode.Name)

	// Find parent
	parent := findParentInTree(orgTree.Tree, data.ID.ValueString())
	if parent != nil {
		data.ParentID = types.StringValue(parent.ID)
	}

	// Read child accounts
	var childAccounts []string
	for _, child := range ouNode.Children {
		if child.Type == "ACCOUNT" {
			childAccounts = append(childAccounts, child.ID)
		}
	}

	if len(childAccounts) > 0 {
		acctSet, diags := types.SetValueFrom(ctx, types.StringType, childAccounts)
		resp.Diagnostics.Append(diags...)
		data.Accounts = acctSet
	} else if !data.Accounts.IsNull() {
		// User specified accounts but OU has none — set empty
		data.Accounts = types.SetValueMust(types.StringType, []attr.Value{})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgOUResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state OrgOUResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ouID := state.ID.ValueString()
	parentID := state.ParentID.ValueString()

	// Diff accounts
	var desiredAccounts, currentAccounts []string
	if !plan.Accounts.IsNull() && !plan.Accounts.IsUnknown() {
		resp.Diagnostics.Append(plan.Accounts.ElementsAs(ctx, &desiredAccounts, false)...)
	}
	if !state.Accounts.IsNull() && !state.Accounts.IsUnknown() {
		resp.Diagnostics.Append(state.Accounts.ElementsAs(ctx, &currentAccounts, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	currentSet := make(map[string]bool, len(currentAccounts))
	for _, a := range currentAccounts {
		currentSet[a] = true
	}
	desiredSet := make(map[string]bool, len(desiredAccounts))
	for _, a := range desiredAccounts {
		desiredSet[a] = true
	}

	// Move new accounts in
	for _, acctID := range desiredAccounts {
		if !currentSet[acctID] {
			sourceParent := r.findAccountParent(acctID)
			if sourceParent == "" {
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Cannot find current parent for account %s", acctID))
				return
			}
			if err := r.client.MoveAccount(acctID, sourceParent, ouID); err != nil {
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to move account %s into OU: %s", acctID, err))
				return
			}
		}
	}

	// Move removed accounts back to parent
	for _, acctID := range currentAccounts {
		if !desiredSet[acctID] {
			if err := r.client.MoveAccount(acctID, ouID, parentID); err != nil {
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to move account %s out of OU: %s", acctID, err))
				return
			}
		}
	}

	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgOUResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrgOUResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ouID := data.ID.ValueString()
	parentID := data.ParentID.ValueString()

	// Move all accounts back to parent before deleting
	var accountIDs []string
	if !data.Accounts.IsNull() && !data.Accounts.IsUnknown() {
		resp.Diagnostics.Append(data.Accounts.ElementsAs(ctx, &accountIDs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	for _, acctID := range accountIDs {
		if err := r.client.MoveAccount(acctID, ouID, parentID); err != nil {
			if !strings.Contains(err.Error(), "404") && !strings.Contains(strings.ToLower(err.Error()), "not found") {
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to move account %s out of OU before deletion: %s", acctID, err))
				return
			}
		}
	}

	if err := r.client.DeleteOU(ouID); err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete OU: %s", err))
		return
	}
}

func (r *OrgOUResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// findAccountParent looks up the current parent of an account in the org tree.
func (r *OrgOUResource) findAccountParent(accountID string) string {
	orgTree, err := r.client.GetOrgStructure()
	if err != nil {
		return ""
	}
	parent := findParentInTree(orgTree.Tree, accountID)
	if parent != nil {
		return parent.ID
	}
	return ""
}
