# Read a single org policy by ID
data "prism_org_policy" "example" {
  id                    = "p-examplepolicyid"
}

output "policy_name" {
  value = data.prism_org_policy.example.name
}

output "policy_content" {
  value = data.prism_org_policy.example.content
}
