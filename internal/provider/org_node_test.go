package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

const orgNodePath = "/api/v1/customers/sso/org/node"

// The OU shape: name, parent and child accounts, all resolved directly.
const orgNodeOUBody = `{
  "id": "ou-1111-aaaaaaaa",
  "name": "Sandbox",
  "type": "OU",
  "parent_id": "r-root",
  "accounts": ["111111111111", "222222222222"]
}`

// The account shape: no accounts key, parent resolved in a single AWS call.
const orgNodeAccountBody = `{
  "id": "111111111111",
  "name": "sandbox-1",
  "type": "ACCOUNT",
  "parent_id": "ou-1111-aaaaaaaa"
}`

// What prism-main answers when its handler ran and the node is genuinely gone:
// the structured code, plus the same code prefixed onto the error string.
const prismNotFoundBody = `{"success":false,"code":"ORG_NODE_NOT_FOUND","message":"ORG_NODE_NOT_FOUND: organizational unit not found","error":"ORG_NODE_NOT_FOUND: organizational unit ou-1111-aaaaaaaa not found"}`

// The same answer through a prism-main that does not forward `code` yet: only
// prism-ops's belt-and-braces prefix survives.
const prismNotFoundPrefixOnlyBody = `{"success":false,"message":"ORG_NODE_NOT_FOUND: organizational unit not found","error":"ORG_NODE_NOT_FOUND: organizational unit ou-1111-aaaaaaaa not found"}`

// A 404 with neither signal: a deployment predating the codes. Absence cannot
// be confirmed, so this must never remove anything.
const prismNotFoundNoSignalBody = `{"success":false,"message":"org node not found","error":"NoSuchOrganizationalUnit"}`

const prismForbiddenBody = `{"success":false,"code":"ORG_NODE_FORBIDDEN","message":"forbidden","error":"ORG_NODE_FORBIDDEN: node outside tenant subtree"}`

const prismInvalidIDBody = `{"success":false,"code":"ORG_NODE_INVALID_ID","message":"invalid id","error":"ORG_NODE_INVALID_ID: \"not-an-ou\" is not an organization node id"}`

// What a deployment WITHOUT the route answers: gorilla/mux finds no match,
// applies no middleware, and falls through to http.NotFoundHandler.
const routerNotFoundBody = "404 page not found\n"

func writeStatus(w http.ResponseWriter, status int, contentType, body string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// readOUState builds the state Terraform hands to Read: the OU's id, plus
// whatever the practitioner had in accounts.
func readOUState(ctx context.Context, t *testing.T, r resource.Resource, id string, accounts []string) tfsdk.State {
	t.Helper()
	st := nullStateFor(ctx, t, r)
	if diags := st.SetAttribute(ctx, path.Root("id"), id); diags.HasError() {
		t.Fatalf("setting id: %v", diags)
	}
	if accounts != nil {
		if diags := st.SetAttribute(ctx, path.Root("accounts"), accounts); diags.HasError() {
			t.Fatalf("setting accounts: %v", diags)
		}
	}
	return st
}

func TestOrgOUReadUsesOrgNode(t *testing.T) {
	ctx := context.Background()
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeStatus(w, http.StatusOK, "application/json", orgNodeOUBody)
	})
	r := &OrgOUResource{client: c}

	resp := &resource.ReadResponse{State: readOUState(ctx, t, r, "ou-1111-aaaaaaaa", nil)}
	r.Read(ctx, resource.ReadRequest{State: readOUState(ctx, t, r, "ou-1111-aaaaaaaa", nil)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("state was removed on a successful read")
	}

	// The cost is the point: one node, one call, no traversal.
	if len(*got) != 1 {
		t.Fatalf("expected exactly 1 request, got %d: %+v", len(*got), *got)
	}
	if (*got)[0].Path != orgNodePath {
		t.Fatalf("expected the read to hit %s, got %s", orgNodePath, (*got)[0].Path)
	}
	if (*got)[0].Body["node_id"] != "ou-1111-aaaaaaaa" {
		t.Fatalf("unexpected request body: %+v", (*got)[0].Body)
	}

	var name, parentID string
	if diags := resp.State.GetAttribute(ctx, path.Root("name"), &name); diags.HasError() {
		t.Fatalf("reading name: %v", diags)
	}
	if diags := resp.State.GetAttribute(ctx, path.Root("parent_id"), &parentID); diags.HasError() {
		t.Fatalf("reading parent_id: %v", diags)
	}
	var accounts []string
	if diags := resp.State.GetAttribute(ctx, path.Root("accounts"), &accounts); diags.HasError() {
		t.Fatalf("reading accounts: %v", diags)
	}

	if name != "Sandbox" {
		t.Errorf("name = %q, want Sandbox", name)
	}
	if parentID != "r-root" {
		t.Errorf("parent_id = %q, want r-root", parentID)
	}
	if strings.Join(accounts, ",") != "111111111111,222222222222" {
		t.Errorf("accounts = %v, want [111111111111 222222222222]", accounts)
	}
}

