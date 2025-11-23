package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/CloudKeeper-Inc/terraform-provider-prism/internal/provider"
)

type Config struct {
	PrismSubdomain string
	APIToken       string
	OutputDir      string
}

type InfrastructureData struct {
	AWSAccounts              []provider.AWSAccount
	PermissionSets           []provider.PermissionSet
	Users                    []provider.User
	Groups                   []provider.Group
	GroupMemberships         map[string][]string // group name -> usernames
	PermissionSetAssignments []provider.PermissionSetAssignment
}

type Variables struct {
	AccountIDs     map[string]string // account_id -> variable name
	PermissionSets map[string]string // permission set id -> variable name
	Users          map[string]string // username -> variable name
	Groups         map[string]string // group name -> variable name
}

func main() {
	config := parseFlags()

	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("🔍 Connecting to Prism API...")
	client := provider.NewClient(
		fmt.Sprintf("https://%s.prism.cloudkeeper.com", config.PrismSubdomain),
		config.PrismSubdomain,
		config.APIToken,
	)

	fmt.Println("📦 Fetching infrastructure data...")
	data, err := fetchAllData(client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching data: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("🔢 Analyzing and extracting variables...")
	variables := extractVariables(data)

	fmt.Println("📝 Generating Terraform files...")
	if err := generateFiles(config.OutputDir, data, variables); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating files: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Successfully generated Terraform configuration!")
	fmt.Printf("\n📁 Output directory: %s\n", config.OutputDir)
	fmt.Println("\n📋 Generated files:")
	fmt.Println("  - provider.tf        (provider configuration)")
	fmt.Println("  - variables.tf       (variable definitions)")
	fmt.Println("  - terraform.tfvars   (variable values)")
	fmt.Println("  - aws_accounts.tf    (AWS account resources)")
	fmt.Println("  - permission_sets.tf (permission set resources)")
	fmt.Println("  - users.tf           (user resources)")
	fmt.Println("  - groups.tf          (group and membership resources)")
	fmt.Println("  - assignments.tf     (permission set assignments)")
	fmt.Println("  - import.sh          (import commands script)")
	fmt.Println("\n🚀 Next steps:")
	fmt.Println("  1. cd", config.OutputDir)
	fmt.Println("  2. Review the generated files")
	fmt.Println("  3. Run: chmod +x import.sh")
	fmt.Println("  4. Run: terraform init")
	fmt.Println("  5. Run: ./import.sh")
	fmt.Println("  6. Run: terraform plan")
}

func parseFlags() Config {
	var config Config

	flag.StringVar(&config.PrismSubdomain, "subdomain", os.Getenv("PRISM_SUBDOMAIN"), "Prism subdomain (or set PRISM_SUBDOMAIN env var)")
	flag.StringVar(&config.APIToken, "token", os.Getenv("PRISM_API_TOKEN"), "API token (or set PRISM_API_TOKEN env var)")
	flag.StringVar(&config.OutputDir, "output", "./generated-terraform", "Output directory for generated files")
	flag.Parse()

	if config.PrismSubdomain == "" {
		fmt.Fprintf(os.Stderr, "Error: Prism subdomain is required (use -subdomain flag or PRISM_SUBDOMAIN env var)\n")
		os.Exit(1)
	}

	if config.APIToken == "" {
		fmt.Fprintf(os.Stderr, "Error: API token is required (use -token flag or PRISM_API_TOKEN env var)\n")
		os.Exit(1)
	}

	return config
}

func fetchAllData(client *provider.Client) (*InfrastructureData, error) {
	data := &InfrastructureData{
		GroupMemberships: make(map[string][]string),
	}

	// Fetch AWS Accounts
	fmt.Println("  → Fetching AWS accounts...")
	accounts, err := client.ListAWSAccounts()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch AWS accounts: %w", err)
	}
	data.AWSAccounts = accounts
	fmt.Printf("    Found %d AWS accounts\n", len(accounts))

	// Fetch Permission Sets
	fmt.Println("  → Fetching permission sets...")
	permSets, err := client.ListPermissionSets()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch permission sets: %w", err)
	}
	data.PermissionSets = permSets
	fmt.Printf("    Found %d permission sets\n", len(permSets))

	// Fetch Users
	fmt.Println("  → Fetching users...")
	users, err := client.ListUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch users: %w", err)
	}
	data.Users = users
	fmt.Printf("    Found %d users\n", len(users))

	// Fetch Groups
	fmt.Println("  → Fetching groups...")
	groups, err := client.ListGroups()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch groups: %w", err)
	}
	data.Groups = groups
	fmt.Printf("    Found %d groups\n", len(groups))

	// Fetch Group Memberships
	fmt.Println("  → Fetching group memberships...")
	for _, group := range groups {
		members, err := client.GetGroupMembers(group.Name)
		if err != nil {
			fmt.Printf("    Warning: failed to fetch members for group %s: %v\n", group.Name, err)
			continue
		}
		if len(members) > 0 {
			data.GroupMemberships[group.Name] = members
		}
	}
	fmt.Printf("    Found memberships for %d groups\n", len(data.GroupMemberships))

	// Fetch Permission Set Assignments
	fmt.Println("  → Fetching permission set assignments...")
	assignments, err := client.ListPermissionSetAssignments()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch permission set assignments: %w", err)
	}
	data.PermissionSetAssignments = assignments
	fmt.Printf("    Found %d permission set assignments\n", len(assignments))

	return data, nil
}

