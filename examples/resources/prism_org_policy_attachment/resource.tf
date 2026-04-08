# Attach a policy to an Organizational Unit
resource "prism_org_policy_attachment" "scp_to_ou" {
  management_account_id = "123456789012"
  policy_id             = prism_org_policy.deny_s3.id
  target_id             = "ou-abc1-exampleou"
}

# Attach a policy to a specific AWS account
resource "prism_org_policy_attachment" "scp_to_account" {
  management_account_id = "123456789012"
  policy_id             = prism_org_policy.deny_s3.id
  target_id             = "987654321098"
}

# Attach a policy to the organization root
resource "prism_org_policy_attachment" "tag_to_root" {
  management_account_id = "123456789012"
  policy_id             = prism_org_policy.require_environment_tag.id
  target_id             = "r-exampleroot"
}