// TestOrgOUReadRemovalRules is the destructive-risk table. Removal requires
// positive evidence -- the structured code or the prefix -- and a bare 404 is
// never enough. Every row names the mistake it is there to prevent.
func TestOrgOUReadRemovalRules(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantRemoved bool
		wantErrText string
	}{
		{
			name:        "structured code confirms the OU is gone, so drift is handled",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        prismNotFoundBody,
			wantRemoved: true,
		},
		{
			name:        "prefix without code still confirms it, so an older prism-main still handles drift",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        prismNotFoundPrefixOnlyBody,
			wantRemoved: true,
		},
		{
			name:        "404 with neither signal proves nothing, so a live OU is not dropped",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        prismNotFoundNoSignalBody,
			wantRemoved: false,
			wantErrText: "without an ORG_NODE_* code",
		},
		{
			name:        "ORG_NODE_FORBIDDEN is a permissions problem, so state survives it",
			status:      http.StatusForbidden,
			contentType: "application/json",
			body:        prismForbiddenBody,
			wantRemoved: false,
			wantErrText: "outside",
		},
		{
			name:        "ORG_NODE_INVALID_ID is a config mistake, so state survives it",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        prismInvalidIDBody,
			wantRemoved: false,
			wantErrText: "not a valid organization node id",
		},
		{
			// An older self-hosted prism-main has no /org/node route at all.
			name:        "router 404 means the endpoint is missing, so no OU is dropped on an old deployment",
			status:      http.StatusNotFound,
			contentType: "text/plain; charset=utf-8",
			body:        routerNotFoundBody,
			wantRemoved: false,
			wantErrText: "does not support POST /org/node",
		},
		{
			// New prism-main, prism-ops not yet deployed: the router 404 arrives
			// re-wrapped in prism-main's envelope.
			name:        "re-wrapped prism-ops 404 is not the OU's absence, so nothing is dropped mid-release",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        `{"success":false,"message":"prism-ops returned status 404: 404 page not found","error":"prism-ops returned status 404: 404 page not found"}`,
			wantRemoved: false,
			wantErrText: "does not support POST /org/node",
		},
		{
			name:        "403 without a code says nothing about existence",
			status:      http.StatusForbidden,
			contentType: "application/json",
			body:        `{"success":false,"message":"forbidden"}`,
			wantRemoved: false,
			wantErrText: "403",
		},
		{
			name:        "502 says nothing about the OU",
			status:      http.StatusBadGateway,
			contentType: "application/json",
			body:        `{"success":false,"message":"upstream unavailable"}`,
			wantRemoved: false,
			wantErrText: "502",
		},
		{
			name:        "405 means the route is not there for POST",
			status:      http.StatusMethodNotAllowed,
			contentType: "text/plain; charset=utf-8",
			body:        "Method Not Allowed\n",
			wantRemoved: false,
			wantErrText: "does not support POST /org/node",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				writeStatus(w, tc.status, tc.contentType, tc.body)
			})
			r := &OrgOUResource{client: c}

			resp := &resource.ReadResponse{State: readOUState(ctx, t, r, "ou-1111-aaaaaaaa", nil)}
			r.Read(ctx, resource.ReadRequest{State: readOUState(ctx, t, r, "ou-1111-aaaaaaaa", nil)}, resp)

			removed := resp.State.Raw.IsNull()
			if removed != tc.wantRemoved {
				t.Fatalf("resource removed from state = %v, want %v (diagnostics: %v)", removed, tc.wantRemoved, resp.Diagnostics)
			}
			if tc.wantErrText == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
				}
				return
			}
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error diagnostic, got none")
			}
			if !strings.Contains(resp.Diagnostics.Errors().Errors()[0].Detail(), tc.wantErrText) {
				t.Fatalf("diagnostic should mention %q, got: %v", tc.wantErrText, resp.Diagnostics)
			}
		})
	}
}

func TestOrgOUReadEmptiesAccountsWhenOUHasNone(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeStatus(w, http.StatusOK, "application/json",
			`{"id":"ou-1111-aaaaaaaa","name":"Sandbox","type":"OU","parent_id":"r-root"}`)
	})
	r := &OrgOUResource{client: c}

	state := readOUState(ctx, t, r, "ou-1111-aaaaaaaa", []string{"111111111111"})
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var accounts []string
	if diags := resp.State.GetAttribute(ctx, path.Root("accounts"), &accounts); diags.HasError() {
		t.Fatalf("reading accounts: %v", diags)
	}
	if len(accounts) != 0 {
		t.Fatalf("accounts = %v, want empty", accounts)
	}
}

