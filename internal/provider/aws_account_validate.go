package provider

import "regexp"

var twelveDigit = regexp.MustCompile(`^\d{12}$`)

// modeIssue is one validation problem, keyed by attribute name.
type modeIssue struct {
	Attr    string
	Summary string
	Detail  string
}

func setStr(v interface {
	IsNull() bool
	ValueString() string
}) (string, bool) {
	if v.IsNull() {
		return "", false
	}
	s := v.ValueString()
	return s, s != ""
}

// validateAccountMode enforces exactly one coherent mode. Unknown create_new
// (interpolated) is treated as "skip" by the caller before invoking this.
func validateAccountMode(d AWSAccountResourceModel) []modeIssue {
	var issues []modeIssue
	_, hasRoot := setStr(d.RootEmail)
	_, hasAdmin := setStr(d.AdminRoleName)
	master, hasMaster := setStr(d.MasterAccount)
	_, hasAccountID := setStr(d.AccountID)

	if d.CreateNew.ValueBool() {
		if !hasRoot {
			issues = append(issues, modeIssue{"root_email", "Missing root_email", "root_email is required when create_new = true."})
		}
		if !hasAdmin {
			issues = append(issues, modeIssue{"admin_role_name", "Missing admin_role_name", "admin_role_name is required when create_new = true."})
		}
		if !hasMaster {
			issues = append(issues, modeIssue{"master_account", "Missing master_account", "master_account is required when create_new = true."})
		} else if !twelveDigit.MatchString(master) {
			issues = append(issues, modeIssue{"master_account", "Invalid master_account", "master_account must be a 12-digit AWS account ID."})
		}
		if hasAccountID {
			issues = append(issues, modeIssue{"account_id", "Unexpected account_id", "account_id must not be set when create_new = true; it is computed by the factory."})
		}
		return issues
	}

	if !hasAccountID {
		issues = append(issues, modeIssue{"account_id", "Missing account_id", "account_id is required when create_new = false (onboard mode)."})
	}
	if hasRoot {
		issues = append(issues, modeIssue{"root_email", "Unexpected root_email", "root_email is only valid when create_new = true."})
	}
	if hasAdmin {
		issues = append(issues, modeIssue{"admin_role_name", "Unexpected admin_role_name", "admin_role_name is only valid when create_new = true."})
	}
	if hasMaster {
		issues = append(issues, modeIssue{"master_account", "Unexpected master_account", "master_account is only valid when create_new = true."})
	}
	return issues
}
