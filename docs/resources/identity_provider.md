---
page_title: "prism_identity_provider Resource - terraform-provider-prism"
subcategory: ""
description: |-
  Manages an identity provider configuration in CloudKeeper. Supports Google, Microsoft Azure AD, Keycloak federation, and custom OIDC providers.
---

# prism_identity_provider (Resource)

Manages an identity provider configuration in CloudKeeper. Supports Google, Microsoft Azure AD, Keycloak federation, and custom OIDC providers.

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

## Drift Detection

`config` is not refreshed from the API, because the client secret cannot be read
back. Terraform therefore reports no drift for any value inside `config`,
including the endpoint URLs, even if it was changed outside Terraform. Terraform
remains authoritative on the next apply that changes `config` for another reason;
to push your configured values back deliberately, taint the resource or make a
no-op edit to `config`.

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `config` (String, Sensitive) JSON-encoded configuration for the identity provider, normally written with `jsonencode()`. The accepted keys depend on `type` - see the configuration reference below. Keys that are not recognised for the given type are ignored, so a misspelled key produces a provider that applies cleanly but does not work.
- `type` (String) The type of identity provider (google, microsoft, keycloak, custom)

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
```

The first plan after an import shows a diff for `config`, because the client
secret cannot be read back from the API. Applying it re-sends your configured
values and reconciles the state.
