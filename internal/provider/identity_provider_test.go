package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

	for _, tc := range []struct {
		name     string
		importID string
		want     map[string]string
	}{
		{
			name:     "valid custom",
			importID: "custom:okta",
			want:     map[string]string{"type": "custom", "alias": "okta", "id": "okta"},
		},
		{
			// SAML aliases are backend-generated and carry the saml- prefix, so
			// the alias half of the import ID contains the type as a substring.
			name:     "valid saml",
			importID: "saml:saml-entra-id",
			want:     map[string]string{"type": "saml", "alias": "saml-entra-id", "id": "saml-entra-id"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &resource.ImportStateResponse{State: nullStateFor(ctx, t, r)}
			r.ImportState(ctx, resource.ImportStateRequest{ID: tc.importID}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}

			for attr, want := range tc.want {
				var got string
				if diags := resp.State.GetAttribute(ctx, path.Root(attr), &got); diags.HasError() {
					t.Fatalf("reading %q: %v", attr, diags)
				}
				if got != want {
					t.Errorf("%s = %q, want %q", attr, got, want)
				}
			}
		})
	}

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

// ========== SAML 2.0 ==========

const wantSAMLBasePath = "/api/v1/customers/sso/identity-providers/saml"

// Every key listed in samlStringKeys, samlBoolKeys and samlIntKeys must reach the
// top level of the create body. The expectations are built from the slices
// themselves, so adding a key to a slice without hoisting it fails here - there
// is no separate list to forget to update.
func TestCreateSAMLIdentityProviderHoistsEveryConfigKey(t *testing.T) {
	config := make(map[string]interface{})
	want := make(map[string]interface{})

	for _, key := range samlStringKeys {
		config[key] = "value-for-" + key
		want[key] = "value-for-" + key
	}
	for _, key := range samlBoolKeys {
		config[key] = true
		want[key] = true // decoded from JSON true, not from "true"
	}
	for _, key := range samlIntKeys {
		config[key] = float64(45)
		want[key] = float64(45) // encoding/json decodes every number as float64
	}

	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeIdP(w, "saml-contoso-entra-id", "Contoso Entra", true, nil)
	})

	// Enabled is false on the resource while the config carries enabled = true,
	// so a hoisted value proves the config key is honoured.
	_, err := c.CreateIdentityProvider("saml", &IdentityProvider{
		DisplayName: "Contoso Entra",
		Enabled:     false,
		Config:      config,
	})
	if err != nil {
		t.Fatalf("CreateIdentityProvider returned error: %v", err)
	}

	if len(*got) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(*got))
	}
	req := (*got)[0]

	if req.Path != wantSAMLBasePath {
		t.Errorf("request path = %q, want %q", req.Path, wantSAMLBasePath)
	}
	if req.Method != http.MethodPost {
		t.Errorf("request method = %q, want POST", req.Method)
	}
	if _, nested := req.Body["config"]; nested {
		t.Error("request body carries a nested config object; the backend reads these keys at the top level")
	}

	for key, wantValue := range want {
		gotValue, ok := req.Body[key]
		if !ok {
			t.Errorf("config key %q never reached the request body; it is listed in a saml*Keys slice but not hoisted", key)
			continue
		}
		if gotValue != wantValue {
			t.Errorf("body[%q] = %#v (%T), want %#v (%T)", key, gotValue, gotValue, wantValue, wantValue)
		}
	}
}

