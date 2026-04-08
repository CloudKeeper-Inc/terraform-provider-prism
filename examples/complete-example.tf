# Complete CloudKeeper Terraform Configuration Example

terraform {
  required_version = ">= 1.0"

  required_providers {
    prism = {
      source = "CloudKeeper-Inc/prism"
    }
  }
}

provider "prism" {
  prism_subdomain = var.prism_subdomain
  base_url        = var.prism_base_url
  api_token       = var.prism_token
}

variable "prism_token" {
  type        = string
  description = "CloudKeeper Prism API token"
  sensitive   = true
}

variable "prism_base_url" {
  type        = string
  description = "Prism base URL (e.g., https://prism.cloudkeeper.com)"
}

variable "prism_subdomain" {
  type        = string
  description = "CloudKeeper Prism subdomain. (Can be found in https://{subdomain}prism.cloudkeeper.com)"
}

# ============= AWS Accounts =============

resource "prism_aws_account" "production" {
  account_id   = "123456789012"
  account_name = "Production"
  region       = "us-east-1"
}

resource "prism_aws_account" "development" {
  account_id   = "234567890123"
  account_name = "Development"
  region       = "us-east-1"
}

resource "prism_aws_account" "staging" {
  account_id   = "345678901234"
  account_name = "Staging"
  region       = "us-east-1"
}

# ============= Permission Sets =============

# Admin Permission Set
resource "prism_permission_set" "admin" {
  name             = "AdministratorAccess"
  description      = "Full administrator access"
  session_duration = "PT8H"

  managed_policies = [
    "arn:aws:iam::aws:policy/AdministratorAccess"
  ]
}

# Developer Permission Set
resource "prism_permission_set" "developer" {
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
resource "prism_permission_set" "readonly" {
  name             = "ReadOnlyAccess"
  description      = "Read-only access for auditors"
  session_duration = "PT2H"

  managed_policies = [
    "arn:aws:iam::aws:policy/ReadOnlyAccess"
  ]
}

# ============= Users =============

resource "prism_user" "john_admin" {
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

resource "prism_user" "jane_dev" {
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

resource "prism_user" "bob_auditor" {
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

resource "prism_group" "admins" {
  name        = "Administrators"
  description = "System administrators with full access"
  path        = "/teams/operations/"
}

resource "prism_group" "developers" {
  name        = "Developers"
  description = "Development team members"
  path        = "/teams/engineering/"
}

resource "prism_group" "auditors" {
  name        = "Auditors"
  description = "Security and compliance auditors"
  path        = "/teams/compliance/"
}

# ============= Group Memberships =============

resource "prism_group_membership" "admin_members" {
  group_name = prism_group.admins.name
  usernames = [
    prism_user.john_admin.username
  ]
}

resource "prism_group_membership" "dev_members" {
  group_name = prism_group.developers.name
  usernames = [
    prism_user.jane_dev.username
  ]
}

resource "prism_group_membership" "auditor_members" {
  group_name = prism_group.auditors.name
  usernames = [
    prism_user.bob_auditor.username
  ]
}

# ============= Permission Set Assignments =============

# Admin access to all accounts
resource "prism_permission_set_assignment" "admin_all_accounts" {
  permission_set_id = prism_permission_set.admin.id
  principal_type    = "GROUP"
  principal_id      = prism_group.admins.name
  account_ids = [
    prism_aws_account.production.account_id,
    prism_aws_account.development.account_id,
    prism_aws_account.staging.account_id
  ]
}

# Developer access to dev and staging
resource "prism_permission_set_assignment" "dev_non_prod" {
  permission_set_id = prism_permission_set.developer.id
  principal_type    = "GROUP"
  principal_id      = prism_group.developers.name
  account_ids = [
    prism_aws_account.development.account_id,
    prism_aws_account.staging.account_id
  ]
}

# Auditor readonly access to all accounts
resource "prism_permission_set_assignment" "auditor_all_accounts" {
  permission_set_id = prism_permission_set.readonly.id
  principal_type    = "GROUP"
  principal_id      = prism_group.auditors.name
  account_ids = [
    prism_aws_account.production.account_id,
    prism_aws_account.development.account_id,
    prism_aws_account.staging.account_id
  ]
}

# ============= Identity Providers =============

# Google OAuth
resource "prism_identity_provider" "google" {
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
resource "prism_identity_provider" "microsoft" {
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

# ============= Organization Policies =============

# Service Control Policy to restrict regions
resource "prism_org_policy" "restrict_regions" {
  management_account_id = "123456789012"
  name                  = "RestrictToApprovedRegions"
  description           = "Restrict operations to approved AWS regions"
  policy_type           = "SERVICE_CONTROL_POLICY"
  content = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Deny"
      Action   = "*"
      Resource = "*"
      Condition = {
        StringNotEquals = {
          "aws:RequestedRegion" = ["us-east-1", "us-west-2", "eu-west-1"]
        }
      }
    }]
  })

  tags = {
    Environment = "production"
    ManagedBy   = "terraform"
  }
}

# Tag Policy to enforce standard tagging
resource "prism_org_policy" "require_tags" {
  management_account_id = "123456789012"
  name                  = "RequireStandardTags"
  description           = "Enforce standard tagging across the organization"
  policy_type           = "TAG_POLICY"
  content = jsonencode({
    tags = {
      Environment = {
        tag_key = {
          "@@assign" = "Environment"
        }
        tag_value = {
          "@@assign" = ["production", "staging", "development"]
        }
      }
    }
  })
}

# Attach SCP to the production account
resource "prism_org_policy_attachment" "restrict_regions_prod" {
  management_account_id = "123456789012"
  policy_id             = prism_org_policy.restrict_regions.id
  target_id             = prism_aws_account.production.account_id
}

# Attach tag policy to an OU
resource "prism_org_policy_attachment" "tags_to_ou" {
  management_account_id = "123456789012"
  policy_id             = prism_org_policy.require_tags.id
  target_id             = "ou-abc1-exampleou"
}

# ============= Outputs =============

output "aws_accounts" {
  description = "Onboarded AWS account IDs"
  value = {
    production  = prism_aws_account.production.account_id
    development = prism_aws_account.development.account_id
    staging     = prism_aws_account.staging.account_id
  }
}

output "permission_sets" {
  description = "Created permission sets"
  value = {
    admin     = prism_permission_set.admin.id
    developer = prism_permission_set.developer.id
    readonly  = prism_permission_set.readonly.id
  }
}

output "org_policies" {
  description = "Created organization policies"
  value = {
    restrict_regions = prism_org_policy.restrict_regions.id
    require_tags     = prism_org_policy.require_tags.id
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
