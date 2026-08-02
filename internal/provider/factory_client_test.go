package provider

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestCreateAccountViaFactory_ReturnsOperationID(t *testing.T) {
	c, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"operationId": "acf-poc-123", "status": "pending"})
	})
	opID, err := c.CreateAccountViaFactory(FactoryCreateInput{
		AccountName: "Acme", RootEmail: "root@x", AdminRoleName: "OrgAdminAccess", AdminTrustAccount: "857639285487",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opID != "acf-poc-123" {
		t.Fatalf("opID = %q, want acf-poc-123", opID)
	}
	if len(*got) != 1 || (*got)[0].Path != "/api/v1/customers/sso/accountfactory/create" {
		t.Fatalf("unexpected request path: %+v", *got)
	}
}

func TestCreateAccountViaFactory_FeatureNotEnabled(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "Org Services with a management account must be enabled to create accounts"})
	})
	_, err := c.CreateAccountViaFactory(FactoryCreateInput{AccountName: "Acme", RootEmail: "root@x", AdminRoleName: "R", AdminTrustAccount: "857639285487"})
	if !errors.Is(err, ErrFeatureNotEnabled) {
		t.Fatalf("err = %v, want ErrFeatureNotEnabled", err)
	}
}

func TestGetFactoryStatus_DecodesCompleted(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "completed", "progress": 100, "progressLabel": "done", "awsAccountId": "123456789012",
		})
	})
	st, err := c.GetFactoryStatus("acf-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Status != "completed" || st.AWSAccountID != "123456789012" {
		t.Fatalf("bad status decode: %+v", st)
	}
}
