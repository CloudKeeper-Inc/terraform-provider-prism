package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// recordedRequest captures what the client actually put on the wire.
type recordedRequest struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

// newTestClient starts a TLS test server (the client forces an https:// prefix on
// BaseURL, so a plain http server would produce "https://http://...") and returns
// a Client pointed at it alongside the slice that records incoming requests.
func newTestClient(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) (*Client, *[]recordedRequest) {
	t.Helper()

	var got []recordedRequest

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{Method: r.Method, Path: r.URL.Path}
		if raw, err := io.ReadAll(r.Body); err == nil && len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.Body)
		}
		got = append(got, rec)
		respond(w, r)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "sso", "test-token")
	c.HTTPClient = srv.Client()

	return c, &got
}

// writeIdP responds with the standard {success, data} envelope wrapping a single
// identity provider, which is what the by-alias endpoints return.
func writeIdP(w http.ResponseWriter, alias, displayName string, enabled bool, config map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "ok",
		"data": map[string]interface{}{
			"identityProvider": map[string]interface{}{
				"alias":       alias,
				"displayName": displayName,
				"providerId":  "oidc",
				"enabled":     enabled,
				"config":      config,
			},
		},
	})
}

const wantBasePath = "/api/v1/customers/sso/identity-providers/custom"

func TestGetIdentityProviderUsesAliasPath(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeIdP(w, "okta", "Okta", true, map[string]string{
			"clientId": "0oa123",
			"tokenUrl": "https://example.okta.com/oauth2/default/v1/token",
		})
	})

	idp, err := c.GetIdentityProvider("custom", "okta")
	if err != nil {
		t.Fatalf("GetIdentityProvider returned error: %v", err)
	}

	if len(*got) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(*got))
	}
	req := (*got)[0]

	if want := wantBasePath + "/okta"; req.Path != want {
		t.Errorf("request path = %q, want %q", req.Path, want)
	}
	if req.Method != http.MethodGet {
		t.Errorf("request method = %q, want GET", req.Method)
	}

	// The collection path is the list endpoint and would decode to zero values,
	// so a populated response proves we hit the item route.
	if idp.Alias != "okta" {
		t.Errorf("alias = %q, want %q", idp.Alias, "okta")
	}
	if idp.DisplayName != "Okta" {
		t.Errorf("displayName = %q, want %q", idp.DisplayName, "Okta")
	}
	if !idp.Enabled {
		t.Error("enabled = false, want true")
	}
	if idp.Config["tokenUrl"] != "https://example.okta.com/oauth2/default/v1/token" {
		t.Errorf("config[tokenUrl] = %v, want the token endpoint", idp.Config["tokenUrl"])
	}
}

// A list-shaped response must not decode into a populated IdP. This is the exact
// failure the old collection-path read produced: no error, no data, no drift.
func TestGetIdentityProviderRejectsListShapedResponse(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"identityProviders": []map[string]interface{}{
					{"alias": "okta", "displayName": "Okta", "enabled": true},
				},
				"total": 1,
			},
		})
	})

	idp, err := c.GetIdentityProvider("custom", "okta")
	if err != nil {
		t.Fatalf("GetIdentityProvider returned error: %v", err)
	}
	if idp.Alias != "" || idp.DisplayName != "" {
		t.Fatalf("list-shaped response decoded to a populated IdP (%+v); "+
			"the by-alias route must be used", idp)
	}
}

func TestUpdateIdentityProviderUsesAliasPath(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeIdP(w, "okta", "Okta", true, nil)
	})

	idp := &IdentityProvider{
		Alias:       "okta",
		DisplayName: "Okta",
		Enabled:     true,
		Config: map[string]interface{}{
			"clientId":     "0oa123",
			"clientSecret": "shh",
			"issuer":       "https://example.okta.com/oauth2/default",
			"tokenUrl":     "https://example.okta.com/oauth2/default/v1/token",
		},
	}

	if _, err := c.UpdateIdentityProvider("custom", "okta", idp); err != nil {
		t.Fatalf("UpdateIdentityProvider returned error: %v", err)
	}

	if len(*got) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(*got))
	}
	req := (*got)[0]

	if want := wantBasePath + "/okta"; req.Path != want {
		t.Errorf("request path = %q, want %q", req.Path, want)
	}
	if req.Method != http.MethodPut {
		t.Errorf("request method = %q, want PUT", req.Method)
	}
	if req.Body["displayName"] != "Okta" {
		t.Errorf("body displayName = %v, want %q", req.Body["displayName"], "Okta")
	}
	if req.Body["tokenUrl"] != "https://example.okta.com/oauth2/default/v1/token" {
		t.Errorf("body tokenUrl = %v, want the token endpoint", req.Body["tokenUrl"])
	}
}

