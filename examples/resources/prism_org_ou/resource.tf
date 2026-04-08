# Create an OU under the organization root
resource "prism_org_ou" "production" {
  name      = "Production"
  parent_id = "ou-xxxx-xxxxxxxx" # Root or parent OU ID (visible in Accounts > Hierarchy view)

  accounts = [
    "111111111111",
    "222222222222",
  ]
}

# Create a nested OU
resource "prism_org_ou" "prod_data_team" {
  name      = "Data-Team"
  parent_id = prism_org_ou.production.id

  accounts = [
    "333333333333",
  ]
}

# OU without accounts (just organizational structure)
resource "prism_org_ou" "staging" {
  name      = "Staging"
  parent_id = "ou-xxxx-xxxxxxxx"
}
