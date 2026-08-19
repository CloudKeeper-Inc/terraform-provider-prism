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

# SAML 2.0 Identity Provider from federation metadata (Microsoft Entra ID)
#
# This is the whole configuration. Prism resolves the metadata document on create
# and on every update, and takes the SSO endpoint, the identity provider's entity
# ID and the signing certificate from it - there is no separate import step, and
# a rotated certificate is picked up by the next apply that updates the resource.
# Use the per-application Federation Metadata Url - the one with the ?appid=
# query parameter - not the tenant-wide document.
#
# Create the resource first, then register the Reply URL (ACS URL) and Identifier
# shown on the Prism Identity Providers page with the Entra enterprise
# application. Sign-in does not work until you do.
resource "prism_identity_provider" "entra_saml" {
  type         = "saml"
  display_name = "Contoso Entra ID"
  enabled      = true

  config = jsonencode({
    metadataUrl = "https://login.microsoftonline.com/00000000-0000-0000-0000-000000000000/federationmetadata/2007-06/federationmetadata.xml?appid=11111111-1111-1111-1111-111111111111"

    # Anything set here wins over the metadata document; imported values only
    # fill in what you leave out. Entra ID publishes no NameID format and its
    # NameID is an opaque pairwise identifier, so pin the username to the UPN
    # claim, which is always populated.
    principalType      = "ATTRIBUTE"
    principalAttribute = "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name"
    emailAttributeName = "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress"

    # Booleans are HCL booleans, not quoted strings.
    validateSignature    = true
    wantAssertionsSigned = true
  })
}

# SAML 2.0 Identity Provider configured by hand
#
# Use this when your identity provider publishes no metadata document, or when
# you want every value pinned in code. Pinned means pinned: the certificate below
# never refreshes on its own, so a rotation at the identity provider takes three
# applies (append the new certificate, cut over, remove the old one).
resource "prism_identity_provider" "saml_manual" {
  type         = "saml"
  display_name = "Contoso SAML"
  enabled      = true

  config = jsonencode({
    # The trailing slash on an Entra ID entity ID is significant - never trim it.
    idpEntityId            = "https://sts.windows.net/00000000-0000-0000-0000-000000000000/"
    singleSignOnServiceUrl = "https://login.microsoftonline.com/00000000-0000-0000-0000-000000000000/saml2"

    # Bare base64: strip the -----BEGIN CERTIFICATE----- / -----END CERTIFICATE-----
    # armor and the line breaks. PEM armor is not tolerated - it produces nested
    # headers inside Keycloak and a 500 at the assertion consumer endpoint.
    # Comma-separate several certificates to trust more than one during rotation.
    signingCertificate = "MIIC8DCCAdigAwIBAgIQZG1lbW9uc3RyYXRpb25jZXJ0aWZpY2F0ZXBsYWNlaG9sZGVy"

    nameIDPolicyFormat = "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent"
    principalType      = "ATTRIBUTE"
    principalAttribute = "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name"
    emailAttributeName = "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress"

    signatureAlgorithm      = "RSA_SHA256"
    validateSignature       = true
    wantAssertionsSigned    = true
    wantAuthnRequestsSigned = false
    forceAuthn              = false
    postBindingResponse     = true
    postBindingAuthnRequest = true
    allowedClockSkew        = 30
  })
}
