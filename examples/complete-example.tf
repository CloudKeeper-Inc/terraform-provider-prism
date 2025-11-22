# Complete CloudKeeper Terraform Configuration Example

terraform {
  required_version = ">= 1.0"

  required_providers {
    cloudkeeper = {
      source = "cloudkeeper/cloudkeeper"
    }
  }
}

provider "cloudkeeper" {
  prism_subdomain = var.prism_subdomain
  api_token       = var.cloudkeeper_token
}

variable "cloudkeeper_token" {
  type        = string
  description = "CloudKeeper API token"
  sensitive   = true
}

variable "prism_subdomain" {
  type        = string
  description = "Prism subdomain. This can be found in the Prism URL of your tenant - https://{prism_subdomain}.prism.cloudkeeper.com"
  sensitive   = false
}

# ============= AWS Accounts =============

resource "cloudkeeper_aws_account" "production" {
  account_id   = "123456789012"
  account_name = "Production"
  region       = "us-east-1"
}

resource "cloudkeeper_aws_account" "development" {
  account_id   = "234567890123"
  account_name = "Development"
  region       = "us-east-1"
}

resource "cloudkeeper_aws_account" "staging" {
  account_id   = "345678901234"
  account_name = "Staging"
  region       = "us-east-1"
}

# ============= Permission Sets =============

# Admin Permission Set
resource "cloudkeeper_permission_set" "admin" {
  name             = "AdministratorAccess"
  description      = "Full administrator access"
  session_duration = "PT8H"

  managed_policies = [
    "arn:aws:iam::aws:policy/AdministratorAccess"
  ]
}

# Developer Permission Set
resource "cloudkeeper_permission_set" "developer" {
  name             = "DeveloperAccess"
  description      = "Developer access with deployment permissions"
  session_duration = "PT4H"

  managed_policies = [
    "arn:aws:iam::aws:policy/PowerUserAccess"
  ]

  inline_policies = {
    dev_resources = jsonencode({
      Version = "2012-10-17"
      Statement = [
        {
          Effect = "Allow"
          Action = [
            "s3:*",
            "lambda:*",
            "dynamodb:*"
          ]
          Resource = "*"
        }
      ]
    })
  }
}

# ReadOnly Permission Set
resource "cloudkeeper_permission_set" "readonly" {
  name             = "ReadOnlyAccess"
  description      = "Read-only access for auditors"
  session_duration = "PT2H"

  managed_policies = [
    "arn:aws:iam::aws:policy/ReadOnlyAccess"
  ]
}

# ============= Users =============

resource "cloudkeeper_user" "john_admin" {
  username   = "john.admin"
  email      = "john.admin@example.com"
  first_name = "John"
  last_name  = "Admin"
  enabled    = true

  attributes = {
    department = "IT Operations"
    location   = "US"
    role       = "Administrator"
  }
}

resource "cloudkeeper_user" "jane_dev" {
  username   = "jane.developer"
  email      = "jane.developer@example.com"
  first_name = "Jane"
  last_name  = "Developer"
  enabled    = true

  attributes = {
    department = "Engineering"
    location   = "US"
    role       = "Developer"
  }
}

resource "cloudkeeper_user" "bob_auditor" {
  username   = "bob.auditor"
  email      = "bob.auditor@example.com"
  first_name = "Bob"
  last_name  = "Auditor"
  enabled    = true

  attributes = {
    department = "Compliance"
    location   = "EU"
    role       = "Auditor"
  }
}

# ============= Groups =============

resource "cloudkeeper_group" "admins" {
  name        = "Administrators"
  description = "System administrators with full access"
  path        = "/teams/operations/"
}

resource "cloudkeeper_group" "developers" {
  name        = "Developers"
  description = "Development team members"
  path        = "/teams/engineering/"
}

resource "cloudkeeper_group" "auditors" {
  name        = "Auditors"
  description = "Security and compliance auditors"
  path        = "/teams/compliance/"
}

# ============= Group Memberships =============

resource "cloudkeeper_group_membership" "admin_members" {
  group_name = cloudkeeper_group.admins.name
  user_ids = [
    cloudkeeper_user.john_admin.id
  ]
}

resource "cloudkeeper_group_membership" "dev_members" {
  group_name = cloudkeeper_group.developers.name
  user_ids = [
    cloudkeeper_user.jane_dev.id
  ]
}

resource "cloudkeeper_group_membership" "auditor_members" {
  group_name = cloudkeeper_group.auditors.name
  user_ids = [
    cloudkeeper_user.bob_auditor.id
  ]
}

# ============= Permission Set Assignments =============