// A boolean written the natural way - jsonencode({ validateSignature = true }) -
// must arrive as JSON true. The .(string) assertion the other provider types use
// would drop it silently, leaving signature validation off on a provider that
// applied cleanly.
func TestCreateSAMLIdentityProviderSendsRealBooleans(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeIdP(w, "saml-contoso-entra-id", "Contoso Entra", true, nil)
	})

	_, err := c.CreateIdentityProvider("saml", &IdentityProvider{
		DisplayName: "Contoso Entra",
		Enabled:     true,
		Config: map[string]interface{}{
			"singleSignOnServiceUrl": "https://login.microsoftonline.com/tid/saml2",
			"validateSignature":      true,    // real HCL boolean
			"wantAssertionsSigned":   "true",  // quoted, must be coerced
			"forceAuthn":             "false", // quoted, must be coerced
		},
	})
	if err != nil {
		t.Fatalf("CreateIdentityProvider returned error: %v", err)
	}

	req := (*got)[0]

	for key, want := range map[string]bool{
		"validateSignature":    true,
		"wantAssertionsSigned": true,
		"forceAuthn":           false,
	} {
		got, ok := req.Body[key].(bool)
		if !ok {
			t.Errorf("body[%q] = %#v (%T), want a JSON boolean", key, req.Body[key], req.Body[key])
			continue
		}
		if got != want {
			t.Errorf("body[%q] = %v, want %v", key, got, want)
		}
	}
}

// Anything that is neither a boolean nor "true"/"false" is a configuration error
// worth failing on, and it must fail before the request goes out.
func TestCreateSAMLIdentityProviderRejectsNonBoolean(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeIdP(w, "saml-contoso-entra-id", "Contoso Entra", true, nil)
	})

	_, err := c.CreateIdentityProvider("saml", &IdentityProvider{
		DisplayName: "Contoso Entra",
		Enabled:     true,
		Config: map[string]interface{}{
			"singleSignOnServiceUrl": "https://login.microsoftonline.com/tid/saml2",
			"validateSignature":      "yes",
		},
	})

	if err == nil {
		t.Fatal("expected an error for validateSignature = \"yes\", got nil")
	}
	if !strings.Contains(err.Error(), "validateSignature") {
		t.Errorf("error = %q, want it to name the offending key", err)
	}
	if len(*got) != 0 {
		t.Errorf("expected no HTTP request to be sent, got %d", len(*got))
	}
}

// An empty signingCertificate means "leave the stored certificate alone". Sending
// "" would clear it in Keycloak while validateSignature stayed true, and every
// login would fail.
func TestSAMLIdentityProviderOmitsEmptySigningCertificate(t *testing.T) {
	config := map[string]interface{}{
		"idpEntityId":            "https://sts.windows.net/tid/",
		"singleSignOnServiceUrl": "https://login.microsoftonline.com/tid/saml2",
		"signingCertificate":     "",
	}

	t.Run("create", func(t *testing.T) {
		c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeIdP(w, "saml-contoso-entra-id", "Contoso Entra", true, nil)
		})

		if _, err := c.CreateIdentityProvider("saml", &IdentityProvider{
			DisplayName: "Contoso Entra",
			Enabled:     true,
			Config:      config,
		}); err != nil {
			t.Fatalf("CreateIdentityProvider returned error: %v", err)
		}

		if _, present := (*got)[0].Body["signingCertificate"]; present {
			t.Errorf("body carries signingCertificate = %#v; an empty certificate must be absent entirely",
				(*got)[0].Body["signingCertificate"])
		}
	})

	t.Run("update", func(t *testing.T) {
		c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			writeIdP(w, "saml-contoso-entra-id", "Contoso Entra", true, nil)
		})

		if _, err := c.UpdateIdentityProvider("saml", "saml-contoso-entra-id", &IdentityProvider{
			Alias:       "saml-contoso-entra-id",
			DisplayName: "Contoso Entra",
			Enabled:     true,
			Config:      config,
		}); err != nil {
			t.Fatalf("UpdateIdentityProvider returned error: %v", err)
		}

		if _, present := (*got)[0].Body["signingCertificate"]; present {
			t.Errorf("body carries signingCertificate = %#v; an empty certificate must be absent entirely",
				(*got)[0].Body["signingCertificate"])
		}
	})
}

