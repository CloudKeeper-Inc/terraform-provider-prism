package provider

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestCustomerManagedPolicy_CRUDWire(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id": "cmp-1", "name": "n", "description": "d", "policy_document": "{}",
		})
	})
	out, err := c.CreateCustomerManagedPolicy(&CustomerManagedPolicy{Name: "n", Description: "d", PolicyDocument: "{}"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "cmp-1" {
		t.Fatalf("id not decoded: %+v", out)
	}
	if (*got)[0].Path != "/api/v1/customers/sso/customer-managed-policies" {
		t.Fatalf("wrong path: %s", (*got)[0].Path)
	}
	if (*got)[0].Body["policy_document"] != "{}" {
		t.Fatalf("policy_document not sent: %+v", (*got)[0].Body)
	}
}