func extractVariables(data *InfrastructureData) *Variables {
	vars := &Variables{
		AccountIDs:     make(map[string]string),
		PermissionSets: make(map[string]string),
		Users:          make(map[string]string),
		Groups:         make(map[string]string),
	}

	// Extract AWS account IDs that appear multiple times
	accountUsage := make(map[string]int)
	for _, assignment := range data.PermissionSetAssignments {
		accountUsage[assignment.AccountID]++
	}

	for accountID, count := range accountUsage {
		if count > 1 {
			// Find the account name
			for _, acc := range data.AWSAccounts {
				if acc.AccountID == accountID {
					varName := toVarName(acc.AccountName) + "_account_id"
					vars.AccountIDs[accountID] = varName
					break
				}
			}
		}
	}

	return vars
}

func toVarName(s string) string {
	// Convert to snake_case and remove special characters
	reg := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	s = reg.ReplaceAllString(s, "_")
	s = strings.ToLower(s)
	s = strings.Trim(s, "_")
	return s
}

func toResourceName(s string) string {
	// Convert to valid Terraform resource name
	reg := regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	s = reg.ReplaceAllString(s, "_")
	s = strings.ToLower(s)
	s = strings.Trim(s, "_")
	return s
}

func generateFiles(outputDir string, data *InfrastructureData, variables *Variables) error {
	// Generate provider.tf
	if err := generateProviderFile(outputDir); err != nil {
		return err
	}

	// Generate variables.tf
	if err := generateVariablesFile(outputDir, variables); err != nil {
		return err
	}

	// Generate terraform.tfvars
	if err := generateTFVarsFile(outputDir, data, variables); err != nil {
		return err
	}

	// Generate AWS accounts
	if err := generateAWSAccountsFile(outputDir, data.AWSAccounts); err != nil {
		return err
	}

	// Generate permission sets
	if err := generatePermissionSetsFile(outputDir, data.PermissionSets); err != nil {
		return err
	}

	// Generate users
	if err := generateUsersFile(outputDir, data.Users); err != nil {
		return err
	}

	// Generate groups
	if err := generateGroupsFile(outputDir, data.Groups, data.GroupMemberships); err != nil {
		return err
	}

	// Generate permission set assignments
	if err := generateAssignmentsFile(outputDir, data); err != nil {
		return err
	}

	// Generate import script
	if err := generateImportScript(outputDir, data); err != nil {
		return err
	}

	return nil
}

func generateProviderFile(outputDir string) error {
	content := `terraform {
  required_version = ">= 1.0"

  required_providers {
    prism = {
      source = "CloudKeeper-Inc/prism"
    }
  }
}

provider "prism" {
  prism_subdomain = var.prism_subdomain
  api_token       = var.prism_api_token
}
`
	return os.WriteFile(filepath.Join(outputDir, "provider.tf"), []byte(content), 0644)
}

