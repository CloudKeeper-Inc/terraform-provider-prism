# List all SCPs attached to a specific OU
data "prism_org_policies_for_target" "ou_scps" {
  management_account_id = "123456789012"
  target_id             = "ou-abc1-exampleou"
  policy_type           = "SERVICE_CONTROL_POLICY"
}

# List all tag policies attached to a specific account
data "prism_org_policies_for_target" "account_tags" {
  management_account_id = "123456789012"
  target_id             = "987654321098"
  policy_type           = "TAG_POLICY"
}

output "ou_scp_count" {
  value = length(data.prism_org_policies_for_target.ou_scps.policies)
}
