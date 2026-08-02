package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func s(v string) types.String { return types.StringValue(v) }

func TestValidateAccountMode_CreateRequiresInputs(t *testing.T) {
	issues := validateAccountMode(AWSAccountResourceModel{
		CreateNew: types.BoolValue(true),
	})
	paths := map[string]bool{}
	for _, is := range issues {
		paths[is.Attr] = true
	}
	for _, want := range []string{"root_email", "admin_role_name", "master_account"} {
		if !paths[want] {
			t.Fatalf("expected a missing-%s issue; got %+v", want, issues)
		}
	}
}

func TestValidateAccountMode_CreateRejectsBadMaster(t *testing.T) {
	issues := validateAccountMode(AWSAccountResourceModel{
		CreateNew: types.BoolValue(true), RootEmail: s("r@x"), AdminRoleName: s("R"), MasterAccount: s("notanid"),
	})
	if !hasIssue(issues, "master_account") {
		t.Fatalf("expected master_account regex issue; got %+v", issues)
	}
}

func TestValidateAccountMode_OnboardRequiresAccountID(t *testing.T) {
	issues := validateAccountMode(AWSAccountResourceModel{CreateNew: types.BoolValue(false)})
	if !hasIssue(issues, "account_id") {
		t.Fatalf("expected missing account_id; got %+v", issues)
	}
}

func TestValidateAccountMode_OnboardRejectsFactoryInputs(t *testing.T) {
	issues := validateAccountMode(AWSAccountResourceModel{
		CreateNew: types.BoolValue(false), AccountID: s("123456789012"), RootEmail: s("r@x"),
	})
	if !hasIssue(issues, "root_email") {
		t.Fatalf("expected unexpected-root_email; got %+v", issues)
	}
}

func TestValidateAccountMode_ValidCreate(t *testing.T) {
	issues := validateAccountMode(AWSAccountResourceModel{
		CreateNew: types.BoolValue(true), RootEmail: s("r@x"), AdminRoleName: s("R"), MasterAccount: s("857639285487"),
	})
	if len(issues) != 0 {
		t.Fatalf("valid create config produced issues: %+v", issues)
	}
}

func hasIssue(issues []modeIssue, attr string) bool {
	for _, is := range issues {
		if is.Attr == attr {
			return true
		}
	}
	return false
}