func generateVariablesFile(outputDir string, variables *Variables) error {
	var sb strings.Builder

	sb.WriteString("# Provider Configuration Variables\n\n")
	sb.WriteString("variable \"prism_subdomain\" {\n")
	sb.WriteString("  type        = string\n")
	sb.WriteString("  description = \"Prism subdomain\"\n")
	sb.WriteString("  sensitive   = false\n")
	sb.WriteString("}\n\n")

	sb.WriteString("variable \"prism_api_token\" {\n")
	sb.WriteString("  type        = string\n")
	sb.WriteString("  description = \"Prism API token\"\n")
	sb.WriteString("  sensitive   = true\n")
	sb.WriteString("}\n")

	// Add account ID variables if any
	if len(variables.AccountIDs) > 0 {
		sb.WriteString("\n# AWS Account ID Variables\n")

		// Sort for consistent output
		var accountIDs []string
		for accountID := range variables.AccountIDs {
			accountIDs = append(accountIDs, accountID)
		}
		sort.Strings(accountIDs)

		for _, accountID := range accountIDs {
			varName := variables.AccountIDs[accountID]
			sb.WriteString(fmt.Sprintf("\nvariable \"%s\" {\n", varName))
			sb.WriteString("  type        = string\n")
			sb.WriteString(fmt.Sprintf("  description = \"AWS Account ID (%s)\"\n", accountID))
			sb.WriteString("}\n")
		}
	}

	return os.WriteFile(filepath.Join(outputDir, "variables.tf"), []byte(sb.String()), 0644)
}

func generateTFVarsFile(outputDir string, data *InfrastructureData, variables *Variables) error {
	var sb strings.Builder

	sb.WriteString("# Provider Configuration\n")
	sb.WriteString("prism_subdomain = \"YOUR_SUBDOMAIN_HERE\"\n")
	sb.WriteString("prism_api_token = \"YOUR_API_TOKEN_HERE\"\n")

	if len(variables.AccountIDs) > 0 {
		sb.WriteString("\n# AWS Account IDs\n")

		var accountIDs []string
		for accountID := range variables.AccountIDs {
			accountIDs = append(accountIDs, accountID)
		}
		sort.Strings(accountIDs)

		for _, accountID := range accountIDs {
			varName := variables.AccountIDs[accountID]
			sb.WriteString(fmt.Sprintf("%s = \"%s\"\n", varName, accountID))
		}
	}

	return os.WriteFile(filepath.Join(outputDir, "terraform.tfvars"), []byte(sb.String()), 0644)
}

func generateAWSAccountsFile(outputDir string, accounts []provider.AWSAccount) error {
	if len(accounts) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# AWS Accounts\n\n")

	for _, acc := range accounts {
		resourceName := toResourceName(acc.AccountName)
		sb.WriteString(fmt.Sprintf("resource \"prism_aws_account\" \"%s\" {\n", resourceName))
		sb.WriteString(fmt.Sprintf("  account_id   = \"%s\"\n", acc.AccountID))
		sb.WriteString(fmt.Sprintf("  account_name = \"%s\"\n", escapeString(acc.AccountName)))
		if acc.Region != "" {
			sb.WriteString(fmt.Sprintf("  region       = \"%s\"\n", acc.Region))
		}
		sb.WriteString("}\n\n")
	}

	return os.WriteFile(filepath.Join(outputDir, "aws_accounts.tf"), []byte(sb.String()), 0644)
}

