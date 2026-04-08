# List all SCPs in the organization
data "prism_org_policies" "all_scps" {
  policy_type           = "SERVICE_CONTROL_POLICY"
}

# List all tag policies
data "prism_org_policies" "all_tag_policies" {
  policy_type           = "TAG_POLICY"
}

output "scp_count" {
  value = length(data.prism_org_policies.all_scps.policies)
}
