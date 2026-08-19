package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const onboardEndpointPath = "/api/v1/customers/sso/accounts/onboard"

// A dry run creates nothing, so its account record has no id. The preflight must
// accept that; CreateAWSAccount must not.
const preflightOKBody = `{
  "success": true,
  "message": "Dry run completed - account would be onboarded successfully for customer",
  "data": {
    "account": {"id":"","account_id":"696856632577","name":"core-prod","status":"would_create","idp_status":"would_create"},
    "totalAccounts": 1,
    "successfulCount": 1,
    "failedCount": 0,
    "skippedCount": 0
  }
}`

const assumeRoleReason = "failed to assume role arn:aws:iam::696856632577:role/CloudKeeper-SSO-Role: AccessDenied"

// awsAccountPlanFor builds the plan the framework hands to Create.
func awsAccountPlanFor(ctx context.Context, t *testing.T, r resource.Resource, set map[string]interface{}) tfsdk.Plan {
	t.Helper()

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema returned diagnostics: %v", schemaResp.Diagnostics)
	}
	objType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	attrs := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		attrs[name] = tftypes.NewValue(attrType, nil)
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, attrs)}
	for name, value := range set {
		if diags := plan.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("setting %q: %v", name, diags)
		}
	}
	return plan
}

func createAWSAccount(ctx context.Context, t *testing.T, r *AWSAccountResource, set map[string]interface{}) *resource.CreateResponse {
	t.Helper()
	resp := &resource.CreateResponse{State: nullStateFor(ctx, t, r)}
	r.Create(ctx, resource.CreateRequest{Plan: awsAccountPlanFor(ctx, t, r, set)}, resp)
	return resp
}

// onboardCalls returns just the onboard requests, with whether each was a dry run.
func onboardCalls(got *[]recordedRequest) []bool {
	var dryRuns []bool
	for _, req := range *got {
		if req.Path == onboardEndpointPath {
			dryRuns = append(dryRuns, req.Body["dryRun"] == true)
		}
	}
	return dryRuns
}

func TestCreatePreflightPassesThenOnboards(t *testing.T) {
	ctx := context.Background()
	var calls int
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(preflightOKBody))
			return
		}
		_, _ = w.Write([]byte(onboardSucceededBody))
	})
	r := &AWSAccountResource{client: c}

	resp := createAWSAccount(ctx, t, r, map[string]interface{}{
		"account_id":   "696856632577",
		"account_name": "core-prod",
		"create_new":   false,
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	// Two onboard calls, dry run first, real onboard second.
	dryRuns := onboardCalls(got)
	if len(dryRuns) != 2 {
		t.Fatalf("expected 2 onboard calls, got %d (%+v)", len(dryRuns), *got)
	}
	if !dryRuns[0] {
		t.Error("the first onboard call must be the dry-run preflight")
	}
	if dryRuns[1] {
		t.Error("the second onboard call must be the real onboard, not a dry run")
	}

	var id string
	if diags := resp.State.GetAttribute(ctx, path.Root("id"), &id); diags.HasError() {
		t.Fatalf("reading id: %v", diags)
	}
	if id != "a1b2c3d4" {
		t.Fatalf("id = %q, want a1b2c3d4", id)
	}
}

// TestCreatePreflightFailureStopsBeforeOnboard is the point of the feature: a
// target account that cannot be assumed into is never onboarded at all.
func TestCreatePreflightFailureStopsBeforeOnboard(t *testing.T) {
	for _, tc := range []struct {
		name    string
		respond func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name: "probing prism-main answers non-2xx",
			respond: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"success":false,"message":"preflight failed","error":"` + assumeRoleReason + `"}`))
			},
		},
		{
			name: "deployment answers 200 with the failure shape",
			respond: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":{"account":{"account":{"account_id":"696856632577"},"error":"` +
					assumeRoleReason + `"},"totalAccounts":1,"successfulCount":0,"failedCount":1}}`))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c, got := newTestClient(t, tc.respond)
			r := &AWSAccountResource{client: c}

			resp := createAWSAccount(ctx, t, r, map[string]interface{}{
				"account_id":   "696856632577",
				"account_name": "core-prod",
				"create_new":   false,
			})

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected an error diagnostic for a failed preflight")
			}

			// Not attempting the onboard is the whole point.
			dryRuns := onboardCalls(got)
			if len(dryRuns) != 1 {
				t.Fatalf("the real onboard must not be attempted after a failed preflight: got %d onboard calls (%+v)", len(dryRuns), *got)
			}
			if !dryRuns[0] {
				t.Fatal("the single call should have been the dry-run preflight")
			}

			diag := resp.Diagnostics.Errors()[0]
			if !strings.Contains(diag.Summary(), crossAccountRoleName) {
				t.Errorf("summary should name the role, got: %q", diag.Summary())
			}
			if !strings.Contains(diag.Summary(), "696856632577") {
				t.Errorf("summary should name the account, got: %q", diag.Summary())
			}
			detail := diag.Detail()
			for _, want := range []string{assumeRoleReason, crossAccountRoleName, "trust policy", "onboarding was not attempted"} {
				if !strings.Contains(detail, want) {
					t.Errorf("detail should mention %q, got: %q", want, detail)
				}
			}
		})
	}
}

// The factory provisions the role as part of creating the account, so the
// create path must not spend a preflight call.
func TestCreateViaFactorySkipsPreflight(t *testing.T) {
	ctx := context.Background()
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/accountfactory/create"):
			_, _ = w.Write([]byte(`{"operationId":"op-1"}`))
		case strings.HasSuffix(r.URL.Path, "/status"):
			_, _ = w.Write([]byte(`{"status":"completed","progress":100,"awsAccountId":"222222222222"}`))
		default: // GET /aws-accounts/{id}
			_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":{"id":"222222222222","account_id":"222222222222","name":"new-acct"}}`))
		}
	})
	r := &AWSAccountResource{client: c}

	resp := createAWSAccount(ctx, t, r, map[string]interface{}{
		"account_name":    "new-acct",
		"create_new":      true,
		"root_email":      "root+new@example.com",
		"admin_role_name": "OrgAdmin",
		"master_account":  "999999999999",
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if n := len(onboardCalls(got)); n != 0 {
		t.Fatalf("the factory path must not call the onboard endpoint at all, got %d calls (%+v)", n, *got)
	}
}

// A dry run creates nothing, so the preflight must not demand an account id --
// the check CreateAWSAccount rightly enforces.
func TestPreflightAcceptsResponseWithoutAccountID(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(preflightOKBody))
	})

	if err := c.PreflightAWSAccount("696856632577", "core-prod"); err != nil {
		t.Fatalf("preflight rejected a valid dry-run response: %v", err)
	}
	if len(*got) != 1 || (*got)[0].Body["dryRun"] != true {
		t.Fatalf("expected one dry-run request, got %+v", *got)
	}
}