func generatePermissionSetsFile(outputDir string, permSets []provider.PermissionSet) error {
	if len(permSets) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# Permission Sets\n\n")

	for _, ps := range permSets {
		resourceName := toResourceName(ps.Name)
		sb.WriteString(fmt.Sprintf("resource \"prism_permission_set\" \"%s\" {\n", resourceName))
		sb.WriteString(fmt.Sprintf("  name        = \"%s\"\n", escapeString(ps.Name)))

		if ps.Description != "" {
			sb.WriteString(fmt.Sprintf("  description = \"%s\"\n", escapeString(ps.Description)))
		}

		if ps.SessionDuration != "" {
			sb.WriteString(fmt.Sprintf("  session_duration = \"%s\"\n", ps.SessionDuration))
		}

		if len(ps.ManagedPolicies) > 0 {
			sb.WriteString("\n  managed_policies = [\n")
			for _, policy := range ps.ManagedPolicies {
				sb.WriteString(fmt.Sprintf("    \"%s\",\n", policy))
			}
			sb.WriteString("  ]\n")
		}

		if len(ps.InlinePolicies) > 0 {
			sb.WriteString("\n  inline_policies = {\n")
			for name, policy := range ps.InlinePolicies {
				// Pretty print JSON
				var policyObj interface{}
				if err := json.Unmarshal([]byte(policy), &policyObj); err == nil {
					prettyJSON, _ := json.MarshalIndent(policyObj, "    ", "  ")
					sb.WriteString(fmt.Sprintf("    %s = <<-EOT\n%s\nEOT\n", name, indent(string(prettyJSON), 4)))
				} else {
					sb.WriteString(fmt.Sprintf("    %s = %q\n", name, policy))
				}
			}
			sb.WriteString("  }\n")
		}

		sb.WriteString("}\n\n")
	}

	return os.WriteFile(filepath.Join(outputDir, "permission_sets.tf"), []byte(sb.String()), 0644)
}

