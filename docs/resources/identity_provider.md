---
page_title: "prism_identity_provider Resource - terraform-provider-prism"
subcategory: ""
description: |-
  Manages an identity provider configuration in CloudKeeper. Supports Google, Microsoft Azure AD, Keycloak federation, custom OIDC providers, and SAML 2.0 providers such as Microsoft Entra ID, Okta and ADFS.
---

# prism_identity_provider (Resource)

Manages an identity provider configuration in CloudKeeper. Supports Google, Microsoft Azure AD, Keycloak federation, custom OIDC providers, and SAML 2.0 providers such as Microsoft Entra ID, Okta and ADFS.

## Example Usage

```terraform
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
```

## Configuration Reference

`config` is a JSON string, normally produced with `jsonencode()`. The keys it
accepts depend on `type`. Keys that are not listed for the type you are using are
ignored: a misspelled key such as `tokenURL` instead of `tokenUrl` produces a
provider that applies cleanly but does not work, with no error and no diff.

Note that `display_name` is a top-level attribute, not a `config` key.

### `type = "custom"` (OIDC)

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `clientId` | String | Yes | Client ID of the application registered with your identity provider. |
| `clientSecret` | String | Yes | Client secret for that application. |
| `issuer` | String | Yes | OIDC issuer URL, for example `https://your-org.okta.com/oauth2/default`. |
| `authorizationUrl` | String | Yes | Authorization endpoint. Users are redirected here to authenticate. |
| `tokenUrl` | String | Yes | Token endpoint, used to exchange authorization codes for tokens. |
| `userInfoUrl` | String | Yes | UserInfo endpoint, used to retrieve the user profile. |
| `logoutUrl` | String | No | Logout endpoint. When set, users are redirected here on sign-out to end the session with the identity provider. |

URL-valued keys use a lowercase `Url` suffix, not `URL`.

### `type = "google"`

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `clientId` | String | Yes | OAuth client ID from the Google Cloud console. |
| `clientSecret` | String | Yes | OAuth client secret. |
| `hostedDomain` | String | No | Restrict sign-in to a single Google Workspace domain. |

### `type = "microsoft"`

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `clientId` | String | Yes | Application (client) ID from the Azure app registration. |
| `clientSecret` | String | Yes | Client secret for that registration. |
| `tenantId` | String | Yes | Directory (tenant) ID. |

### `type = "saml"`

A SAML 2.0 provider: Microsoft Entra ID, Okta, Google Workspace, ADFS. Prism is
the Service Provider and your identity provider is the asserting party.

Describe the connection with a metadata document if you can, and fill in the
endpoints by hand only if you cannot. A `config` carrying nothing but
`metadataUrl` is a complete, working configuration.

#### Metadata

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `metadataUrl` | String | No | URL of the identity provider's SAML federation metadata document, `https` only. Recommended: it carries the endpoints, the entity ID and the signing certificate in one value, and Prism re-resolves it on every create and update - so a certificate rotated at the identity provider is picked up by the next apply that updates this resource, which the manual form cannot do. For Entra ID use the per-application Federation Metadata Url, the one with the `?appid=` query parameter, not the tenant-wide document. |
| `metadataXml` | String | No | The metadata document itself, for identity providers that do not publish it at a URL: `metadataXml = file("${path.module}/entra-metadata.xml")`. Unlike `metadataUrl` this is inert - it is refreshed only when you update the file, which does at least show up as a real diff. Setting both metadata keys at once is rejected. |

#### Endpoints and trust

