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

# Custom OIDC Identity Provider (Okta, Auth0, OneLogin, Ping, ...)
#
# Register the application with your identity provider first, then add the
# redirect URI shown on the Prism Identity Providers page to that application's
# list of allowed redirect URIs.
resource "prism_identity_provider" "okta" {
  type         = "custom"
  display_name = "Okta"
  enabled      = true

  config = jsonencode({
    clientId         = "your-okta-client-id"
    clientSecret     = "your-okta-client-secret"
    issuer           = "https://your-org.okta.com/oauth2/default"
    authorizationUrl = "https://your-org.okta.com/oauth2/default/v1/authorize"
    tokenUrl         = "https://your-org.okta.com/oauth2/default/v1/token"
    userInfoUrl      = "https://your-org.okta.com/oauth2/default/v1/userinfo"
    logoutUrl        = "https://your-org.okta.com/oauth2/default/v1/logout"
  })
}
