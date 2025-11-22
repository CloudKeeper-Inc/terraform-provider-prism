# Terraform Import Tool for Prism

This tool automatically generates Terraform configuration from your existing Prism infrastructure. It fetches all resources from your Prism instance and creates properly formatted `.tf` files along with import scripts.

## Features

- ✅ **Fetches all resources**: AWS accounts, permission sets, users, groups, and assignments
- ✅ **Multiple organized files**: Separate files for each resource type
- ✅ **Variable extraction**: Automatically identifies and extracts repeated values into variables
- ✅ **Import script generation**: Creates a ready-to-use bash script for importing resources
- ✅ **Proper references**: Uses Terraform references (e.g., `prism_user.john.username`) instead of hardcoded values

## Usage

### Option 1: Using Make (Recommended)

From the repository root:

```bash
make generate-terraform SUBDOMAIN=your-subdomain TOKEN=your-api-token
```

Or with a custom output directory:

```bash
make generate-terraform SUBDOMAIN=your-subdomain TOKEN=your-api-token OUTPUT=./my-terraform
```

Or using environment variables:

```bash
export PRISM_SUBDOMAIN=your-subdomain
export PRISM_API_TOKEN=your-api-token
make generate-terraform
```

### Option 2: Building and Running Directly

```bash
# Build the tool
cd tools/terraform-import
go build -o terraform-import .

# Run it
./terraform-import -subdomain your-subdomain -token your-api-token -output ./generated
```

## Generated Files

The tool creates the following files in the output directory:

| File | Description |
|------|-------------|
| `provider.tf` | Provider configuration |
| `variables.tf` | Variable definitions |
| `terraform.tfvars` | Variable values (you'll need to fill in credentials) |
| `aws_accounts.tf` | AWS account resources |
| `permission_sets.tf` | Permission set resources with inline and managed policies |
| `users.tf` | User resources with attributes |
| `groups.tf` | Group resources and group memberships |
| `assignments.tf` | Permission set assignments (grouped by permission set + principal) |
| `import.sh` | Executable bash script to import all resources |

## Example Workflow

1. **Generate Terraform code from existing infrastructure:**
   ```bash
   make generate-terraform SUBDOMAIN=mycompany TOKEN=my-token
   ```

2. **Navigate to generated directory:**
   ```bash
   cd generated-terraform
   ```

3. **Review and customize the generated files:**
   - Edit `terraform.tfvars` to add your actual credentials
   - Review resource configurations for any needed adjustments

4. **Initialize Terraform:**
   ```bash
   terraform init
   ```

5. **Import existing resources:**
   ```bash
   chmod +x import.sh
   ./import.sh
   ```

6. **Verify the import:**
   ```bash
   terraform plan
   ```

   You should see "No changes" if everything was imported correctly.

7. **Start managing with Terraform:**
   ```bash
   # Make changes to your .tf files
   terraform plan
   terraform apply
   ```

## How It Works

### Resource Fetching

The tool connects to your Prism API and fetches:
- All AWS accounts
- All permission sets (with managed and inline policies)
- All users (with attributes)
- All groups (with descriptions and paths)
- Group memberships for each group
- All permission set assignments

### Smart Grouping

Permission set assignments are automatically grouped by:
- Permission set
- Principal (user or group)

Multiple account assignments for the same permission set + principal combination are combined into a single resource with multiple `account_ids`.

### Variable Extraction

The tool identifies values that appear multiple times (like AWS account IDs used in multiple assignments) and extracts them into variables for easier maintenance.

### Import ID Generation

For permission set assignments, the tool generates composite import IDs in the format:
```
permissionSetId:principalType:principalId:accountId1,accountId2,...
```

## Troubleshooting

### "No such file or directory" error
Make sure you're running from the repository root when using `make generate-terraform`.

### "API error" messages
- Verify your API token is valid
- Check that your subdomain is correct
- Ensure you have network connectivity to the Prism API

### Import fails
- Make sure you run `terraform init` before running the import script
- Check that the provider is installed correctly
- Verify all resource names are valid (no special characters)

### "Resource already in state" error
If a resource is already imported, you can skip it by commenting out that line in `import.sh`.

## Advanced Options

### Custom Output Directory

```bash
make generate-terraform SUBDOMAIN=mycompany TOKEN=mytoken OUTPUT=/path/to/output
```

### Using the Binary Directly

After building with `make build-import-tool`, the binary is available at `bin/terraform-import`:

```bash
./bin/terraform-import -subdomain mycompany -token mytoken -output ./custom-dir
```

## Contributing

This tool is part of the terraform-provider-prism repository. If you find issues or want to add features:

1. Check existing issues
2. Create a new issue describing the problem/feature
3. Submit a pull request with your changes

## License

This tool is licensed under the same license as terraform-provider-prism (Mozilla Public License 2.0).
