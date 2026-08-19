package provider

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The prism-main handler POST /accounts/onboard answers HTTP 200 with
// success:true regardless of whether the account actually onboarded. When
// onboarding fails, data.account carries an AccountOnboardingError
// ({"account":{...},"error":"..."}) instead of an AWSAccountResponse, and when
// it is skipped it carries the bare SSOAccount echo. Both shapes must be
// rejected by the client rather than silently yielding a blank id.

const onboardFailedBody = `{
  "success": true,
  "message": "AWS account onboarded successfully for customer",
  "data": {
    "account": {
      "account": {"id":"","account_id":"696856632577","name":"core-prod","status":"","idp_arn":"","idp_name":"KeyCloakProvider","idp_status":"","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"},
      "error": "failed to create/update IdP: failed to get account credentials: failed to assume role in target account: failed to assume role: AccessDenied"
    },
    "totalAccounts": 1,
    "successfulCount": 0,
    "failedCount": 1,
    "skippedCount": 0
  }
}`

const onboardSkippedBody = `{
  "success": true,
  "message": "AWS account onboarded successfully for customer",
  "data": {
    "account": {"id":"","account_id":"696856632577","name":"core-prod","status":"","idp_arn":"","idp_name":"KeyCloakProvider","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"},
    "totalAccounts": 1,
    "successfulCount": 0,
    "failedCount": 0,
    "skippedCount": 1
  }
}`

func TestCreateAWSAccount_OnboardFailure_MustError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(onboardFailedBody))
	})

	got, err := c.CreateAWSAccount(&AWSAccount{AccountID: "696856632577", AccountName: "core-prod"})
	if err == nil {
		t.Fatalf("expected an error for a failed onboard, got nil with result %+v", got)
	}
	if !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("error should surface the server-side reason, got: %v", err)
	}
}

func TestCreateAWSAccount_OnboardSkipped_MustError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(onboardSkippedBody))
	})

	got, err := c.CreateAWSAccount(&AWSAccount{AccountID: "696856632577", AccountName: "core-prod"})
	if err == nil {
		t.Fatalf("expected an error for a skipped onboard, got nil with result %+v", got)
	}
}

const onboardSucceededBody = `{
  "success": true,
  "message": "AWS account onboarded successfully for customer",
  "data": {
    "account": {"id":"a1b2c3d4","account_id":"696856632577","name":"core-prod","status":"active","owner_emails":["ops@example.com"],"idp_name":"KeyCloakProvider"},
    "totalAccounts": 1,
    "successfulCount": 1,
    "failedCount": 0,
    "skippedCount": 0
  }
}`

