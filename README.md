# Terraform Provider for CloudKeeper Prism 

This is the official Terraform provider for CloudKeeper Prism, a centralized workforce identity and access management solution.
## Features

The CloudKeeper Prism Terraform provider allows you to manage:

- **AWS Accounts**: Onboarded AWS accounts with SAML/OIDC configuration
- **Permission Sets**: IAM-like permission definitions
- **Permission Set Assignments**: Assign permissions to users/groups for specific accounts
- **Users**: Keycloak users with attributes
- **Groups**: User groups with hierarchies
- **Group Memberships**: Manage group membership
- **Identity Providers**: Google, Microsoft Azure AD, Keycloak, and custom OIDC providers

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.23 (for development)
- CloudKeeper Prism instance with API access

## Installation

### Using the Provider

Add the following to your Terraform configuration:

```hcl
terraform {
  required_providers {
    prism = {
      source = "CloudKeeper-Inc/prism"
    }
  }
}

provider "prism" {
  prism_subdomain = "YOUR_PRISM_SUBDOMAIN"
  api_token = var.prism_token
}
```

### Local Development

1. Clone the repository:
```bash
git clone https://github.com/CloudKeeper-Inc/terraform-provider-prism.git
cd terraform-provider-prism
```

2. Build and install the provider:
```bash
make install
```

3. Create a `.terraformrc` file in your home directory with local provider override:
```hcl
provider_installation {
  dev_overrides {
    "CloudKeeper-Inc/prism" = "/path/to/your/go/bin"
  }
  direct {}
}
```

## Quick Start Example

```hcl
# Configure the provider
provider "cloudkeeper" {
  prism_subdomain = "YOUR_PRISM_SUBDOMAIN"
  api_token = var.prism_token
}

# Onboard an AWS account
resource "prism_aws_account" "production" {
  account_id   = "123456789012"
  account_name = "Production"
  region       = "us-east-1"
}

# Create a permission set
resource "prism_permission_set" "developer" {
  name             = "DeveloperAccess"
  description      = "Developer access permissions"
  session_duration = "PT12H"

  managed_policies = [
    "arn:aws:iam::aws:policy/ReadOnlyAccess"
  ]

  inline_policies = {
    s3_access = jsonencode({
      Version = "2012-10-17"
      Statement = [
        {
          Effect = "Allow"
          Action = [
            "s3:ListBucket",
            "s3:GetObject"
          ]
          Resource = "*"
        }
      ]
    })
  }
}

# Create a user
resource "prism_user" "john_doe" {
  username   = "john.doe"
  email      = "john.doe@example.com"
  first_name = "John"
  last_name  = "Doe"
  enabled    = true
}

# Create a group
resource "prism_group" "developers" {
  name        = "Developers"
  description = "Development team"
}

# Add user to group
resource "prism_group_membership" "dev_members" {
  group_name = prism_group.developers.name
  user_ids   = [prism_user.john_doe.id]
}

# Assign permission set to group for multiple accounts
resource "prism_permission_set_assignment" "dev_access" {
  permission_set_id = prism_permission_set.developer.id
  principal_type    = "GROUP"
  principal_id      = prism_group.developers.name
  account_ids       = [prism_aws_account.production.account_id]
}

# Configure identity provider
resource "prism_identity_provider" "google" {
  type         = "google"
  alias        = "google"
  display_name = "Sign in with Google"
  enabled      = true

  config = jsonencode({
    clientId     = "your-google-client-id"
    clientSecret = "your-google-client-secret"
    hostedDomain = "example.com"
  })
}
```

## Configuration

### Environment Variables

- `PRISM_SUBDOMAIN`: CloudKeeper Prism subdomain
- `PRISM_API_TOKEN`: API authentication token

### Provider Arguments

- `prism_subdomain` (Optional, String): The subdomain of your tenant in CloudKeeper Prism. Can also be set via `PRISM_SUBDOMAIN` environment variable.
- `api_token` (Optional, String, Sensitive): The API token for authentication. Can also be set via `PRISM_API_TOKEN` environment variable.