func TestFindAccountParentThreeStatesInOneCall(t *testing.T) {
	t.Run("account found", func(t *testing.T) {
		c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeStatus(w, http.StatusOK, "application/json", orgNodeAccountBody)
		})
		r := &OrgOUResource{client: c}

		parent, err := r.findAccountParent("111111111111")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parent != "ou-1111-aaaaaaaa" {
			t.Fatalf("parent = %q, want ou-1111-aaaaaaaa", parent)
		}
		if len(*got) != 1 || (*got)[0].Path != orgNodePath {
			t.Fatalf("expected exactly 1 request to %s, got %+v", orgNodePath, *got)
		}
	})

	t.Run("account genuinely absent", func(t *testing.T) {
		c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeStatus(w, http.StatusNotFound, "application/json", prismNotFoundBody)
		})
		r := &OrgOUResource{client: c}

		parent, err := r.findAccountParent("333333333333")
		if err != nil {
			t.Fatalf("an absent account is not a lookup failure, got: %v", err)
		}
		if parent != "" {
			t.Fatalf("parent = %q, want empty", parent)
		}
		if len(*got) != 1 {
			t.Fatalf("expected exactly 1 request, got %d", len(*got))
		}
	})

	t.Run("lookup failed", func(t *testing.T) {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeStatus(w, http.StatusGatewayTimeout, "application/json", `{"success":false,"message":"timeout"}`)
		})
		r := &OrgOUResource{client: c}

		parent, err := r.findAccountParent("111111111111")
		if err == nil {
			t.Fatalf("a failed lookup must not look like an absent account (got parent %q)", parent)
		}
		if !strings.Contains(err.Error(), "504") {
			t.Fatalf("error should carry the underlying failure, got: %v", err)
		}
	})
}

func TestClassifyOrgNodeError(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		body            string
		wantNotFound    bool
		wantForbidden   bool
		wantInvalidID   bool
		wantUnsupported bool
	}{
		{name: "structured not-found code", status: http.StatusNotFound, body: prismNotFoundBody, wantNotFound: true},
		{name: "prefix only, no code", status: http.StatusNotFound, body: prismNotFoundPrefixOnlyBody, wantNotFound: true},
		{name: "404 with neither signal", status: http.StatusNotFound, body: prismNotFoundNoSignalBody},
		{name: "forbidden code", status: http.StatusForbidden, body: prismForbiddenBody, wantForbidden: true},
		{name: "invalid id code", status: http.StatusBadRequest, body: prismInvalidIDBody, wantInvalidID: true},
		{name: "unknown code falls through", status: http.StatusNotFound, body: `{"success":false,"code":"SOMETHING_ELSE","error":"?"}`},
		{name: "code mid-string is not evidence", status: http.StatusNotFound, body: `{"success":false,"error":"wrapped: ORG_NODE_NOT_FOUND: gone"}`},
		{name: "router 404", status: http.StatusNotFound, body: routerNotFoundBody, wantUnsupported: true},
		{name: "html 404 from an ingress", status: http.StatusNotFound, body: "<html><body>404</body></html>", wantUnsupported: true},
		{name: "405", status: http.StatusMethodNotAllowed, body: "Method Not Allowed\n", wantUnsupported: true},
		{
			name: "relayed prism-ops router 404", status: http.StatusNotFound,
			body:            `{"success":false,"message":"prism-ops returned status 404: 404 page not found","error":"prism-ops returned status 404: 404 page not found"}`,
			wantUnsupported: true,
		},
		{name: "403 without a code", status: http.StatusForbidden, body: `{"success":false,"error":"forbidden"}`},
		{name: "500", status: http.StatusInternalServerError, body: `{"success":false}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyOrgNodeError(&APIError{StatusCode: tc.status, Body: tc.body})
			for _, check := range []struct {
				name string
				got  bool
				want bool
			}{
				{"ErrOrgNodeNotFound", errors.Is(err, ErrOrgNodeNotFound), tc.wantNotFound},
				{"ErrOrgNodeForbidden", errors.Is(err, ErrOrgNodeForbidden), tc.wantForbidden},
				{"ErrOrgNodeInvalidID", errors.Is(err, ErrOrgNodeInvalidID), tc.wantInvalidID},
				{"ErrOrgNodeUnsupported", errors.Is(err, ErrOrgNodeUnsupported), tc.wantUnsupported},
			} {
				if check.got != check.want {
					t.Errorf("%s = %v, want %v (err: %v)", check.name, check.got, check.want, err)
				}
			}
		})
	}
}
