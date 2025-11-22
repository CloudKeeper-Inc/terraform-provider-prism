# Publishing Guide for CloudKeeper Prism Terraform Provider

This guide walks you through publishing the `cloudkeeper/prism` provider to the Terraform Registry.

## Provider Details

- **Registry Name**: `cloudkeeper/prism`
- **Registry URL**: https://registry.terraform.io/providers/cloudkeeper/prism
- **GitHub Repository**: https://github.com/CloudKeeper-Inc/terraform-provider-prism
- **Provider Documentation**: https://registry.terraform.io/providers/cloudkeeper/prism/latest/docs

## Prerequisites

### 1. GitHub Organization Setup

The provider must be published under the `cloudkeeper` namespace, which requires:
- Access to a GitHub organization named `cloudkeeper` (or request namespace verification from HashiCorp if using `CloudKeeper-Inc`)
- The repository `terraform-provider-prism` must be **public**
- Repository must be under the organization (not a personal account)

### 2. GPG Key for Signing Releases

Terraform Registry requires all providers to sign their releases with a GPG key.

#### Generate a GPG Key

```bash
# Generate a new GPG key
gpg --full-generate-key

# When prompted:
# - Key type: (1) RSA and RSA
# - Key size: 4096
# - Expiration: 0 (never expires) or set an appropriate expiration
# - Real name: Your name or "CloudKeeper Inc"
# - Email: Use the email associated with your GitHub account
```

#### Export Your GPG Key

```bash
# List your keys to get the key ID
gpg --list-secret-keys --keyid-format=long

# The output will look like:
# sec   rsa4096/ABCD1234EFGH5678 2024-01-01 [SC]
#       1234567890ABCDEF1234567890ABCDEF12345678
# uid                 [ultimate] Your Name <your.email@example.com>

# Export the public key (replace with your key ID)
gpg --armor --export ABCD1234EFGH5678 > terraform-registry-key.asc

# Export the private key for GitHub Secrets
gpg --armor --export-secret-keys ABCD1234EFGH5678 > private-key.asc

# Get the fingerprint
gpg --fingerprint ABCD1234EFGH5678
```

### 3. Add GitHub Secrets

Add these secrets to your GitHub repository (Settings → Secrets and variables → Actions → New repository secret):

1. **GPG_PRIVATE_KEY**: Contents of `private-key.asc` (the entire key including headers)
2. **PASSPHRASE**: Your GPG key passphrase (if you set one)

## Publishing Steps

### Step 1: Sign Up for Terraform Registry

1. Go to https://registry.terraform.io/
2. Click "Sign in" and authenticate with your GitHub account
3. Ensure you're signed in with access to the `cloudkeeper` organization (or `CloudKeeper-Inc`)

### Step 2: Add Your GPG Public Key to Registry

1. Go to https://registry.terraform.io/settings/gpg-keys
2. Click "Add GPG Key"
3. Paste the contents of `terraform-registry-key.asc`
4. Click "Add key"

### Step 3: Publish the Provider

1. Go to https://registry.terraform.io/publish/provider
2. Select repository: `CloudKeeper-Inc/terraform-provider-prism`
3. Click "Publish Provider"

The registry will:
- Verify you have admin access to the repository
- Set up webhooks to detect new releases
- Verify your GPG signing setup on the first release

### Step 4: Create Your First Release

Once everything is set up, create a release:

```bash
# Ensure all changes are committed
git add .
git commit -m "feat: initial provider release"
git push origin main

# Tag the release
git tag v1.0.0
git push origin v1.0.0
```

The GitHub Action will automatically:
1. Build binaries for all platforms
2. Create SHA256 checksums
3. Sign the checksums with your GPG key
4. Create a GitHub release
5. Upload all artifacts

The Terraform Registry will detect the new tag and index the release automatically.

## Release Checklist

Before creating a release, ensure:

- [ ] All tests pass: `make test`
- [ ] Provider builds successfully: `make build`
- [ ] Documentation is up to date
- [ ] CHANGELOG is updated (if you have one)
- [ ] Version follows semantic versioning (v1.0.0, v1.1.0, etc.)
- [ ] GPG key is configured in GitHub secrets
- [ ] Repository is public
- [ ] Provider is registered in Terraform Registry

## Using the Published Provider

Once published, users can use your provider like this:

```hcl
terraform {
  required_providers {
    prism = {
      source  = "cloudkeeper/prism"
      version = "~> 1.0"
    }
  }
}

provider "prism" {
  prism_subdomain = "your-subdomain"
  api_token       = var.prism_token
}

resource "prism_user" "example" {
  username = "john.doe"
  email    = "john.doe@example.com"
}
```

## Troubleshooting

### GPG Signature Verification Failed

If the release fails with GPG errors:
1. Verify the GPG_PRIVATE_KEY secret contains the entire key including headers
2. Ensure the passphrase is correct
3. Check that the public key is added to Terraform Registry

### Registry Not Picking Up Releases

If Terraform Registry doesn't detect your release:
1. Ensure the repository is public
2. Verify the webhook is configured (Settings → Webhooks)
3. Check that the tag format is correct (must start with `v`)
4. Ensure all required artifacts are present in the GitHub release

### Namespace Issues

If you can't publish under `cloudkeeper`:
1. You may need to request namespace verification from HashiCorp
2. Email terraform-provider-dev@hashicorp.com with:
   - Your desired namespace
   - GitHub organization link
   - Proof of ownership/authorization

## Updating the Provider

For subsequent releases:

```bash
# Make your changes
git add .
git commit -m "feat: add new feature"
git push origin main

# Create a new tag (increment version appropriately)
git tag v1.1.0
git push origin v1.1.0
```

The release workflow will automatically handle everything else.

## Additional Resources

- [Terraform Registry Publishing Guide](https://www.terraform.io/docs/registry/providers/publishing.html)
- [Provider Requirements](https://www.terraform.io/docs/registry/providers/requirements.html)
- [GoReleaser Documentation](https://goreleaser.com/intro/)
- [GPG Signing Guide](https://docs.github.com/en/authentication/managing-commit-signature-verification/generating-a-new-gpg-key)