Required when no metadata document is supplied. When one is, these keys are
optional and override it: an imported value only fills a field you left empty, so
anything you state explicitly always wins.

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `idpEntityId` | String | Yes | The identity provider's entity ID, used to validate the assertion Issuer. Entra ID publishes `https://sts.windows.net/{tenantId}/` - the trailing slash is part of the value, never trim it. |
| `singleSignOnServiceUrl` | String | Yes | The identity provider's SSO endpoint, `https` only. Users are redirected here to authenticate. |
| `signingCertificate` | String | Yes, when `validateSignature` is `true` | The identity provider's token-signing certificate as bare base64: strip the `-----BEGIN CERTIFICATE-----` / `-----END CERTIFICATE-----` armor. Comma-separate several certificates to accept more than one during a rotation. An empty string means *unchanged* - it is not sent at all, so it cannot clear the stored certificate. |
| `entityId` | String | No | Overrides the Service Provider entity ID that Prism advertises. Defaults to `https://{keycloak-host}/realms/{realm}`. Change it only if you have already registered a different Identifier at your identity provider. |
| `metadataDescriptorUrl` | String | No | Recorded alongside the provider so the Prism console can offer to re-import a rotated signing certificate. Informational; Prism does not fetch it during login. |

#### Identity mapping and protocol options

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `nameIDPolicyFormat` | String | No | NameID format requested in the AuthnRequest. Note the capital `ID`. The legal URNs span two SAML versions: `urn:oasis:names:tc:SAML:2.0:nameid-format:persistent` and `...:2.0:nameid-format:transient`, but `urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress` and `...:1.1:nameid-format:unspecified`. A hand-written `2.0:nameid-format:emailAddress` does not exist and fails every login. |
| `principalType` | String | No | How the username is taken from the assertion: `SUBJECT`, `ATTRIBUTE` or `FRIENDLY_ATTRIBUTE`. Entra ID needs `ATTRIBUTE`, because it publishes no NameID format and its NameID is an opaque pairwise identifier. Do not write `ATTRIBUTE_NAME`: Keycloak accepts the string, silently falls back to `SUBJECT`, and usernames become opaque GUIDs. |
| `principalAttribute` | String | Yes, when `principalType` is not `SUBJECT` | Name of the assertion attribute holding the username. For Entra ID use the UPN claim, `http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name` - it is always populated, whereas the email claim is empty for mailbox-less users and most B2B guests. |
| `emailAttributeName` | String | No | Name of the assertion attribute holding the user's email address, which is what Prism matches federated logins against. Defaults to `http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress`. |
| `signatureAlgorithm` | String | No | `RSA_SHA256` (the default, and the only algorithm Entra ID verifies), `RSA_SHA512` or `RSA_SHA1`. |
| `validateSignature` | Bool | No | Verify the signature on incoming assertions against `signingCertificate`. Leave it on. |
| `wantAssertionsSigned` | Bool | No | Require the assertion itself to be signed. Matches Entra ID's default. |
| `wantAuthnRequestsSigned` | Bool | No | Sign the AuthnRequest Prism sends. Entra ID does not require it. |
| `forceAuthn` | Bool | No | Ask the identity provider to re-authenticate the user even if they have a live session. |
| `postBindingResponse` | Bool | No | Expect the response over HTTP-POST rather than Redirect. |
| `postBindingAuthnRequest` | Bool | No | Send the AuthnRequest over HTTP-POST rather than Redirect. |
| `allowedClockSkew` | Number | No | Seconds of tolerance on the assertion's `NotBefore` and `NotOnOrAfter`, 0-300. Entra ID adds no buffer of its own, so a small non-zero value avoids sporadic failures. |

`enabled` is also accepted inside `config`, but use the top-level `enabled`
attribute instead: when both are present the `config` value is the one that is
sent, which makes the resource's own attribute look as though it is ignored.

#### Booleans must be HCL booleans

Write `validateSignature = true`, not `validateSignature = "true"`. The quoted
forms are accepted and coerced, but any other string - `"yes"`, `"on"` - is an
error rather than something silently dropped.

#### `principalType` and `principalAttribute` are effectively immutable

They decide which value in the assertion identifies a user. Changing either on a
provider that people have already signed in through orphans every existing
federated link: those users are no longer recognised, and the next login is
treated as a brand new identity. Pick them before the first login and leave them
alone.

