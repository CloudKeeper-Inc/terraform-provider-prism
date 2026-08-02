package provider

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestPermissionSet_SendsRelayStateAndCMPIDs(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true, "message": "ok",
			"data": map[string]interface{}{
				"id": "ps-1", "name": "PS", "relay_state": "https://x",
				"customer_managed_policies": []map[string]string{{"id": "cmp-1", "name": "n", "description": "d"}},
			},
		})
	})
	out, err := c.CreatePermissionSet(&PermissionSet{
		Name: "PS", RelayState: "https://x", CustomerManagedPolicyIDs: []string{"cmp-1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := (*got)[0].Body
	if body["relay_state"] != "https://x" {
		t.Fatalf("relay_state not sent: %+v", body)
	}
	if ids, ok := body["customer_managed_policy_ids"].([]interface{}); !ok || len(ids) != 1 || ids[0] != "cmp-1" {
		t.Fatalf("customer_managed_policy_ids not sent: %+v", body)
	}
	if out.RelayState != "https://x" {
		t.Fatalf("relay_state not decoded: %+v", out)
	}
	if len(out.CustomerManagedPolicies) != 1 || out.CustomerManagedPolicies[0].ID != "cmp-1" {
		t.Fatalf("customer_managed_policies not decoded: %+v", out)
	}
}
