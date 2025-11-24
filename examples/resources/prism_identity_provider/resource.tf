# Google Identity Provider
resource "prism_identity_provider" "google" {
  type         = "google"
  display_name = "Sign in with Google"
  enabled      = true

  config = jsonencode({
    clientId     = "your-google-client-id"
    clientSecret = "your-google-client-secret"
    hostedDomain = "example.com"
  })
}

# Note: The alias is automatically computed by the backend based on type
# For google: alias = "google"
# For microsoft: alias = "microsoft"
# For keycloak: alias = "keycloak"

# Microsoft Azure AD Identity Provider
resource "prism_identity_provider" "microsoft" {
  type         = "microsoft"
  display_name = "Sign in with Microsoft"
  enabled      = true

  config = jsonencode({
    clientId     = "your-azure-client-id"
    clientSecret = "your-azure-client-secret"
    tenantId     = "your-azure-tenant-id"
  })
}
