# Onboard an EXISTING AWS account.
resource "prism_aws_account" "production" {
  account_id   = "123456789012"
  account_name = "Production"
}

# Create a BRAND-NEW AWS account via the Account Factory.
# Requires the customer to have Org Services + a management account configured;
# otherwise apply fails fast with an "Account creation not enabled" error.
resource "prism_aws_account" "new_prod" {
  create_new      = true
  account_name    = "Acme Prod"
  root_email      = "aws-acme-prod@example.com"
  admin_role_name = "OrgAdminAccess"
  master_account  = "857639285487"
  owner_emails    = ["ops@example.com"]
  create_timeout  = "45m"
}