## Resources

### prism_aws_account

Manages an AWS account onboarded to CloudKeeper.

**Arguments:**
- `account_id` (Required, String): AWS account ID (12-digit)
- `account_name` (Required, String): Friendly name
- `region` (Optional, String): Primary AWS region
- `role_arn` (Optional, String): IAM role ARN for cross-account access

### prism_permission_set

Manages a permission set.

**Arguments:**
- `name` (Required, String): Permission set name
- `description` (Optional, String): Description
- `session_duration` (Optional, String): Session duration (ISO 8601 format, e.g., PT4H)
- `managed_policies` (Optional, List of Strings): AWS managed policy ARNs
- `inline_policies` (Optional, Map of Strings): Map of inline IAM policies (JSON). Key is the policy name, value is the policy document.

### prism_permission_set_assignment

Assigns a permission set to a user or group for multiple AWS accounts.

**Arguments:**
- `permission_set_id` (Required, String): Permission set ID
- `principal_type` (Required, String): Principal type (USER or GROUP)
- `principal_id` (Required, String): Username or group name
- `account_ids` (Required, List of Strings): List of AWS account IDs to grant access to

### prism_user

Manages a user.

**Arguments:**
- `username` (Required, String): Username
- `email` (Required, String): Email address
- `first_name` (Optional, String): First name
- `last_name` (Optional, String): Last name
- `enabled` (Optional, Bool): Whether user is enabled (default: true)
- `attributes` (Optional, Map of Strings): Custom attributes

### prism_group

Manages a group.

**Arguments:**
- `name` (Required, String): Group name
- `description` (Optional, String): Description
- `path` (Optional, String): Group path for hierarchy

### prism_group_membership

Manages group membership.

**Arguments:**
- `group_name` (Required, String): Group name
- `user_ids` (Required, List of Strings): User IDs to add to group

### prism_identity_provider

Manages an identity provider.

**Arguments:**
- `type` (Required, String): Provider type (google, microsoft, keycloak, custom)
- `alias` (Required, String): Provider alias
- `display_name` (Optional, String): Display name
- `enabled` (Optional, Bool): Whether provider is enabled (default: true)
- `config` (Required, String, Sensitive): JSON configuration

## Data Sources

All resources have corresponding data sources for reading existing configurations:

- `data.prism_customer`
- `data.prism_aws_account`
- `data.prism_permission_set`
- `data.prism_user`
- `data.prism_group`

## Importing Existing Infrastructure

If you have existing Prism infrastructure that you want to manage with Terraform, use the built-in import tool to automatically generate Terraform configuration from your current setup.

### Quick Start

```bash
# Generate Terraform code from existing infrastructure
make generate-terraform SUBDOMAIN=your-subdomain TOKEN=your-api-token

# Navigate to generated directory
cd generated-terraform

# Review generated files
ls -la

# Initialize Terraform
terraform init

# Import existing resources into state
chmod +x import.sh
./import.sh

# Verify everything matches
terraform plan
```

### Features

- ✅ Automatically fetches all resources from your Prism instance
- ✅ Generates organized `.tf` files (separate files for users, groups, etc.)
- ✅ Extracts repeated values into variables
- ✅ Creates import script for bringing resources into Terraform state
- ✅ Uses proper Terraform references instead of hardcoded values

### Generated Files

The import tool creates:
- `provider.tf` - Provider configuration
- `variables.tf` - Variable definitions
- `terraform.tfvars` - Variable values template
- `aws_accounts.tf` - AWS account resources
- `permission_sets.tf` - Permission sets with policies
- `users.tf` - User resources
- `groups.tf` - Groups and memberships
- `assignments.tf` - Permission set assignments
- `import.sh` - Import commands script

For detailed documentation, see [tools/terraform-import/README.md](tools/terraform-import/README.md).

## Development

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Acceptance Tests

```bash
make testacc
```

### Generating Documentation

```bash
make docs
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## License

This provider is licensed under the Mozilla Public License 2.0.