func generateUsersFile(outputDir string, users []provider.User) error {
	if len(users) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# Users\n\n")

	for _, user := range users {
		resourceName := toResourceName(user.Username)
		sb.WriteString(fmt.Sprintf("resource \"prism_user\" \"%s\" {\n", resourceName))
		sb.WriteString(fmt.Sprintf("  username   = \"%s\"\n", escapeString(user.Username)))
		sb.WriteString(fmt.Sprintf("  email      = \"%s\"\n", escapeString(user.Email)))

		if user.FirstName != "" {
			sb.WriteString(fmt.Sprintf("  first_name = \"%s\"\n", escapeString(user.FirstName)))
		}

		if user.LastName != "" {
			sb.WriteString(fmt.Sprintf("  last_name  = \"%s\"\n", escapeString(user.LastName)))
		}

		sb.WriteString(fmt.Sprintf("  enabled    = %t\n", user.Enabled))

		if len(user.Attributes) > 0 {
			sb.WriteString("\n  attributes = {\n")
			// Sort keys for consistent output
			var keys []string
			for k := range user.Attributes {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, k := range keys {
				values := user.Attributes[k]
				if len(values) > 0 {
					sb.WriteString(fmt.Sprintf("    %s = \"%s\"\n", k, escapeString(values[0])))
				}
			}
			sb.WriteString("  }\n")
		}

		sb.WriteString("}\n\n")
	}

	return os.WriteFile(filepath.Join(outputDir, "users.tf"), []byte(sb.String()), 0644)
}

func generateGroupsFile(outputDir string, groups []provider.Group, memberships map[string][]string) error {
	if len(groups) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# Groups\n\n")

	for _, group := range groups {
		resourceName := toResourceName(group.Name)
		sb.WriteString(fmt.Sprintf("resource \"prism_group\" \"%s\" {\n", resourceName))
		sb.WriteString(fmt.Sprintf("  name        = \"%s\"\n", escapeString(group.Name)))

		if group.Description != "" {
			sb.WriteString(fmt.Sprintf("  description = \"%s\"\n", escapeString(group.Description)))
		}

		if group.Path != "" {
			sb.WriteString(fmt.Sprintf("  path        = \"%s\"\n", escapeString(group.Path)))
		}

		sb.WriteString("}\n\n")
	}

	// Group memberships
	if len(memberships) > 0 {
		sb.WriteString("# Group Memberships\n\n")

		for groupName, members := range memberships {
			if len(members) == 0 {
				continue
			}

			resourceName := toResourceName(groupName) + "_members"
			groupResourceName := toResourceName(groupName)

			sb.WriteString(fmt.Sprintf("resource \"prism_group_membership\" \"%s\" {\n", resourceName))
			sb.WriteString(fmt.Sprintf("  group_name = prism_group.%s.name\n", groupResourceName))
			sb.WriteString("  usernames  = [\n")

			for _, member := range members {
				userResourceName := toResourceName(member)
				sb.WriteString(fmt.Sprintf("    prism_user.%s.username,\n", userResourceName))
			}

			sb.WriteString("  ]\n")
			sb.WriteString("}\n\n")
		}
	}

	return os.WriteFile(filepath.Join(outputDir, "groups.tf"), []byte(sb.String()), 0644)
}

func generateAssignmentsFile(outputDir string, data *InfrastructureData) error {
	if len(data.PermissionSetAssignments) == 0 {
		return nil
	}

	// Group assignments by permission set + principal
	type assignmentKey struct {
		PermissionSetID string
		PrincipalType   string
		PrincipalID     string
	}

	grouped := make(map[assignmentKey][]string)

	for _, assignment := range data.PermissionSetAssignments {
		principalID := assignment.Username
		if assignment.PrincipalType == "GROUP" {
			principalID = assignment.GroupName
		}

		key := assignmentKey{
			PermissionSetID: assignment.PermissionSetID,
			PrincipalType:   assignment.PrincipalType,
			PrincipalID:     principalID,
		}

		grouped[key] = append(grouped[key], assignment.AccountID)
	}

	var sb strings.Builder
	sb.WriteString("# Permission Set Assignments\n\n")

	counter := 0
	for key, accountIDs := range grouped {
		counter++

		// Find permission set name
		permSetName := ""
		for _, ps := range data.PermissionSets {
			if ps.ID == key.PermissionSetID {
				permSetName = ps.Name
				break
			}
		}

		resourceName := fmt.Sprintf("assignment_%d", counter)
		if permSetName != "" && key.PrincipalID != "" {
			resourceName = toResourceName(permSetName + "_" + key.PrincipalID)
		}

		sb.WriteString(fmt.Sprintf("resource \"prism_permission_set_assignment\" \"%s\" {\n", resourceName))

		// Find permission set resource
		permSetResourceName := toResourceName(permSetName)
		sb.WriteString(fmt.Sprintf("  permission_set_id = prism_permission_set.%s.id\n", permSetResourceName))
		sb.WriteString(fmt.Sprintf("  principal_type    = \"%s\"\n", key.PrincipalType))

		if key.PrincipalType == "USER" {
			userResourceName := toResourceName(key.PrincipalID)
			sb.WriteString(fmt.Sprintf("  principal_id      = prism_user.%s.username\n", userResourceName))
		} else {
			groupResourceName := toResourceName(key.PrincipalID)
			sb.WriteString(fmt.Sprintf("  principal_id      = prism_group.%s.name\n", groupResourceName))
		}

		sb.WriteString("  account_ids       = [\n")
		for _, accountID := range accountIDs {
			// Find account resource name
			accountResourceName := ""
			for _, acc := range data.AWSAccounts {
				if acc.AccountID == accountID {
					accountResourceName = toResourceName(acc.AccountName)
					break
				}
			}
			if accountResourceName != "" {
				sb.WriteString(fmt.Sprintf("    prism_aws_account.%s.account_id,\n", accountResourceName))
			} else {
				sb.WriteString(fmt.Sprintf("    \"%s\",\n", accountID))
			}
		}
		sb.WriteString("  ]\n")
		sb.WriteString("}\n\n")
	}

	return os.WriteFile(filepath.Join(outputDir, "assignments.tf"), []byte(sb.String()), 0644)
}

func generateImportScript(outputDir string, data *InfrastructureData) error {
	var sb strings.Builder

	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("# Terraform import script - generated automatically\n")
	sb.WriteString("# This script imports existing resources into Terraform state\n\n")
	sb.WriteString("set -e\n\n")
	sb.WriteString("echo \"Starting Terraform import process...\"\n\n")

	// Import AWS accounts
	if len(data.AWSAccounts) > 0 {
		sb.WriteString("# Import AWS Accounts\n")
		sb.WriteString("echo \"Importing AWS accounts...\"\n")
		for _, acc := range data.AWSAccounts {
			resourceName := toResourceName(acc.AccountName)
			sb.WriteString(fmt.Sprintf("terraform import prism_aws_account.%s %s\n", resourceName, acc.AccountID))
		}
		sb.WriteString("\n")
	}

	// Import permission sets
	if len(data.PermissionSets) > 0 {
		sb.WriteString("# Import Permission Sets\n")
		sb.WriteString("echo \"Importing permission sets...\"\n")
		for _, ps := range data.PermissionSets {
			resourceName := toResourceName(ps.Name)
			sb.WriteString(fmt.Sprintf("terraform import prism_permission_set.%s %s\n", resourceName, ps.ID))
		}
		sb.WriteString("\n")
	}

	// Import users
	if len(data.Users) > 0 {
		sb.WriteString("# Import Users\n")
		sb.WriteString("echo \"Importing users...\"\n")
		for _, user := range data.Users {
			resourceName := toResourceName(user.Username)
			sb.WriteString(fmt.Sprintf("terraform import prism_user.%s %s\n", resourceName, user.ID))
		}
		sb.WriteString("\n")
	}

	// Import groups
	if len(data.Groups) > 0 {
		sb.WriteString("# Import Groups\n")
		sb.WriteString("echo \"Importing groups...\"\n")
		for _, group := range data.Groups {
			resourceName := toResourceName(group.Name)
			sb.WriteString(fmt.Sprintf("terraform import prism_group.%s %s\n", resourceName, group.ID))
		}
		sb.WriteString("\n")
	}

	// Import group memberships
	groupsWithMembers := 0
	for _, members := range data.GroupMemberships {
		if len(members) > 0 {
			groupsWithMembers++
		}
	}
	if groupsWithMembers > 0 {
		sb.WriteString("# Import Group Memberships\n")
		sb.WriteString("echo \"Importing group memberships...\"\n")
		for groupName, members := range data.GroupMemberships {
			if len(members) == 0 {
				continue
			}
			resourceName := toResourceName(groupName) + "_members"
			sb.WriteString(fmt.Sprintf("terraform import prism_group_membership.%s %s\n", resourceName, groupName))
		}
		sb.WriteString("\n")
	}

	// Import permission set assignments
	if len(data.PermissionSetAssignments) > 0 {
		sb.WriteString("# Import Permission Set Assignments\n")
		sb.WriteString("echo \"Importing permission set assignments...\"\n")

		// Group assignments by permission set + principal to match Terraform resources
		type assignmentKey struct {
			PermissionSetID string
			PrincipalType   string
			PrincipalID     string
		}

		type assignmentGroup struct {
			AccountIDs    []string
			AssignmentIDs []string
		}

		grouped := make(map[assignmentKey]*assignmentGroup)

		for _, assignment := range data.PermissionSetAssignments {
			principalID := assignment.Username
			if assignment.PrincipalType == "GROUP" {
				principalID = assignment.GroupName
			}

			key := assignmentKey{
				PermissionSetID: assignment.PermissionSetID,
				PrincipalType:   assignment.PrincipalType,
				PrincipalID:     principalID,
			}

			if grouped[key] == nil {
				grouped[key] = &assignmentGroup{}
			}
			grouped[key].AccountIDs = append(grouped[key].AccountIDs, assignment.AccountID)
			grouped[key].AssignmentIDs = append(grouped[key].AssignmentIDs, assignment.ID)
		}

		counter := 0
		for key, group := range grouped {
			counter++

			// Find permission set name
			permSetName := ""
			for _, ps := range data.PermissionSets {
				if ps.ID == key.PermissionSetID {
					permSetName = ps.Name
					break
				}
			}

			resourceName := fmt.Sprintf("assignment_%d", counter)
			if permSetName != "" && key.PrincipalID != "" {
				resourceName = toResourceName(permSetName + "_" + key.PrincipalID)
			}

			// Create composite ID from actual assignment IDs (new format)
			compositeID := strings.Join(group.AssignmentIDs, ",")

			sb.WriteString(fmt.Sprintf("terraform import prism_permission_set_assignment.%s '%s'\n", resourceName, compositeID))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("echo \"✅ Import complete!\"\n")
	sb.WriteString("echo \"Next steps:\"\n")
	sb.WriteString("echo \"  1. Run: terraform plan\"\n")
	sb.WriteString("echo \"  2. Review any differences\"\n")
	sb.WriteString("echo \"  3. Run: terraform apply (if needed)\"\n")

	return os.WriteFile(filepath.Join(outputDir, "import.sh"), []byte(sb.String()), 0755)
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func indent(s string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}