# Admin access to all accounts
resource "cloudkeeper_permission_set_assignment" "admin_production" {
  permission_set_id = cloudkeeper_permission_set.admin.id
  principal_type    = "GROUP"
  principal_id      = cloudkeeper_group.admins.name
  account_id        = cloudkeeper_aws_account.production.account_id
}

resource "cloudkeeper_permission_set_assignment" "admin_development" {
  permission_set_id = cloudkeeper_permission_set.admin.id
  principal_type    = "GROUP"
  principal_id      = cloudkeeper_group.admins.name
  account_id        = cloudkeeper_aws_account.development.account_id
}

resource "cloudkeeper_permission_set_assignment" "admin_staging" {
  permission_set_id = cloudkeeper_permission_set.admin.id
  principal_type    = "GROUP"
  principal_id      = cloudkeeper_group.admins.name
  account_id        = cloudkeeper_aws_account.staging.account_id
}

# Developer access to dev and staging
resource "cloudkeeper_permission_set_assignment" "dev_development" {
  permission_set_id = cloudkeeper_permission_set.developer.id
  principal_type    = "GROUP"
  principal_id      = cloudkeeper_group.developers.name
  account_id        = cloudkeeper_aws_account.development.account_id
}

resource "cloudkeeper_permission_set_assignment" "dev_staging" {
  permission_set_id = cloudkeeper_permission_set.developer.id
  principal_type    = "GROUP"
  principal_id      = cloudkeeper_group.developers.name
  account_id        = cloudkeeper_aws_account.staging.account_id
}

# Auditor readonly access to all accounts
resource "cloudkeeper_permission_set_assignment" "auditor_production" {
  permission_set_id = cloudkeeper_permission_set.readonly.id
  principal_type    = "GROUP"
  principal_id      = cloudkeeper_group.auditors.name
  account_id        = cloudkeeper_aws_account.production.account_id
}

resource "cloudkeeper_permission_set_assignment" "auditor_development" {
  permission_set_id = cloudkeeper_permission_set.readonly.id
  principal_type    = "GROUP"
  principal_id      = cloudkeeper_group.auditors.name
  account_id        = cloudkeeper_aws_account.development.account_id
}

resource "cloudkeeper_permission_set_assignment" "auditor_staging" {
  permission_set_id = cloudkeeper_permission_set.readonly.id
  principal_type    = "GROUP"
  principal_id      = cloudkeeper_group.auditors.name
  account_id        = cloudkeeper_aws_account.staging.account_id
}

# ============= Identity Providers =============

# Google OAuth
resource "cloudkeeper_identity_provider" "google" {
  type         = "google"
  alias        = "google"
  display_name = "Sign in with Google"
  enabled      = true

  config = jsonencode({
    clientId     = var.google_client_id
    clientSecret = var.google_client_secret
    hostedDomain = "example.com"
  })
}

# Microsoft Azure AD
resource "cloudkeeper_identity_provider" "microsoft" {
  type         = "microsoft"
  alias        = "azure-ad"
  display_name = "Sign in with Microsoft"
  enabled      = true

  config = jsonencode({
    clientId     = var.azure_client_id
    clientSecret = var.azure_client_secret
    tenantId     = var.azure_tenant_id
  })
}

# ============= Outputs =============

output "customer_id" {
  description = "The customer ID"
  value       = cloudkeeper_customer.example_corp.id
}

output "aws_accounts" {
  description = "Onboarded AWS account IDs"
  value = {
    production  = cloudkeeper_aws_account.production.account_id
    development = cloudkeeper_aws_account.development.account_id
    staging     = cloudkeeper_aws_account.staging.account_id
  }
}

output "permission_sets" {
  description = "Created permission sets"
  value = {
    admin     = cloudkeeper_permission_set.admin.id
    developer = cloudkeeper_permission_set.developer.id
    readonly  = cloudkeeper_permission_set.readonly.id
  }
}

# Additional variables for identity providers
variable "google_client_id" {
  type        = string
  description = "Google OAuth client ID"
  sensitive   = true
  default     = ""
}

variable "google_client_secret" {
  type        = string
  description = "Google OAuth client secret"
  sensitive   = true
  default     = ""
}

variable "azure_client_id" {
  type        = string
  description = "Azure AD client ID"
  sensitive   = true
  default     = ""
}

variable "azure_client_secret" {
  type        = string
  description = "Azure AD client secret"
  sensitive   = true
  default     = ""
}

variable "azure_tenant_id" {
  type        = string
  description = "Azure AD tenant ID"
  sensitive   = true
  default     = ""
}