// An empty displayName would blank the IdP's name in Keycloak, because the
// backend PUT replaces the whole definition. Refuse before sending anything.
func TestUpdateIdentityProviderRejectsEmptyDisplayName(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeIdP(w, "okta", "", true, nil)
	})

	_, err := c.UpdateIdentityProvider("custom", "okta", &IdentityProvider{
		Alias:  "okta",
		Config: map[string]interface{}{"clientId": "0oa123"},
	})

	if err == nil {
		t.Fatal("expected an error for an empty displayName, got nil")
	}
	if !strings.Contains(err.Error(), "displayName is required") {
		t.Errorf("error = %q, want it to mention displayName is required", err)
	}
	if len(*got) != 0 {
		t.Errorf("expected no HTTP request to be sent, got %d", len(*got))
	}
}

func TestDeleteIdentityProviderUsesAliasPath(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "deleted",
		})
	})

	if err := c.DeleteIdentityProvider("custom", "okta"); err != nil {
		t.Fatalf("DeleteIdentityProvider returned error: %v", err)
	}

	if len(*got) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(*got))
	}
	req := (*got)[0]

	if want := wantBasePath + "/okta"; req.Path != want {
		t.Errorf("request path = %q, want %q", req.Path, want)
	}
	if req.Method != http.MethodDelete {
		t.Errorf("request method = %q, want DELETE", req.Method)
	}
}

// Deleting an IdP that is already gone must surface as a not-found error so
// Delete() can treat it as success rather than stranding the resource in state.
func TestDeleteIdentityProviderNotFoundIsDetectable(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Custom Identity Provider not found with alias gone",
		})
	})

	err := c.DeleteIdentityProvider("custom", "gone")
	if err == nil {
		t.Fatal("expected an error for a missing IdP, got nil")
	}
	if !isDependencyNotFoundError(err) {
		t.Errorf("error %q is not recognised as not-found; destroy would fail", err)
	}
}

// A 404 from the backend must surface as a not-found error so Read() can drop the
// resource from state rather than failing every subsequent plan.
func TestGetIdentityProviderNotFoundIsDetectable(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Custom Identity Provider not found with alias gone",
		})
	})

	_, err := c.GetIdentityProvider("custom", "gone")
	if err == nil {
		t.Fatal("expected an error for a missing IdP, got nil")
	}
	if !isDependencyNotFoundError(err) {
		t.Errorf("error %q is not recognised as not-found; Read() would not remove it from state", err)
	}
}

// nullStateFor builds an all-null state object for a resource schema, matching
// what the framework hands to ImportState.
func nullStateFor(ctx context.Context, t *testing.T, r resource.Resource) tfsdk.State {
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

	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(objType, attrs),
	}
}

func TestIdentityProviderImportState(t *testing.T) {
	ctx := context.Background()
	r := &IdentityProviderResource{}

	t.Run("valid", func(t *testing.T) {
		resp := &resource.ImportStateResponse{State: nullStateFor(ctx, t, r)}
		r.ImportState(ctx, resource.ImportStateRequest{ID: "custom:okta"}, resp)

		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}

		for attr, want := range map[string]string{
			"type":  "custom",
			"alias": "okta",
			"id":    "okta",
		} {
			var got string
			if diags := resp.State.GetAttribute(ctx, path.Root(attr), &got); diags.HasError() {
				t.Fatalf("reading %q: %v", attr, diags)
			}
			if got != want {
				t.Errorf("%s = %q, want %q", attr, got, want)
			}
		}
	})

	for _, id := range []string{"okta", "", ":okta", "custom:"} {
		t.Run("rejects "+id, func(t *testing.T) {
			resp := &resource.ImportStateResponse{State: nullStateFor(ctx, t, r)}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)

			if !resp.Diagnostics.HasError() {
				t.Errorf("import ID %q was accepted, want an error", id)
			}
		})
	}
}
