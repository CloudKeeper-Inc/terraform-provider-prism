# List all SCPs in the organization
data "prism_org_policies" "all_scps" {
  management_account_id = "123456789012"
  policy_type           = "SERVICE_CONTROL_POLICY"
}

# List all tag policies
data "prism_org_policies" "all_tag_policies" {
  management_account_id = "123456789012"
  policy_type           = "TAG_POLICY"
}

output "scp_count" {
  value = length(data.prism_org_policies.all_scps.policies)
}
