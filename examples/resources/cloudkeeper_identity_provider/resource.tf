# Google Identity Provider
resource "cloudkeeper_identity_provider" "google" {
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

# Microsoft Azure AD Identity Provider
resource "cloudkeeper_identity_provider" "microsoft" {
  type         = "microsoft"
  alias        = "azure-ad"
  display_name = "Sign in with Microsoft"
  enabled      = true

  config = jsonencode({
    clientId     = "your-azure-client-id"
    clientSecret = "your-azure-client-secret"
    tenantId     = "your-azure-tenant-id"
  })
}