#### Keys that are deliberately not supported

| Key | Why |
|-----|-----|
| `wantAssertionsEncrypted`, `encryptionPublicKey` | Assertion encryption is not read by the SAML broker in this configuration; the assertion travels inside TLS. |
| `singleLogoutServiceUrl`, `backchannelSupported`, `postBindingLogout` | Entra ID publishes no SOAP logout endpoint, and Prism's own sign-out is an admin-REST session kill rather than a SAML flow. A half-wired logout URL produces logout failures that present as login failures. |
| `loginHint` | Entra ID rejects an AuthnRequest that carries a `<Subject>`. |
| `authnContextClassRefs`, `authnContextDeclRefs` | Entra ID answers a requested authentication context with AADSTS75011. |
| `hideOnLoginPage` | It hides the provider's button. On an identity-provider-only realm that leaves a login page with nothing on it. |
| `useMetadataDescriptorUrl`, `descriptorCacheSeconds` | Prism pins the signing certificate. Turning the switch on would make every login trust a live fetch of a customer-controlled URL instead, and `config` is never refreshed, so the change would never show as a diff. |

#### How metadata is resolved

When `metadataUrl` or `metadataXml` is present, Prism resolves it server-side on
both create and update: it fetches or reads the document, parses it, and fills in
the endpoints, entity ID, signing certificate and identity mapping from it. There
is no separate import step to run first.

- Exactly one of the two may be set. Setting both is an error.
- Imported values only fill in fields you left empty. An explicitly supplied
  `singleSignOnServiceUrl`, `idpEntityId`, `signingCertificate`,
  `nameIDPolicyFormat`, `principalType` or `principalAttribute` always wins over
  the document, which is how you correct or pin a value the document gets wrong.
- Validation runs after resolution, against the merged result. A create carrying
  only `metadataUrl` therefore passes.

#### Backend compatibility

The `saml` type calls SAML-specific endpoints. A Prism backend older than this
release answers them with 404, and the provider does not version-gate: the apply
fails with a not-found error rather than a clear "unsupported provider type"
message.

## Aliases and Redirect URIs

`alias` is read-only. Prism assigns it when the provider is created, and it stays
the same for as long as the resource exists. You need it to import an existing
provider, and it determines the redirect URI that your identity provider must
allow. Read it from the Identity Providers page in the Prism admin console, or
from `terraform show`.

Changing `display_name` on an existing resource does not change the alias, so the
redirect URI is unaffected. Replacing the resource does produce a new alias, and
therefore a new redirect URI, which you must then add to the application
registration on your identity provider's side. Anything that forces replacement -
including changing `type`, or destroying and recreating the resource - has this
effect.

For `type = "saml"` the alias appears in the Service Provider values you register
at your identity provider. All three are built from the Prism realm's Keycloak
host and are shown on the Identity Providers page after the provider is created:

| Value | Form | Registered at the identity provider as |
|-------|------|----------------------------------------|
| ACS URL | `https://{keycloak-host}/realms/{realm}/broker/{alias}/endpoint` | Reply URL / Assertion Consumer Service URL |
| SP Entity ID | `https://{keycloak-host}/realms/{realm}` | Identifier / Audience (unless overridden with `entityId`) |
| SP metadata descriptor | `https://{keycloak-host}/realms/{realm}/broker/{alias}/endpoint/descriptor` | Not registered; a public document you can import at the identity provider. It 404s until the provider exists. |

Only the ACS URL contains the alias, and that makes replacement dangerous: any
change that **replaces** the resource assigns a new alias and therefore a new ACS
URL. Update the Reply URL at the identity provider **first**, or every login
fails with AADSTS50011 (Entra ID) or the equivalent "reply URL does not match"
error, including for users who are already signed in when their session expires.
Reply URL matching is exact - case and trailing slash included.

