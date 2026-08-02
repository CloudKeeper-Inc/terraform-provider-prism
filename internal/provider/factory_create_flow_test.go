package provider

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestFactoryFlow_CreateThenStatusThenAccount(t *testing.T) {
	var statusCalls int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/customers/sso/accountfactory/create":
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"operationId": "acf-1", "status": "pending"})
		case r.URL.Path == "/api/v1/customers/sso/accountfactory/create/acf-1/status":
			if atomic.AddInt32(&statusCalls, 1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "running", "progress": 40})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "completed", "awsAccountId": "123456789012"})
			}
		case r.URL.Path == "/api/v1/customers/sso/aws-accounts/123456789012":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{
				"account_id": "123456789012", "name": "Acme", "role_arn": "arn:aws:iam::123456789012:role/CloudKeeper-SSO-Role",
			}})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	})

	opID, err := c.CreateAccountViaFactory(FactoryCreateInput{AccountName: "Acme", RootEmail: "r@x", AdminRoleName: "R", AdminTrustAccount: "857639285487"})
	if err != nil || opID != "acf-1" {
		t.Fatalf("create: opID=%q err=%v", opID, err)
	}
	var accountID string
	for i := 0; i < 5; i++ {
		st, err := c.GetFactoryStatus(opID)
		if err != nil {
			t.Fatalf("status err: %v", err)
		}
		if st.Status == "completed" {
			accountID = st.AWSAccountID
			break
		}
	}
	if accountID != "123456789012" {
		t.Fatalf("accountID = %q", accountID)
	}
	acct, err := c.GetAWSAccount(accountID)
	if err != nil || acct.AccountID != "123456789012" {
		t.Fatalf("get account: %+v err=%v", acct, err)
	}
}