// The success path must be untouched by the failure detection above.
func TestCreateAWSAccount_OnboardSuccess(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(onboardSucceededBody))
	})

	acct, err := c.CreateAWSAccount(&AWSAccount{
		AccountID:   "696856632577",
		AccountName: "core-prod",
		OwnerEmails: []string{"ops@example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.ID != "a1b2c3d4" || acct.AccountID != "696856632577" || acct.AccountName != "core-prod" {
		t.Fatalf("unexpected account: %+v", acct)
	}
	if len(acct.OwnerEmails) != 1 || acct.OwnerEmails[0] != "ops@example.com" {
		t.Fatalf("unexpected owner emails: %+v", acct.OwnerEmails)
	}
	if len(*got) != 1 || (*got)[0].Path != "/api/v1/customers/sso/accounts/onboard" {
		t.Fatalf("unexpected requests on the wire: %+v", *got)
	}
	if (*got)[0].Body["accountId"] != "696856632577" || (*got)[0].Body["accountName"] != "core-prod" {
		t.Fatalf("unexpected request body: %+v", (*got)[0].Body)
	}
}

// A backend fixed to answer non-2xx must still be reported as a failure.
func TestCreateAWSAccount_OnboardHTTPError_MustError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"error":"failed to assume role: AccessDenied"}`))
	})

	if _, err := c.CreateAWSAccount(&AWSAccount{AccountID: "696856632577", AccountName: "core-prod"}); err == nil {
		t.Fatal("expected an error for a non-2xx onboard response, got nil")
	} else if !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("error should surface the server-side reason, got: %v", err)
	}
}

// An AWSAccountResponse carries an `error` field of its own (the row-level
// error column in prism-main). A response with a real account id must be
// treated as the success it is, however that field is populated -- keying the
// failure decision on `error` rather than on the nested `account` object would
// fail an apply for an account that actually onboarded.
const onboardSucceededWithRowErrorBody = `{
  "success": true,
  "message": "AWS account onboarded successfully for customer",
  "data": {
    "account": {"id":"a1b2c3d4","account_id":"696856632577","name":"core-prod","status":"active","error":"last sync failed: throttled","owner_emails":["ops@example.com"]},
    "totalAccounts": 1,
    "successfulCount": 1,
    "failedCount": 0,
    "skippedCount": 0
  }
}`

// The failure shape is recognised by its nested account object, so it is caught
// even when the reason is missing.
const onboardFailedNoReasonBody = `{
  "success": true,
  "message": "AWS account onboarded successfully for customer",
  "data": {
    "account": {
      "account": {"id":"","account_id":"696856632577","name":"core-prod","status":""},
      "error": ""
    },
    "totalAccounts": 1,
    "successfulCount": 0,
    "failedCount": 1,
    "skippedCount": 0
  }
}`

func TestCreateAWSAccount_SuccessWithRowError_IsNotAFailure(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(onboardSucceededWithRowErrorBody))
	})

	acct, err := c.CreateAWSAccount(&AWSAccount{AccountID: "696856632577", AccountName: "core-prod"})
	if err != nil {
		t.Fatalf("a success shape carrying a row-level error must not be reported as a failed onboard, got: %v", err)
	}
	if acct.ID != "a1b2c3d4" || acct.AccountID != "696856632577" {
		t.Fatalf("unexpected account: %+v", acct)
	}
}

func TestCreateAWSAccount_OnboardFailureWithoutReason_MustError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(onboardFailedNoReasonBody))
	})

	got, err := c.CreateAWSAccount(&AWSAccount{AccountID: "696856632577", AccountName: "core-prod"})
	if err == nil {
		t.Fatalf("the failure shape must be caught by its structure even with no reason, got nil with result %+v", got)
	}
	if !strings.Contains(err.Error(), "696856632577") {
		t.Fatalf("error should name the account, got: %v", err)
	}
}

// The nested account can be null rather than an object, which the structural
// discriminator correctly declines -- but the server's reason is still right
// there and must not be traded for the generic "no account record id".
const onboardFailedNullAccountBody = `{
  "success": true,
  "message": "AWS account onboarded successfully for customer",
  "data": {
    "account": {"account": null, "error": "assume role failed"},
    "totalAccounts": 1,
    "successfulCount": 0,
    "failedCount": 1,
    "skippedCount": 0
  }
}`

func TestCreateAWSAccount_NullNestedAccount_KeepsServerReason(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(onboardFailedNullAccountBody))
	})

	got, err := c.CreateAWSAccount(&AWSAccount{AccountID: "696856632577", AccountName: "core-prod"})
	if err == nil {
		t.Fatalf("expected an error, got nil with result %+v", got)
	}
	if !strings.Contains(err.Error(), "assume role failed") {
		t.Fatalf("the server-side reason must survive, got: %v", err)
	}
}

func TestIsJSONObject(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"object", `{"a":1}`, true},
		{"empty object", `{}`, true},
		{"leading whitespace", "  \n\t{\"a\":1}", true},
		{"null", `null`, false},
		{"absent", ``, false},
		{"string", `"{}"`, false},
		{"array", `[{"a":1}]`, false},
		{"number", `0`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isJSONObject(json.RawMessage(tc.raw)); got != tc.want {
				t.Fatalf("isJSONObject(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