func TestUpdateSAMLIdentityProviderUsesAliasPath(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeIdP(w, "saml-contoso-entra-id", "Contoso Entra", true, nil)
	})

	_, err := c.UpdateIdentityProvider("saml", "saml-contoso-entra-id", &IdentityProvider{
		Alias:       "saml-contoso-entra-id",
		DisplayName: "Contoso Entra",
		Enabled:     true,
		Config: map[string]interface{}{
			"idpEntityId":            "https://sts.windows.net/tid/",
			"singleSignOnServiceUrl": "https://login.microsoftonline.com/tid/saml2",
			"signingCertificate":     "MIIC8DCCAdigAwIBAgIQ",
			"nameIDPolicyFormat":     "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent",
			"allowedClockSkew":       float64(30),
			"validateSignature":      true,
		},
	})
	if err != nil {
		t.Fatalf("UpdateIdentityProvider returned error: %v", err)
	}

	if len(*got) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(*got))
	}
	req := (*got)[0]

	if want := wantSAMLBasePath + "/saml-contoso-entra-id"; req.Path != want {
		t.Errorf("request path = %q, want %q", req.Path, want)
	}
	if req.Method != http.MethodPut {
		t.Errorf("request method = %q, want PUT", req.Method)
	}
	if req.Body["displayName"] != "Contoso Entra" {
		t.Errorf("body displayName = %v, want %q", req.Body["displayName"], "Contoso Entra")
	}
	if req.Body["signingCertificate"] != "MIIC8DCCAdigAwIBAgIQ" {
		t.Errorf("body signingCertificate = %v, want the supplied certificate", req.Body["signingCertificate"])
	}
	// Capital ID: the key matches Keycloak's own config key, so a lowercase
	// nameIdPolicyFormat would be dropped by the backend with no error.
	if req.Body["nameIDPolicyFormat"] != "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent" {
		t.Errorf("body nameIDPolicyFormat = %v, want the persistent URN", req.Body["nameIDPolicyFormat"])
	}
	if req.Body["allowedClockSkew"] != float64(30) {
		t.Errorf("body allowedClockSkew = %#v, want 30", req.Body["allowedClockSkew"])
	}
}

// The type attribute's OneOf validator is the only thing stopping a typo from
// reaching the API as an unknown provider type.
func TestIdentityProviderTypeValidator(t *testing.T) {
	ctx := context.Background()
	r := &IdentityProviderResource{}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema returned diagnostics: %v", schemaResp.Diagnostics)
	}

	attr, ok := schemaResp.Schema.Attributes["type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("type attribute is %T, want schema.StringAttribute", schemaResp.Schema.Attributes["type"])
	}
	if len(attr.Validators) == 0 {
		t.Fatal("the type attribute has no validators; every string would be accepted")
	}

	validate := func(value string) diag.Diagnostics {
		var diags diag.Diagnostics
		for _, v := range attr.Validators {
			resp := &validator.StringResponse{}
			v.ValidateString(ctx, validator.StringRequest{
				Path:        path.Root("type"),
				ConfigValue: types.StringValue(value),
			}, resp)
			diags.Append(resp.Diagnostics...)
		}
		return diags
	}

	for _, accepted := range []string{"google", "microsoft", "keycloak", "custom", "saml"} {
		t.Run("accepts "+accepted, func(t *testing.T) {
			if diags := validate(accepted); diags.HasError() {
				t.Errorf("type = %q was rejected: %v", accepted, diags)
			}
		})
	}

	// The validator is case-sensitive and exact: "SAML" and "saml2" must not slip
	// through to the API as an unknown provider type.
	for _, rejected := range []string{"saml2", "SAML", "SAML2", "oidc", ""} {
		t.Run("rejects "+rejected, func(t *testing.T) {
			if diags := validate(rejected); !diags.HasError() {
				t.Errorf("type = %q was accepted, want a diagnostic", rejected)
			}
		})
	}
}
