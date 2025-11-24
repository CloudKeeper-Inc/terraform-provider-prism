# Documentation Guide

This document explains how the Terraform Provider documentation is structured and how to maintain it.

## Overview

The CloudKeeper Prism provider uses **terraform-plugin-docs** to automatically generate documentation from:
1. Schema descriptions in the provider code
2. Example Terraform files
3. Custom templates

## Documentation Structure

```
terraform-provider-prism/
├── docs/                          # Generated documentation (published to Terraform Registry)
│   ├── index.md                   # Provider overview (generated from template)
│   ├── resources/                 # Resource documentation
│   │   ├── aws_account.md
│   │   ├── permission_set.md
│   │   ├── permission_set_assignment.md
│   │   ├── user.md
│   │   ├── group.md
│   │   ├── group_membership.md
│   │   └── identity_provider.md
│   └── data-sources/              # Data source documentation
│       ├── aws_account.md
│       ├── permission_set.md
│       ├── user.md
│       └── group.md
│
├── templates/                     # Custom templates for documentation
│   └── index.md.tmpl              # Provider overview template
│
├── examples/                      # Example code used in documentation
│   ├── provider/
│   │   └── provider.tf            # Provider configuration example
│   └── resources/
│       ├── prism_aws_account/
│       │   ├── resource.tf        # Example usage
│       │   └── import.sh          # Import example
│       ├── prism_permission_set/
│       │   ├── resource.tf
│       │   └── import.sh
│       └── ...
│
└── .tfplugindocs.yml              # Documentation generator configuration
```

## How It Works

### 1. Provider Index Page

The main provider documentation (`docs/index.md`) is generated from `templates/index.md.tmpl`.

**To modify:**
- Edit `templates/index.md.tmpl`
- Run `make docs` to regenerate
- The template supports Go template syntax and can reference example files:
  ```
  {{ tffile "examples/provider/provider.tf" }}
  ```

### 2. Resource Documentation

Resource pages are automatically generated from:

**Schema Descriptions** (in Go code):
```go
"name": schema.StringAttribute{
    Required:            true,
    MarkdownDescription: "The name of the permission set",
},
```

**Example Files**:
- `examples/resources/<resource_name>/resource.tf` - Usage examples
- `examples/resources/<resource_name>/import.sh` - Import examples

### 3. Data Source Documentation

Similar to resources, data source docs are generated from schema descriptions and examples in `examples/data-sources/`.

## Maintaining Documentation

### Adding a New Resource

1. **Implement the resource** with good schema descriptions:
   ```go
   MarkdownDescription: "Clear description of what this field does",
   ```

2. **Create example files**:
   ```bash
   mkdir -p examples/resources/prism_new_resource
   touch examples/resources/prism_new_resource/resource.tf
   touch examples/resources/prism_new_resource/import.sh
   ```

3. **Write example code** in `resource.tf`:
   ```hcl
   resource "prism_new_resource" "example" {
     name = "example"
     # ... other fields
   }
   ```

4. **Write import example** in `import.sh`:
   ```bash
   # Description of import format
   terraform import prism_new_resource.example "resource-id"
   ```

5. **Generate documentation**:
   ```bash
   make docs
   ```

### Updating Existing Documentation

#### Option 1: Update Schema Descriptions (Recommended)

Edit the resource code to improve descriptions:
```go
"account_ids": schema.ListAttribute{
    ElementType:         types.StringType,
    Required:            true,
    MarkdownDescription: "List of AWS account IDs to grant access to. Each assignment creates a separate permission mapping.",
},
```

Then regenerate:
```bash
make docs
```

#### Option 2: Update Examples

Edit files in `examples/resources/<resource_name>/`:
- Improve `resource.tf` with better examples
- Add more import scenarios to `import.sh`

Then regenerate:
```bash
make docs
```

#### Option 3: Update Provider Index

Edit `templates/index.md.tmpl` for provider-level documentation changes.

### Best Practices

1. **Write Clear Descriptions**: Use markdown formatting in descriptions
   ```go
   MarkdownDescription: "The session duration in ISO 8601 format (e.g., `PT4H` for 4 hours, `PT12H` for 12 hours)",
   ```

2. **Keep Examples Simple**: Start with the simplest working example
   ```hcl
   # Good: Minimal working example
   resource "prism_user" "example" {
     username = "john.doe"
     email    = "john.doe@example.com"
     enabled  = true
   }
   ```

3. **Add Comments to Examples**: Explain non-obvious configurations
   ```hcl
   # Configure Google Identity Provider
   resource "prism_identity_provider" "google" {
     type = "google"
     # Note: alias is auto-generated as "google" by the backend
     display_name = "Sign in with Google"

     config = jsonencode({
       clientId     = "your-client-id"
       clientSecret = "your-client-secret"
       hostedDomain = "example.com"  # Optional: restrict to organization domain
     })
   }
   ```

4. **Document Breaking Changes**: When making breaking changes, update both:
   - Schema descriptions (what changed)
   - Examples (how to migrate)

## Generating Documentation

### Local Generation

```bash
# Generate all documentation
make docs

# Verify generated files
ls -la docs/
```

### During Release

Documentation is automatically included when you:
1. Commit generated docs to the repository:
   ```bash
   git add docs/
   git commit -m "docs: update provider documentation"
   ```

2. Create a release tag:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

The Terraform Registry will automatically pull the documentation from the `docs/` directory.

## Configuration

### .tfplugindocs.yml

This file configures the documentation generator:

```yaml
provider:
  name: prism

rendering:
  include_resource_subcategories: true

resources:
  - name: prism_aws_account
    subcategory: "AWS Resources"

  - name: prism_permission_set
    subcategory: "Permission Management"
  # ... more resources
```

**Subcategories** organize resources in the navigation menu on the Terraform Registry.

### main.go

The generate directive triggers documentation generation:

```go
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
```

This is executed by `go generate` or `make docs`.

## Troubleshooting

### Documentation Not Appearing on Registry

1. Ensure `docs/` directory is committed to the repository
2. Check that file structure matches:
   - `docs/index.md`
   - `docs/resources/*.md`
   - `docs/data-sources/*.md`
3. Verify the release tag is pushed to GitHub
4. Wait a few minutes for Registry to update

### Generation Errors

```bash
# Install/update terraform-plugin-docs
go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest

# Clean and regenerate
rm -rf docs/
make docs
```

### Broken Links in Examples

Ensure example file paths in templates are correct:
```
{{ tffile "examples/provider/provider.tf" }}
```

Path is relative to repository root.

## Publishing Checklist

Before publishing a new provider version:

- [ ] Update schema descriptions for any new/changed fields
- [ ] Add/update example files for new resources
- [ ] Run `make docs` to regenerate documentation
- [ ] Review generated docs in `docs/` directory
- [ ] Commit docs with descriptive message
- [ ] Tag release
- [ ] Push tag to GitHub
- [ ] Verify docs on Terraform Registry after ~5 minutes

## Resources

- [terraform-plugin-docs Documentation](https://github.com/hashicorp/terraform-plugin-docs)
- [Terraform Registry Publishing](https://www.terraform.io/registry/providers/publishing)
- [Provider Design Principles](https://www.terraform.io/plugin/best-practices/hashicorp-provider-design-principles)
