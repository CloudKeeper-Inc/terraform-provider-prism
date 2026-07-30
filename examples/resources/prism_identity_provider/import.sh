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
