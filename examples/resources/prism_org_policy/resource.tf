# Service Control Policy (SCP)
resource "prism_org_policy" "deny_s3" {
  name        = "DenyS3Access"
  description = "Deny all S3 operations"
  policy_type = "SERVICE_CONTROL_POLICY"
  content = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Deny"
      Action   = "s3:*"
      Resource = "*"
    }]
  })

  tags = {
    Environment = "production"
    ManagedBy   = "terraform"
  }
}

# Resource Control Policy (RCP)
resource "prism_org_policy" "rcp_deny_external_s3" {
  name        = "DenyExternalS3Access"
  description = "Deny S3 access from outside the organization"
  policy_type = "RESOURCE_CONTROL_POLICY"
  content = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Deny"
      Principal = "*"
      Action    = "s3:*"
      Resource  = "*"
      Condition = {
        StringNotEquals = {
          "aws:PrincipalOrgID" = "o-exampleorgid"
        }
      }
    }]
  })
}

# Tag Policy
resource "prism_org_policy" "require_environment_tag" {
  name        = "RequireEnvironmentTag"
  description = "Enforce Environment tag values"
  policy_type = "TAG_POLICY"
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

# Backup Policy
resource "prism_org_policy" "daily_backup" {
  name        = "DailyBackupPolicy"
  description = "Daily backup plan for tagged resources"
  policy_type = "BACKUP_POLICY"
  content = jsonencode({
    plans = {}
  })
}

# AI Services Opt-Out Policy
resource "prism_org_policy" "ai_optout" {
  name        = "AIServicesOptOut"
  description = "Opt out of all AI service data usage"
  policy_type = "AISERVICES_OPT_OUT_POLICY"
  content = jsonencode({
    services = {
      default = {
        opt_out_policy = {
          "@@assign" = "optOut"
        }
      }
    }
  })
}