## Drift Detection

`config` is not refreshed from the API, because the client secret cannot be read
back. Terraform therefore reports no drift for any value inside `config`,
including the endpoint URLs, even if it was changed outside Terraform. Terraform
remains authoritative on the next apply that changes `config` for another reason;
to push your configured values back deliberately, taint the resource or make a
no-op edit to `config`.

For `type = "saml"` this cuts differently depending on how you described the
provider, and signing-certificate rotation is where it matters.

**Managed by `metadataUrl`.** Prism re-resolves the document server-side on every
create and update, so the stored certificate is refreshed whenever the resource is
updated - a rotation at the identity provider needs no Terraform change of its
own. What Terraform still cannot do is *notice* the rotation: nothing inside
`config` is refreshed, so the rotation produces no diff and schedules no apply by
itself. Any apply that updates this resource for any reason picks up the current
certificate; if none is due, make a real change to `config` - bumping
`allowedClockSkew` by a second is enough - to force one. Do **not** taint or
`-replace` a SAML provider to force a refresh: replacement mints a new alias and
therefore a new ACS URL, which breaks every login until the identity provider is
updated to match.

**Described by hand.** The certificate is pinned in `config` and never refreshes,
so a rotation produces **no** diff and nothing warns you before logins start
failing. Rotate in three applies:

1. Append the new certificate to `signingCertificate`, comma-separated, and
   apply. Both certificates are then trusted.
2. Cut over at the identity provider and confirm real logins are using the new
   certificate.
3. Remove the old certificate and apply again.

Keycloak silently ignores an expired certificate rather than reporting it, so a
`signingCertificate` whose entries have all expired fails every login with no
signal on the Prism side. Note also that the certificate you put in `config`
lands in Terraform state: `config` is marked sensitive, so it is redacted from
plan output, but state files and any remote backend holding them still need to be
protected accordingly.

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `config` (String, Sensitive) JSON-encoded configuration for the identity provider, normally written with `jsonencode()`. The accepted keys depend on `type` - see the configuration reference below. Keys that are not recognised for the given type are ignored, so a misspelled key produces a provider that applies cleanly but does not work.
- `type` (String) The type of identity provider (google, microsoft, keycloak, custom, saml). `custom` is a generic OIDC provider; `saml` is a SAML 2.0 provider.

### Optional

- `display_name` (String) The display name for the identity provider
- `enabled` (Boolean) Whether the identity provider is enabled

### Read-Only

- `alias` (String) The alias that identifies this identity provider in Prism. Assigned when the provider is created and stable for as long as the resource exists. It forms part of the import ID and of the provider's redirect URI; retrieve it from the Prism admin console or `terraform show`.
- `id` (String) The unique identifier for the identity provider

## Import

Import is supported using the following syntax:

```shell
# Identity providers are imported using the type and alias, separated by a colon.
# Format: <type>:<alias>
#
# The alias is assigned by Prism when the provider is created and is not derivable
# from your configuration. Read it from the Identity Providers page in the Prism
# admin console, or from `terraform show` if the resource is already managed.
terraform import prism_identity_provider.okta "custom:custom-okta-ck"
terraform import prism_identity_provider.google "google:google-sign-in"

# The client secret cannot be read back, so the first plan after an import shows a
# diff for `config`. Applying it re-sends your configured values and reconciles
# the state.

# SAML providers are imported the same way. The alias is generated from the
# display name with a saml- prefix, so it contains the type as a substring.
terraform import prism_identity_provider.entra_saml "saml:saml-contoso-entra-id"

# The first plan after importing a SAML provider also shows a config diff:
# nothing inside `config` is refreshed from the API, so it starts out empty in
# state. Check the plan before applying - it re-sends every configured value,
# including the signing certificate.
```

The first plan after an import shows a diff for `config`, because the client
secret cannot be read back from the API. Applying it re-sends your configured
values and reconciles the state.
