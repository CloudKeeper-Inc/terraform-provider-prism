package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// APIError carries the HTTP status of a failed API call so callers can branch on
// it (e.g. 403 => feature not enabled). Error() keeps the previous fmt.Errorf
// format, so existing substring checks (e.g. "404") continue to work.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (%d): %s", e.StatusCode, e.Body)
}

// Global counter for API call sequence tracking
var apiCallCounter int64
var apiStartTime = time.Now()

// First request serialization mechanism
// Ensures the first API request completes before allowing parallel requests,
// giving the backend time to cache token details.
var (
	firstRequestOnce sync.Once
	firstRequestDone = make(chan struct{})
)

// signalFirstRequestComplete signals that the first request has completed.
// Safe to call multiple times (only first call has effect).
func signalFirstRequestComplete() {
	select {
	case <-firstRequestDone:
		// Already closed
	default:
		close(firstRequestDone)
	}
}

// Client is the CloudKeeper API client
type Client struct {
	BaseURL        string
	PrismSubdomain string
	HTTPClient     *http.Client
	Token          string
}

// NewClient creates a new CloudKeeper API client
func NewClient(baseURL, prismSubdomain, token string) *Client {
	return &Client{
		BaseURL:        baseURL,
		PrismSubdomain: prismSubdomain,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		Token: token,
	}
}

// doRequestRaw performs an HTTP request without customer path prefix
func (c *Client) doRequestRaw(method, path string, body interface{}) ([]byte, error) {
	// First request serialization - ensure first request completes before others proceed
	isFirst := false
	firstRequestOnce.Do(func() {
		isFirst = true
	})
	if !isFirst {
		// Wait for first request to complete
		<-firstRequestDone
	}
	// Signal completion when done (only effective for first request)
	defer signalFirstRequestComplete()

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Token", c.Token)

	callNum := atomic.AddInt64(&apiCallCounter, 1)
	sinceStart := time.Since(apiStartTime)
	startTime := time.Now()
	resp, err := c.HTTPClient.Do(req)
	elapsed := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "[API TIMING] #%d @%.2fs | %s %s | Response: %v\n", callNum, sinceStart.Seconds(), method, c.BaseURL+path, elapsed)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return respBody, nil
}

// doRequestCustomerRaw performs an HTTP request with customer path prefix but returns raw response (no unwrap).
// Used for endpoints that return raw JSON instead of the standard {success, data} wrapper.
func (c *Client) doRequestCustomerRaw(method, path string, body interface{}) ([]byte, error) {
	if !strings.HasPrefix(c.BaseURL, "https://") {
		c.BaseURL = "https://" + c.BaseURL
	}
	fullPath := fmt.Sprintf("/api/v1/customers/%s%s", c.PrismSubdomain, path)
	return c.doRequestRaw(method, fullPath, body)
}

// doRequest performs an HTTP request with customer path prefix and unwraps the API response
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	// First request serialization - ensure first request completes before others proceed
	isFirst := false
	firstRequestOnce.Do(func() {
		isFirst = true
	})
	if !isFirst {
		// Wait for first request to complete
		<-firstRequestDone
	}
	// Signal completion when done (only effective for first request)
	defer signalFirstRequestComplete()

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	if !strings.HasPrefix(c.BaseURL, "https://") {
		c.BaseURL = "https://" + c.BaseURL
	}
	url := fmt.Sprintf("%s/api/v1/customers/%s%s", c.BaseURL, c.PrismSubdomain, path)
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Token", c.Token)

	callNum := atomic.AddInt64(&apiCallCounter, 1)
	sinceStart := time.Since(apiStartTime)
	startTime := time.Now()
	resp, err := c.HTTPClient.Do(req)
	elapsed := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "[API TIMING] #%d @%.2fs | %s %s | Response: %v\n", callNum, sinceStart.Seconds(), method, url, elapsed)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	// Unwrap the API response to extract the data field
	data, err := unwrapAPIResponse(respBody)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// APIResponse represents the standard API response wrapper
type APIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// unwrapAPIResponse extracts the data field from an API response
func unwrapAPIResponse(body []byte) ([]byte, error) {
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API response: %w", err)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("API request failed: %s", apiResp.Error)
	}

	return apiResp.Data, nil
}

// ========== AWS Account Operations ==========

type AWSAccount struct {
	ID          string   `json:"id,omitempty"`
	CustomerID  string   `json:"customer_id,omitempty"`
	AccountID   string   `json:"account_id"`
	AccountName string   `json:"name"`
	OwnerEmails []string `json:"owner_emails,omitempty"`
}

// onboardData is the `data` object of POST /accounts/onboard.
//
// As of prism-main 2.0.0 the handler answered HTTP 200 with success:true
// whether the onboard succeeded, failed or was skipped, and only the *shape* of
// data.account told the three apart: an AWSAccountResponse on success, an
// AccountOnboardingError ({"account":{...},"error":"..."}) on failure, and the
// bare SSOAccount echo when the account was skipped. prism-main is being
// changed to answer non-2xx for the failure cases, and customers run older
// self-hosted versions either way, so this decoding handles both.
//
// Account is therefore kept raw and decoded in a second step -- decoding
// straight into the success shape silently yields a blank account for the other
// two, which is how empty ids used to reach state.
type onboardData struct {
	Account json.RawMessage `json:"account"`

	// Counters are pointers so a body that omits them is distinguishable from
	// one that reports zero; they are the only machine-readable hint about why
	// a 200 produced no account record.
	TotalAccounts   *int `json:"totalAccounts"`
	SuccessfulCount *int `json:"successfulCount"`
	FailedCount     *int `json:"failedCount"`
	SkippedCount    *int `json:"skippedCount"`
}

// countsSuffix renders the onboarding tallies as a parenthesised message
// fragment with a leading space, or "" when the response carried none.
func (d onboardData) countsSuffix() string {
	var parts []string
	for _, f := range []struct {
		name string
		val  *int
	}{
		{"total", d.TotalAccounts},
		{"successful", d.SuccessfulCount},
		{"failed", d.FailedCount},
		{"skipped", d.SkippedCount},
	} {
		if f.val != nil {
			parts = append(parts, fmt.Sprintf("%s=%d", f.name, *f.val))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// isJSONObject reports whether raw is a JSON object, as opposed to absent,
// null, or a scalar.
func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// onboardFailure is the AccountOnboardingError shape that data.account carries
// when an onboard failed: a nested account object plus the reason. Both the real
// onboard and the dry-run preflight have to recognise it, so it is decoded in
// one place.
type onboardFailure struct {
	Account json.RawMessage `json:"account"`
	Error   string          `json:"error"`
}

// decodeOnboardFailure reads the failure shape out of data.account. A body that
// is not this shape simply yields a zero value.
func decodeOnboardFailure(raw json.RawMessage) onboardFailure {
	var failure onboardFailure
	if err := json.Unmarshal(raw, &failure); err != nil {
		return onboardFailure{}
	}
	return failure
}

// isFailureShape reports whether this is an AccountOnboardingError, told apart
// from the success shape STRUCTURALLY -- by its nested `account` object, which
// an AWSAccountResponse never has.
//
// Keying on a non-empty `error` instead would be ambiguous: an
// AWSAccountResponse carries an `error` field of its own (the row-level error
// column), so a successful onboard that happened to surface a stale row error
// would be reported as a failure and the account would be left out of state
// despite existing.
func (f onboardFailure) isFailureShape() bool { return isJSONObject(f.Account) }

// reason is the server-side reason, verbatim, or "" when none was given.
func (f onboardFailure) reason() string { return strings.TrimSpace(f.Error) }

// PreflightAWSAccount probes whether an account can be onboarded, without
// onboarding it: POST /accounts/onboard with dryRun:true, which assumes the
// cross-account role and stops there.
//
// This is a separate method rather than a flag on CreateAWSAccount for a
// concrete reason: a dry run creates nothing, so its response carries no account
// record id. CreateAWSAccount rejects exactly that -- an empty id is how a
// failed onboard used to reach Terraform state -- and it must keep doing so.
func (c *Client) PreflightAWSAccount(accountID, accountName string) error {
	requestBody := map[string]interface{}{
		"accountId":   accountID,
		"accountName": accountName,
		"dryRun":      true,
	}

	// A prism-main that actually probes answers non-2xx carrying the verbatim
	// AWS reason; doRequest has already turned that into an error.
	body, err := c.doRequest("POST", "/accounts/onboard", requestBody)
	if err != nil {
		return err
	}

	// A deployment that answers 200 for a failed probe reports it in the body
	// instead, in the same shape the real onboard uses.
	var data onboardData
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("failed to unmarshal preflight response: %w", err)
	}
	if failure := decodeOnboardFailure(data.Account); failure.isFailureShape() {
		reason := failure.reason()
		if reason == "" {
			reason = "Prism reported the preflight as failed but gave no reason"
		}
		return errors.New(reason)
	}

	// No id is expected here: a dry run creates nothing.
	return nil
}

func (c *Client) CreateAWSAccount(account *AWSAccount) (*AWSAccount, error) {
	// Use the onboard endpoint which does full account setup (IdP/OIDC)
	requestBody := map[string]interface{}{
		"accountId":   account.AccountID,
		"accountName": account.AccountName,
	}

	// Include owner_emails if provided
	if len(account.OwnerEmails) > 0 {
		requestBody["ownerEmails"] = account.OwnerEmails
	}

	// A backend new enough to answer non-2xx for a failed onboard is handled
	// here: doRequest turns >=400 into an error. Older self-hosted versions
	// answer 200 for failures, which the shape checks below catch.
	body, err := c.doRequest("POST", "/accounts/onboard", requestBody)
	if err != nil {
		return nil, err
	}

	var data onboardData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if len(data.Account) == 0 {
		return nil, fmt.Errorf("onboarding AWS account %s failed: Prism returned no account in its response%s",
			account.AccountID, data.countsSuffix())
	}

	// Failure shape: an AccountOnboardingError. See onboardFailure for why the
	// nested object, and not a non-empty `error`, is the discriminator -- that
	// it cannot misfire today rests on the re-onboard path in prism-main
	// explicitly writing error:"" before it snapshots the row, incidental
	// behaviour in another repo that nothing here can see or hold still.
	failure := decodeOnboardFailure(data.Account)
	if failure.isFailureShape() {
		// The reason still comes from `error`, verbatim.
		reason := failure.reason()
		if reason == "" {
			reason = "Prism reported the onboard as failed but gave no reason"
		}
		return nil, fmt.Errorf("onboarding AWS account %s failed: %s", account.AccountID, reason)
	}

	// Success shape: the onboarded account record.
	var onboarded struct {
		ID          string   `json:"id"`
		AccountID   string   `json:"account_id"`
		Name        string   `json:"name"`
		Status      string   `json:"status"`
		OwnerEmails []string `json:"owner_emails,omitempty"`
	}

	if err := json.Unmarshal(data.Account, &onboarded); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Defence in depth: whatever new failure shape the backend grows, an
	// account record with no id does not exist in Prism and must never be
	// written to state as if it did.
	if strings.TrimSpace(onboarded.ID) == "" {
		// Salvage a reason if the body carried one. This is reachable when the
		// nested account is null or a non-object, so the structural check above
		// declined it -- and a reason from the server beats the generic message
		// below, which is the whole point of this change.
		//
		// It cannot misfire on a real AWSAccountResponse carrying a stale
		// row-level error: this branch only runs once the id is known to be
		// empty, and a genuine account record always has one.
		if reason := failure.reason(); reason != "" {
			return nil, fmt.Errorf("onboarding AWS account %s failed: %s", account.AccountID, reason)
		}
		return nil, fmt.Errorf("onboarding AWS account %s did not complete: Prism accepted the request but returned no account record id%s. "+
			"The account is not managed by Prism -- resolve the underlying failure and re-apply",
			account.AccountID, data.countsSuffix())
	}

	// Convert to AWSAccount
	result := &AWSAccount{
		ID:          onboarded.ID,
		AccountID:   onboarded.AccountID,
		AccountName: onboarded.Name,
		OwnerEmails: onboarded.OwnerEmails,
	}

	return result, nil
}

func (c *Client) GetAWSAccount(accountID string) (*AWSAccount, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/aws-accounts/%s", accountID), nil)
	if err != nil {
		return nil, err
	}

	var result AWSAccount
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// ========== Account Factory Operations ==========

// ErrFeatureNotEnabled is returned by CreateAccountViaFactory when the customer
// has no management account configured (the create endpoint answers 403 before
// launching anything).
var ErrFeatureNotEnabled = errors.New("account creation is not enabled for this customer: no management account is configured")

// FactoryCreateInput is the body for POST /accountfactory/create (camelCase to
// match the prism-main handler).
type FactoryCreateInput struct {
	AccountName       string   `json:"accountName"`
	RootEmail         string   `json:"rootEmail"`
	OwnerEmails       []string `json:"ownerEmails,omitempty"`
	AdminRoleName     string   `json:"adminRoleName"`
	AdminTrustAccount string   `json:"adminTrustAccount"`
}

// FactoryStatus is the slice of the operation row the resource polls.
type FactoryStatus struct {
	Status        string `json:"status"`
	Progress      int    `json:"progress"`
	ProgressLabel string `json:"progressLabel"`
	AWSAccountID  string `json:"awsAccountId"`
	FailureReason string `json:"failureReason"`
	ErrorMessage  string `json:"errorMessage"`
}

// CreateAccountViaFactory launches an account-factory run and returns its
// operation id. A 403 (feature gate) is mapped to ErrFeatureNotEnabled.
func (c *Client) CreateAccountViaFactory(in FactoryCreateInput) (string, error) {
	body, err := c.doRequestCustomerRaw("POST", "/accountfactory/create", in)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
			return "", ErrFeatureNotEnabled
		}
		return "", err
	}
	var resp struct {
		OperationID string `json:"operationId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse create response: %w", err)
	}
	if resp.OperationID == "" {
		return "", fmt.Errorf("account factory returned no operation id")
	}
	return resp.OperationID, nil
}

// GetFactoryStatus fetches the live status of a create operation.
func (c *Client) GetFactoryStatus(operationID string) (*FactoryStatus, error) {
	body, err := c.doRequestCustomerRaw("GET", fmt.Sprintf("/accountfactory/create/%s/status", operationID), nil)
	if err != nil {
		return nil, err
	}
	var st FactoryStatus
	if err := json.Unmarshal(body, &st); err != nil {
		return nil, fmt.Errorf("failed to parse status response: %w", err)
	}
	return &st, nil
}

func (c *Client) UpdateAWSAccount(accountID string, account *AWSAccount) (*AWSAccount, error) {
	body, err := c.doRequest("PUT", fmt.Sprintf("/aws-accounts/%s", accountID), account)
	if err != nil {
		return nil, err
	}

	var result AWSAccount
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) DeleteAWSAccount(accountID string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/aws-accounts/%s/deboard", accountID), nil)
	return err
}

func (c *Client) ListAWSAccounts() ([]AWSAccount, error) {
	body, err := c.doRequest("GET", "/aws-accounts", nil)
	if err != nil {
		return nil, err
	}

	var result []AWSAccount
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ========== Permission Set Operations ==========

type PermissionSet struct {
	ID              string            `json:"id,omitempty"`
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	SessionDuration string            `json:"session_duration,omitempty"`
	ManagedPolicies []string          `json:"managed_policies,omitempty"`
	InlinePolicies  map[string]string `json:"inline_policies,omitempty"`
	RelayState      string            `json:"relay_state,omitempty"`
	// CustomerManagedPolicyIDs is the SEND side: attach these CMP ids to the PS.
	CustomerManagedPolicyIDs []string `json:"customer_managed_policy_ids,omitempty"`
	// CustomerManagedPolicies is the RECEIVE side: the backend returns the attached
	// CMPs as slim {id,name,description} objects.
	CustomerManagedPolicies []CustomerManagedPolicySlim `json:"customer_managed_policies,omitempty"`
}

// CustomerManagedPolicySlim is the attached-CMP shape in a permission-set response.
type CustomerManagedPolicySlim struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (c *Client) CreatePermissionSet(permSet *PermissionSet) (*PermissionSet, error) {
	body, err := c.doRequest("POST", "/permission-sets", permSet)
	if err != nil {
		return nil, err
	}

	var result PermissionSet
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetPermissionSet(permSetID string) (*PermissionSet, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/permission-sets/%s", permSetID), nil)
	if err != nil {
		return nil, err
	}

	var result PermissionSet
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) UpdatePermissionSet(permSetID string, permSet *PermissionSet) (*PermissionSet, error) {
	body, err := c.doRequest("PUT", fmt.Sprintf("/permission-sets/%s", permSetID), permSet)
	if err != nil {
		return nil, err
	}

	var result PermissionSet
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) DeletePermissionSet(permSetID string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/permission-sets/%s", permSetID), nil)
	return err
}

func (c *Client) ListPermissionSets() ([]PermissionSet, error) {
	body, err := c.doRequest("GET", "/permission-sets", nil)
	if err != nil {
		return nil, err
	}

	var result []PermissionSet
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ========== Permission Set Assignment Operations ==========

type PermissionSetAssignment struct {
	ID              string   `json:"id,omitempty"`
	CustomerID      string   `json:"customerId,omitempty"`
	PermissionSetID string   `json:"permissionSetId"`
	PrincipalType   string   `json:"principalType"` // USER or GROUP
	PrincipalID     string   `json:"principalId"`
	AccountID       string   `json:"accountId,omitempty"`  // Single account (backwards compatibility)
	AccountIDs      []string `json:"accountIds,omitempty"` // Multiple accounts
	Username        string   `json:"username,omitempty"`   // For USER type
	GroupName       string   `json:"groupName,omitempty"`  // For GROUP type
}

func (c *Client) CreatePermissionSetAssignment(assignment *PermissionSetAssignment) (*PermissionSetAssignment, error) {
	body, err := c.doRequest("POST", "/permission-set-assignments", assignment)
	if err != nil {
		return nil, err
	}

	var result PermissionSetAssignment
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetPermissionSetAssignment(assignmentID string) (*PermissionSetAssignment, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/permission-set-assignments/%s", assignmentID), nil)
	if err != nil {
		return nil, err
	}

	var result PermissionSetAssignment
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) DeletePermissionSetAssignment(assignmentID string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/permission-set-assignments/%s", assignmentID), nil)
	return err
}

func (c *Client) ListPermissionSetAssignments() ([]PermissionSetAssignment, error) {
	body, err := c.doRequest("GET", "/permission-set-assignments", nil)
	if err != nil {
		return nil, err
	}

	// Backend returns { "assignments": [...], "count": N }
	var result struct {
		Assignments []PermissionSetAssignment `json:"assignments"`
		Count       int                       `json:"count"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result.Assignments, nil
}

// ========== User Operations ==========

type User struct {
	ID         string              `json:"id,omitempty"`
	CustomerID string              `json:"customerId"`
	Username   string              `json:"username"`
	Email      string              `json:"email"`
	FirstName  string              `json:"firstName,omitempty"`
	LastName   string              `json:"lastName,omitempty"`
	Enabled    bool                `json:"enabled"`
	Attributes map[string][]string `json:"attributes,omitempty"`
}

func (c *Client) CreateUser(user *User) (*User, error) {
	body, err := c.doRequest("POST", "/users", user)
	if err != nil {
		return nil, err
	}

	var result User
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetUser(userID string) (*User, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/users/%s", userID), nil)
	if err != nil {
		return nil, err
	}

	var result User
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) UpdateUser(userID string, user *User) (*User, error) {
	body, err := c.doRequest("PUT", fmt.Sprintf("/users/%s", userID), user)
	if err != nil {
		return nil, err
	}

	var result User
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) DeleteUser(userID string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/users/%s", userID), nil)
	return err
}

func (c *Client) ListUsers() ([]User, error) {
	body, err := c.doRequest("GET", "/users", nil)
	if err != nil {
		return nil, err
	}

	var result []User
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ========== Group Operations ==========

type Group struct {
	ID          string   `json:"id,omitempty"`
	CustomerID  string   `json:"customerId"`
	Name        string   `json:"name"`
	Path        string   `json:"path,omitempty"`
	Members     []string `json:"members,omitempty"`
}

func (c *Client) CreateGroup(group *Group) (*Group, error) {
	body, err := c.doRequest("POST", "/groups", group)
	if err != nil {
		return nil, err
	}

	var result Group
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetGroup(groupName string) (*Group, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/groups/%s", groupName), nil)
	if err != nil {
		return nil, err
	}

	var result Group
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) UpdateGroup(groupName string, group *Group) (*Group, error) {
	body, err := c.doRequest("PUT", fmt.Sprintf("/groups/%s", groupName), group)
	if err != nil {
		return nil, err
	}

	var result Group
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) DeleteGroup(groupName string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/groups/%s", groupName), nil)
	return err
}

func (c *Client) ListGroups() ([]Group, error) {
	body, err := c.doRequest("GET", "/groups", nil)
	if err != nil {
		return nil, err
	}

	var result []Group
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ========== Group Membership Operations ==========

type GroupMembership struct {
	GroupName string   `json:"groupName"`
	Usernames []string `json:"users"`
}

func (c *Client) AddGroupMembers(groupName string, usernames []string) error {
	membership := GroupMembership{
		Usernames: usernames,
	}
	_, err := c.doRequest("POST", fmt.Sprintf("/groups/%s/members", groupName), membership)
	return err
}

func (c *Client) RemoveGroupMembers(groupName string, usernames []string) error {
	membership := GroupMembership{
		Usernames: usernames,
	}
	_, err := c.doRequest("DELETE", fmt.Sprintf("/groups/%s/members", groupName), membership)
	return err
}

func (c *Client) GetGroupMembers(groupName string) ([]string, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/groups/%s/members", groupName), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Group   string `json:"group"`
		Members []struct {
			Username string `json:"username"`
		} `json:"members"`
		Count int    `json:"count"`
		Realm string `json:"realm"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract usernames from user objects
	usernames := make([]string, len(result.Members))
	for i, member := range result.Members {
		usernames[i] = member.Username
	}

	return usernames, nil
}

// ========== Identity Provider Operations ==========

type IdentityProvider struct {
	ID          string                 `json:"id,omitempty"`
	CustomerID  string                 `json:"customerId"`
	Type        string                 `json:"type"` // google, microsoft, custom, keycloak, saml
	Alias       string                 `json:"alias"`
	DisplayName string                 `json:"displayName,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Config      map[string]interface{} `json:"config"`
}

// SAML 2.0 config keys. The names below are the JSON tags of the backend's
// models.SAMLIdPRequest, transcribed verbatim - note nameIDPolicyFormat's capital
// "ID", which matches Keycloak's own config key so a document imported from an
// IdP round-trips into a create without renaming.
//
// displayName is deliberately absent: it is the resource's own display_name
// attribute and is hoisted for every provider type before the type switch.
//
// useMetadataDescriptorUrl and descriptorCacheSeconds are deliberately not
// exposed. The backend strips them, and exposing the switch on its own would let
// a user turn off the pinned signing certificate - every login then trusts
// whatever a live fetch of a customer-controlled URL returns - with no visible
// diff in Terraform, because config is never refreshed.
var samlStringKeys = []string{
	// Metadata (the backend can populate the endpoints and certificate from one
	// of these instead of having them supplied by hand).
	"metadataUrl",
	"metadataXml",
	// Endpoints and trust.
	"entityId",
	"idpEntityId",
	"singleSignOnServiceUrl",
	"signingCertificate",
	"metadataDescriptorUrl",
	// Identity mapping.
	"nameIDPolicyFormat",
	"principalType",
	"principalAttribute",
	"emailAttributeName",
	// Protocol options.
	"signatureAlgorithm",
}

// samlBoolKeys are *bool on the backend: absent, true and false are three
// distinct states, because the backend PUT is a full replace and an omitted key
// means "leave it as it is".
var samlBoolKeys = []string{
	"validateSignature",
	"wantAssertionsSigned",
	"wantAuthnRequestsSigned",
	"forceAuthn",
	"postBindingResponse",
	"postBindingAuthnRequest",
	"enabled",
}

// samlIntKeys are *int on the backend.
var samlIntKeys = []string{
	"allowedClockSkew",
}

// applySAMLConfig hoists the SAML config keys onto the top level of a create or
// update request body, which is where the backend expects them.
//
// Strings are written only when they are non-empty. Writing "" is never correct:
// an empty signingCertificate means "leave the stored certificate alone", and
// sending the empty string would clear it in Keycloak while validateSignature
// stayed true, breaking every login.
//
// Booleans accept a real Go bool as well as the quoted forms "true"/"false".
// Every other arm in this file uses a bare .(string) type assertion, which
// silently DROPS a boolean written the natural way -
// jsonencode({ validateSignature = true }) - producing a provider that applies
// cleanly with signature validation never switched on. Anything that is neither
// a bool nor parseable by strconv.ParseBool is an error naming the key.
//
// Integers accept float64, which is how encoding/json decodes every JSON number,
// as well as numeric strings.
//
// Keys that are not recognised are ignored, consistent with the other types.
func applySAMLConfig(body map[string]interface{}, config map[string]interface{}) error {
	for _, key := range samlStringKeys {
		raw, ok := config[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return fmt.Errorf("identity provider config key %q must be a string, got %T", key, raw)
		}
		if value == "" {
			continue
		}
		body[key] = value
	}

	for _, key := range samlBoolKeys {
		raw, ok := config[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case bool:
			body[key] = value
		case string:
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("identity provider config key %q must be a boolean (true or false), got %q", key, value)
			}
			body[key] = parsed
		default:
			return fmt.Errorf("identity provider config key %q must be a boolean (true or false), got %T", key, raw)
		}
	}

	for _, key := range samlIntKeys {
		raw, ok := config[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case float64:
			if value != float64(int64(value)) {
				return fmt.Errorf("identity provider config key %q must be a whole number, got %v", key, value)
			}
			body[key] = int64(value)
		case string:
			if value == "" {
				continue
			}
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("identity provider config key %q must be a whole number, got %q", key, value)
			}
			body[key] = parsed
		default:
			return fmt.Errorf("identity provider config key %q must be a whole number, got %T", key, raw)
		}
	}

	return nil
}

func (c *Client) CreateIdentityProvider(idpType string, idp *IdentityProvider) (*IdentityProvider, error) {
	// Build request body based on IdP type - backend expects fields at top level, not nested in config
	requestBody := make(map[string]interface{})

	// Common fields
	if idp.DisplayName != "" {
		requestBody["displayName"] = idp.DisplayName
	}
	requestBody["enabled"] = idp.Enabled

	// Extract config fields and add to top level based on type
	switch idpType {
	case "google":
		// Google requires: clientId, clientSecret, hostedDomain (optional)
		if clientId, ok := idp.Config["clientId"].(string); ok {
			requestBody["clientId"] = clientId
		}
		if clientSecret, ok := idp.Config["clientSecret"].(string); ok {
			requestBody["clientSecret"] = clientSecret
		}
		if hostedDomain, ok := idp.Config["hostedDomain"].(string); ok {
			requestBody["hostedDomain"] = hostedDomain
		}
		// Backend forces these values
		requestBody["trustEmail"] = true
		requestBody["storeToken"] = false
		requestBody["addReadTokenRoleOnCreate"] = false
		requestBody["syncMode"] = "FORCE"

	case "microsoft":
		// Microsoft requires: clientId, clientSecret, tenantId
		if clientId, ok := idp.Config["clientId"].(string); ok {
			requestBody["clientId"] = clientId
		}
		if clientSecret, ok := idp.Config["clientSecret"].(string); ok {
			requestBody["clientSecret"] = clientSecret
		}
		if tenantId, ok := idp.Config["tenantId"].(string); ok {
			requestBody["tenantId"] = tenantId
		}
		requestBody["trustEmail"] = true
		requestBody["storeToken"] = false
		requestBody["syncMode"] = "FORCE"

	case "keycloak":
		// Keycloak requires: clientId, clientSecret, authServerUrl, targetRealm
		if clientId, ok := idp.Config["clientId"].(string); ok {
			requestBody["clientId"] = clientId
		}
		if clientSecret, ok := idp.Config["clientSecret"].(string); ok {
			requestBody["clientSecret"] = clientSecret
		}
		if authServerUrl, ok := idp.Config["authServerUrl"].(string); ok {
			requestBody["authServerUrl"] = authServerUrl
		}
		if targetRealm, ok := idp.Config["targetRealm"].(string); ok {
			requestBody["targetRealm"] = targetRealm
		}

	case "custom":
		// Custom OIDC requires: clientId, clientSecret, authServerUrl, authorizationUrl, tokenUrl, userInfoUrl, issuer
		if clientId, ok := idp.Config["clientId"].(string); ok {
			requestBody["clientId"] = clientId
		}
		if clientSecret, ok := idp.Config["clientSecret"].(string); ok {
			requestBody["clientSecret"] = clientSecret
		}
		if authServerUrl, ok := idp.Config["authServerUrl"].(string); ok {
			requestBody["authServerUrl"] = authServerUrl
		}
		if authorizationUrl, ok := idp.Config["authorizationUrl"].(string); ok {
			requestBody["authorizationUrl"] = authorizationUrl
		}
		if tokenUrl, ok := idp.Config["tokenUrl"].(string); ok {
			requestBody["tokenUrl"] = tokenUrl
		}
		if userInfoUrl, ok := idp.Config["userInfoUrl"].(string); ok {
			requestBody["userInfoUrl"] = userInfoUrl
		}
		if logoutUrl, ok := idp.Config["logoutUrl"].(string); ok {
			requestBody["logoutUrl"] = logoutUrl
		}
		if issuer, ok := idp.Config["issuer"].(string); ok {
			requestBody["issuer"] = issuer
		}
		if providerName, ok := idp.Config["providerName"].(string); ok {
			requestBody["providerName"] = providerName
		}

	case "saml":
		// SAML 2.0 (Microsoft Entra ID, Okta, Google Workspace, ADFS, ...).
		// Typed hoist: the helper explains why a .(string) assertion is not good
		// enough for the boolean keys.
		if err := applySAMLConfig(requestBody, idp.Config); err != nil {
			return nil, err
		}
	}

	body, err := c.doRequest("POST", fmt.Sprintf("/identity-providers/%s", idpType), requestBody)
	if err != nil {
		return nil, err
	}

	// Parse response - backend returns nested in "identityProvider" field
	var response struct {
		IdentityProvider struct {
			Alias       string            `json:"alias"`
			DisplayName string            `json:"displayName"`
			ProviderId  string            `json:"providerId"`
			Enabled     bool              `json:"enabled"`
			TrustEmail  bool              `json:"trustEmail"`
			StoreToken  bool              `json:"storeToken"`
			Config      map[string]string `json:"config"`
		} `json:"identityProvider"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert response to IdentityProvider
	result := &IdentityProvider{
		Type:        idpType,
		Alias:       response.IdentityProvider.Alias,
		DisplayName: response.IdentityProvider.DisplayName,
		Enabled:     response.IdentityProvider.Enabled,
		Config:      make(map[string]interface{}),
	}

	// Convert config map from string to interface{}
	for k, v := range response.IdentityProvider.Config {
		result.Config[k] = v
	}

	return result, nil
}

func (c *Client) GetIdentityProvider(idpType, alias string) (*IdentityProvider, error) {
	// A single IdP is addressed by alias. The collection path is the *list*
	// endpoint and returns {"identityProviders": [...]}, which does not match the
	// response struct below and would silently decode to zero values.
	body, err := c.doRequest("GET", fmt.Sprintf("/identity-providers/%s/%s", idpType, alias), nil)
	if err != nil {
		return nil, err
	}

	// Parse response - backend returns nested in "identityProvider" field
	var response struct {
		IdentityProvider struct {
			Alias       string            `json:"alias"`
			DisplayName string            `json:"displayName"`
			ProviderId  string            `json:"providerId"`
			Enabled     bool              `json:"enabled"`
			TrustEmail  bool              `json:"trustEmail"`
			StoreToken  bool              `json:"storeToken"`
			Config      map[string]string `json:"config"`
		} `json:"identityProvider"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert response to IdentityProvider
	result := &IdentityProvider{
		Type:        idpType,
		Alias:       response.IdentityProvider.Alias,
		DisplayName: response.IdentityProvider.DisplayName,
		Enabled:     response.IdentityProvider.Enabled,
		Config:      make(map[string]interface{}),
	}

	// Convert config map from string to interface{}
	for k, v := range response.IdentityProvider.Config {
		result.Config[k] = v
	}

	return result, nil
}

func (c *Client) UpdateIdentityProvider(idpType, alias string, idp *IdentityProvider) (*IdentityProvider, error) {
	// Build request body based on IdP type - backend expects fields at top level, not nested in config
	requestBody := make(map[string]interface{})

	// The backend PUT replaces the entire IdP definition, so omitting displayName
	// decodes to "" server-side and blanks it in Keycloak rather than leaving it
	// untouched. Refuse the call instead of silently clearing it.
	if idp.DisplayName == "" {
		return nil, fmt.Errorf("displayName is required to update an identity provider: an empty value would clear the existing display name")
	}
	requestBody["displayName"] = idp.DisplayName
	requestBody["enabled"] = idp.Enabled

	// Extract config fields and add to top level based on type
	switch idpType {
	case "google":
		if clientId, ok := idp.Config["clientId"].(string); ok {
			requestBody["clientId"] = clientId
		}
		if clientSecret, ok := idp.Config["clientSecret"].(string); ok {
			requestBody["clientSecret"] = clientSecret
		}
		if hostedDomain, ok := idp.Config["hostedDomain"].(string); ok {
			requestBody["hostedDomain"] = hostedDomain
		}
		requestBody["trustEmail"] = true
		requestBody["storeToken"] = false
		requestBody["addReadTokenRoleOnCreate"] = false
		requestBody["syncMode"] = "FORCE"

	case "microsoft":
		if clientId, ok := idp.Config["clientId"].(string); ok {
			requestBody["clientId"] = clientId
		}
		if clientSecret, ok := idp.Config["clientSecret"].(string); ok {
			requestBody["clientSecret"] = clientSecret
		}
		if tenantId, ok := idp.Config["tenantId"].(string); ok {
			requestBody["tenantId"] = tenantId
		}
		requestBody["trustEmail"] = true
		requestBody["storeToken"] = false
		requestBody["syncMode"] = "FORCE"

	case "keycloak":
		if clientId, ok := idp.Config["clientId"].(string); ok {
			requestBody["clientId"] = clientId
		}
		if clientSecret, ok := idp.Config["clientSecret"].(string); ok {
			requestBody["clientSecret"] = clientSecret
		}
		if authServerUrl, ok := idp.Config["authServerUrl"].(string); ok {
			requestBody["authServerUrl"] = authServerUrl
		}
		if targetRealm, ok := idp.Config["targetRealm"].(string); ok {
			requestBody["targetRealm"] = targetRealm
		}

	case "custom":
		if clientId, ok := idp.Config["clientId"].(string); ok {
			requestBody["clientId"] = clientId
		}
		if clientSecret, ok := idp.Config["clientSecret"].(string); ok {
			requestBody["clientSecret"] = clientSecret
		}
		if authServerUrl, ok := idp.Config["authServerUrl"].(string); ok {
			requestBody["authServerUrl"] = authServerUrl
		}
		if authorizationUrl, ok := idp.Config["authorizationUrl"].(string); ok {
			requestBody["authorizationUrl"] = authorizationUrl
		}
		if tokenUrl, ok := idp.Config["tokenUrl"].(string); ok {
			requestBody["tokenUrl"] = tokenUrl
		}
		if userInfoUrl, ok := idp.Config["userInfoUrl"].(string); ok {
			requestBody["userInfoUrl"] = userInfoUrl
		}
		if logoutUrl, ok := idp.Config["logoutUrl"].(string); ok {
			requestBody["logoutUrl"] = logoutUrl
		}
		if issuer, ok := idp.Config["issuer"].(string); ok {
			requestBody["issuer"] = issuer
		}
		if providerName, ok := idp.Config["providerName"].(string); ok {
			requestBody["providerName"] = providerName
		}

	case "saml":
		// An empty signingCertificate is skipped rather than sent, so an update
		// that does not restate the certificate leaves the stored one intact.
		if err := applySAMLConfig(requestBody, idp.Config); err != nil {
			return nil, err
		}
	}

	// Updates are addressed by alias; the collection path has no PUT handler.
	body, err := c.doRequest("PUT", fmt.Sprintf("/identity-providers/%s/%s", idpType, alias), requestBody)
	if err != nil {
		return nil, err
	}

	// Parse response - backend returns nested in "identityProvider" field
	var response struct {
		IdentityProvider struct {
			Alias       string            `json:"alias"`
			DisplayName string            `json:"displayName"`
			ProviderId  string            `json:"providerId"`
			Enabled     bool              `json:"enabled"`
			TrustEmail  bool              `json:"trustEmail"`
			StoreToken  bool              `json:"storeToken"`
			Config      map[string]string `json:"config"`
		} `json:"identityProvider"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert response to IdentityProvider
	result := &IdentityProvider{
		Type:        idpType,
		Alias:       response.IdentityProvider.Alias,
		DisplayName: response.IdentityProvider.DisplayName,
		Enabled:     response.IdentityProvider.Enabled,
		Config:      make(map[string]interface{}),
	}

	// Convert config map from string to interface{}
	for k, v := range response.IdentityProvider.Config {
		result.Config[k] = v
	}

	return result, nil
}

func (c *Client) DeleteIdentityProvider(idpType, alias string) error {
	// Deletes are addressed by alias; the collection path has no DELETE handler.
	_, err := c.doRequest("DELETE", fmt.Sprintf("/identity-providers/%s/%s", idpType, alias), nil)
	return err
}

func (c *Client) ListIdentityProviders() ([]IdentityProvider, error) {
	body, err := c.doRequest("GET", "/identity-providers", nil)
	if err != nil {
		return nil, err
	}

	var result []IdentityProvider
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ========== Organization Policy Operations ==========

// orgPolicyMu serializes all org policy API calls to prevent concurrent operations.
var orgPolicyMu sync.Mutex

type OrgPolicy struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Content     string `json:"content,omitempty"`
	AWSManaged  bool   `json:"aws_managed,omitempty"`
}

type OrgPolicyTarget struct {
	TargetID string `json:"target_id"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Arn      string `json:"arn,omitempty"`
}

func (c *Client) CreateOrgPolicy(managementAccountID string, policy *OrgPolicy) (*OrgPolicy, error) {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"name":                  policy.Name,
		"content":               policy.Content,
	}
	if policy.Description != "" {
		requestBody["description"] = policy.Description
	}
	if policy.Type != "" {
		requestBody["policy_type"] = policy.Type
	}

	body, err := c.doRequestCustomerRaw("POST", "/org/policies/create", requestBody)
	if err != nil {
		return nil, err
	}

	var result OrgPolicy
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetOrgPolicy(managementAccountID, policyID string) (*OrgPolicy, error) {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"policy_id":             policyID,
	}

	body, err := c.doRequestCustomerRaw("POST", "/org/policies/describe", requestBody)
	if err != nil {
		return nil, err
	}

	var result OrgPolicy
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) UpdateOrgPolicy(managementAccountID, policyID string, policy *OrgPolicy) (*OrgPolicy, error) {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"policy_id":             policyID,
	}
	if policy.Name != "" {
		requestBody["name"] = policy.Name
	}
	if policy.Description != "" {
		requestBody["description"] = policy.Description
	}
	if policy.Content != "" {
		requestBody["content"] = policy.Content
	}

	body, err := c.doRequestCustomerRaw("POST", "/org/policies/update", requestBody)
	if err != nil {
		return nil, err
	}

	var result OrgPolicy
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) DeleteOrgPolicy(managementAccountID, policyID string) error {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"policy_id":             policyID,
	}

	_, err := c.doRequestCustomerRaw("POST", "/org/policies/delete", requestBody)
	return err
}

func (c *Client) ListOrgPolicies(managementAccountID, policyType string) ([]OrgPolicy, error) {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
	}
	if policyType != "" {
		requestBody["policy_type"] = policyType
	}

	body, err := c.doRequestCustomerRaw("POST", "/org/policies", requestBody)
	if err != nil {
		return nil, err
	}

	var result []OrgPolicy
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

func (c *Client) AttachOrgPolicy(managementAccountID, policyID, targetID string) error {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"policy_id":             policyID,
		"target_id":             targetID,
	}

	_, err := c.doRequestCustomerRaw("POST", "/org/policies/attach", requestBody)
	return err
}

func (c *Client) DetachOrgPolicy(managementAccountID, policyID, targetID string) error {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"policy_id":             policyID,
		"target_id":             targetID,
	}

	_, err := c.doRequestCustomerRaw("POST", "/org/policies/detach", requestBody)
	return err
}

func (c *Client) ListOrgPolicyTargets(managementAccountID, policyID string) ([]OrgPolicyTarget, error) {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"policy_id":             policyID,
	}

	body, err := c.doRequestCustomerRaw("POST", "/org/policies/targets", requestBody)
	if err != nil {
		return nil, err
	}

	var result []OrgPolicyTarget
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

func (c *Client) ListOrgPoliciesForTarget(managementAccountID, targetID, policyType string) ([]OrgPolicy, error) {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"target_id":             targetID,
	}
	if policyType != "" {
		requestBody["policy_type"] = policyType
	}

	body, err := c.doRequestCustomerRaw("POST", "/org/policies/for-target", requestBody)
	if err != nil {
		return nil, err
	}

	var result []OrgPolicy
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ========== Organization Structure Operations ==========

// OrgNode is a single organization node as returned by POST /org/create-ou.
type OrgNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Email  string `json:"email,omitempty"`
	Status string `json:"status,omitempty"`
}

// OrgNodeDetail is the response of POST /org/node: one node, resolved directly,
// without walking the tree. Accounts is populated for OUs only.
type OrgNodeDetail struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"` // "OU" or "ACCOUNT"
	ParentID string   `json:"parent_id"`
	Accounts []string `json:"accounts,omitempty"`
}

// ErrOrgNodeNotFound reports that the deployment answered a structured 404 for
// the node itself: it genuinely is not in the organization. This is the only
// error a caller may safely treat as "gone" and drop from Terraform state.
var ErrOrgNodeNotFound = errors.New("org node not found")

// ErrOrgNodeUnsupported reports that this deployment does not implement
// POST /org/node at all -- the request never reached a handler.
//
// It exists to keep that case away from ErrOrgNodeNotFound. Both arrive as a
// 404, and conflating them would delete live OUs from state on every refresh
// against an older deployment, which Terraform would then try to re-create.
var ErrOrgNodeUnsupported = errors.New("this Prism deployment does not support POST /org/node")

// ErrOrgNodeForbidden reports that the node exists but sits outside the caller's
// tenant subtree. Distinct from absence: the resource must stay in state.
var ErrOrgNodeForbidden = errors.New("org node is outside this tenant's subtree")

// ErrOrgNodeInvalidID reports that the id is not a well-formed organization node
// id -- a configuration mistake, not drift.
var ErrOrgNodeInvalidID = errors.New("not a valid organization node id")

// GetOrgNode resolves a single node -- an OU or an account -- by id.
//
// Server-side this is 3 AWS calls for an OU and 1 for an account. It replaced a
// full organization traversal, which cost ~275ms per node against a throughput
// cap of ~2.2 AWS Organizations calls/sec and timed out an ECS Envoy sidecar at
// 15.7s on a 28-OU tree.
//
// Deliberately does NOT take orgPolicyMu. That mutex serializes org MUTATIONS so
// concurrent Create/Delete/Move cannot race on the same tree; this is a pure read
// with no shared state to guard. Holding it here serializes every prism_org_ou
// refresh behind one global lock and defeats Terraform's parallelism entirely:
// measured live against a 36-OU tree, `terraform plan` cost ~1.75s per managed OU
// (2.4s for one, 14.0s for eight) instead of finishing in roughly one round.
// An earlier revision of this cache-free path inherited the lock by convention
// from the policy calls below -- do not reinstate it.
func (c *Client) GetOrgNode(nodeID string) (*OrgNodeDetail, error) {
	requestBody := map[string]interface{}{
		"management_account_id": "",
		"node_id":               nodeID,
	}

	body, err := c.doRequestCustomerRaw("POST", "/org/node", requestBody)
	if err != nil {
		return nil, classifyOrgNodeError(err)
	}

	var result OrgNodeDetail
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal org node: %w", err)
	}

	return &result, nil
}

// Machine-readable codes returned by POST /org/node. These are the positive
// signal a caller needs before acting: a bare 404 two proxies deep says nothing
// reliable about whether the node exists.
const (
	orgNodeCodeNotFound  = "ORG_NODE_NOT_FOUND"
	orgNodeCodeForbidden = "ORG_NODE_FORBIDDEN"
	orgNodeCodeInvalidID = "ORG_NODE_INVALID_ID"
)

// classifyOrgNodeError turns an /org/node failure into one of the sentinels, in
// a fixed order of decreasing confidence:
//
//  1. the structured `code` in the error envelope -- the intended signal;
//  2. the "ORG_NODE_NOT_FOUND:" prefix prism-ops also stamps onto the `error`
//     string, which survives an older prism-main that drops unknown keys;
//  3. evidence that no handler ran at all (a router's plain-text 404, a 405, or
//     prism-main relaying a 404 from a prism-ops without the route);
//  4. anything else -- returned as an error, never as absence.
//
// Removal from Terraform state requires positive evidence from step 1 or 2. A
// 404 on its own never qualifies. The asymmetry is deliberate: a wrong
// "unsupported" or "unrecognised" is a loud message the operator can act on,
// while a wrong "not found" silently drops a live OU from state and has
// Terraform try to re-create infrastructure that already exists.
func classifyOrgNodeError(err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	body := strings.TrimSpace(apiErr.Body)

	// 1. The structured code.
	switch orgNodeErrorCode(apiErr.Body) {
	case orgNodeCodeNotFound:
		return fmt.Errorf("%w: %s", ErrOrgNodeNotFound, body)
	case orgNodeCodeForbidden:
		return fmt.Errorf("%w: %s", ErrOrgNodeForbidden, body)
	case orgNodeCodeInvalidID:
		return fmt.Errorf("%w: %s", ErrOrgNodeInvalidID, body)
	}

	// 2. The code prefixed onto the error string, for a prism-main that does not
	// forward `code` yet. Anchored at the start on purpose: a code appearing
	// mid-string means something re-wrapped the message, and a re-wrapped
	// message is not evidence a handler answered.
	if strings.HasPrefix(orgNodeErrorMessage(apiErr.Body), orgNodeCodeNotFound+":") {
		return fmt.Errorf("%w: %s", ErrOrgNodeNotFound, body)
	}

	// 3. No handler ran.
	switch apiErr.StatusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		if !looksLikePrismError(apiErr.Body) {
			// gorilla/mux falls through to http.NotFoundHandler when no route
			// matches (prism-main registers no custom NotFoundHandler, and its
			// middleware does not run on an unmatched route), which writes the
			// plain text "404 page not found". An ingress error page is
			// likewise not a Prism envelope.
			return fmt.Errorf("%w (answered %d with a non-Prism body: %s)",
				ErrOrgNodeUnsupported, apiErr.StatusCode, body)
		}
		if strings.Contains(apiErr.Body, opsNoHandlerMarker) {
			// prism-main relayed a 404 from prism-ops, wrapped in its own
			// envelope. It only falls back to this phrasing when the upstream
			// body carried no "error" field -- i.e. prism-ops answered from its
			// router, not from a handler.
			return fmt.Errorf("%w (prism-ops relayed: %s)", ErrOrgNodeUnsupported, body)
		}
		// A Prism envelope, but with no ORG_NODE_* code and no prefix: this
		// deployment predates the codes, so absence cannot be confirmed.
		return fmt.Errorf("Prism answered %d for this org node without an ORG_NODE_* code, "+
			"so it is not possible to tell a deleted node from a request that never reached the handler: %s. "+
			"Upgrade prism-main and prism-ops to a version that returns the code. Nothing was removed from state",
			apiErr.StatusCode, body)
	}

	// 4. Everything else -- a 5xx, a timeout, or a 403 without a code -- says
	// nothing about whether the node exists.
	return err
}

// orgNodeErrorCode extracts the machine-readable `code` from an error envelope.
func orgNodeErrorCode(body string) string {
	raw := json.RawMessage(body)
	if !isJSONObject(raw) {
		return ""
	}
	var envelope struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Code)
}

// orgNodeErrorMessage returns the human-readable reason from an error envelope,
// preferring `error` (what prism-main copies the upstream reason into) over
// `message`.
func orgNodeErrorMessage(body string) string {
	raw := json.RawMessage(body)
	if !isJSONObject(raw) {
		return ""
	}
	var envelope struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ""
	}
	if reason := strings.TrimSpace(envelope.Error); reason != "" {
		return reason
	}
	return strings.TrimSpace(envelope.Message)
}

// opsNoHandlerMarker is the prefix prism-main's extractOpsError falls back to
// when a prism-ops response carries no "error" field -- which is what a router
// 404 from a prism-ops that predates /org/node looks like by the time it has
// been wrapped in prism-main's own error envelope.
const opsNoHandlerMarker = "prism-ops returned status"

// looksLikePrismError reports whether body is a Prism API error envelope, as
// opposed to a router's plain-text 404 or an ingress error page. The `success`
// key is the marker: models.APIResponse always carries it.
func looksLikePrismError(body string) bool {
	raw := json.RawMessage(body)
	if !isJSONObject(raw) {
		return false
	}
	var envelope struct {
		Success *bool `json:"success"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return false
	}
	return envelope.Success != nil
}

func (c *Client) CreateOU(parentID, name string) (*OrgNode, error) {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": "",
		"parent_id":             parentID,
		"name":                  name,
	}

	body, err := c.doRequestCustomerRaw("POST", "/org/create-ou", requestBody)
	if err != nil {
		return nil, err
	}

	var result OrgNode
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal created OU: %w", err)
	}

	return &result, nil
}

func (c *Client) DeleteOU(ouID string) error {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": "",
		"ou_id":                 ouID,
	}

	_, err := c.doRequestCustomerRaw("POST", "/org/delete-ou", requestBody)
	return err
}

func (c *Client) MoveAccount(accountID, sourceParentID, destParentID string) error {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": "",
		"account_id":            accountID,
		"source_parent_id":      sourceParentID,
		"dest_parent_id":        destParentID,
	}

	_, err := c.doRequestCustomerRaw("POST", "/org/move-account", requestBody)
	return err
}

// OrgPolicyTag represents a key-value tag on an org policy.
type OrgPolicyTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (c *Client) ListTagsForOrgPolicy(managementAccountID, policyID string) ([]OrgPolicyTag, error) {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"policy_id":             policyID,
	}

	body, err := c.doRequestCustomerRaw("POST", "/org/policies/tags", requestBody)
	if err != nil {
		return nil, err
	}

	var result []OrgPolicyTag
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags response: %w", err)
	}

	return result, nil
}

func (c *Client) TagOrgPolicy(managementAccountID, policyID string, tags []OrgPolicyTag) error {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"policy_id":             policyID,
		"tags":                  tags,
	}

	_, err := c.doRequestCustomerRaw("POST", "/org/policies/tag", requestBody)
	return err
}

func (c *Client) UntagOrgPolicy(managementAccountID, policyID string, tagKeys []string) error {
	orgPolicyMu.Lock()
	defer orgPolicyMu.Unlock()

	requestBody := map[string]interface{}{
		"management_account_id": managementAccountID,
		"policy_id":             policyID,
		"tag_keys":              tagKeys,
	}

	_, err := c.doRequestCustomerRaw("POST", "/org/policies/untag", requestBody)
	return err
}

// ========== Dependency Waiting Utilities ==========

// isDependencyNotFoundError checks if an error indicates a resource does not yet exist.
func isDependencyNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not found")
}

// waitForDependency polls checkFunc every 2s for up to 60s until the dependency exists.
// Returns nil immediately if the dependency is found on the first check.
// Returns immediately on non-404 errors. Times out with a descriptive error.
func waitForDependency(ctx context.Context, resourceType, resourceID string, checkFunc func() error) error {
	const maxWait = 60 * time.Second
	const pollInterval = 2 * time.Second

	err := checkFunc()
	if err == nil {
		return nil
	}
	if !isDependencyNotFoundError(err) {
		return fmt.Errorf("error checking %s %q: %w", resourceType, resourceID, err)
	}

	fmt.Fprintf(os.Stderr, "[DEPENDENCY WAIT] %s %q not found, polling for up to %s...\n", resourceType, resourceID, maxWait)

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for %s %q", resourceType, resourceID)
		case <-time.After(pollInterval):
		}

		err = checkFunc()
		if err == nil {
			fmt.Fprintf(os.Stderr, "[DEPENDENCY WAIT] %s %q is now available\n", resourceType, resourceID)
			return nil
		}
		if !isDependencyNotFoundError(err) {
			return fmt.Errorf("error checking %s %q: %w", resourceType, resourceID, err)
		}
		fmt.Fprintf(os.Stderr, "[DEPENDENCY WAIT] %s %q still not found, retrying...\n", resourceType, resourceID)
	}

	return fmt.Errorf("timed out after %s waiting for %s %q to become available", maxWait, resourceType, resourceID)
}

// ========== Customer Managed Policy Operations ==========
// The CMP endpoints return RAW JSON (no {success,data} envelope), so these use
// doRequestCustomerRaw.

type CustomerManagedPolicy struct {
	ID             string `json:"id,omitempty"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	PolicyDocument string `json:"policy_document"`
}

func (c *Client) CreateCustomerManagedPolicy(p *CustomerManagedPolicy) (*CustomerManagedPolicy, error) {
	body, err := c.doRequestCustomerRaw("POST", "/customer-managed-policies", p)
	if err != nil {
		return nil, err
	}
	var result CustomerManagedPolicy
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *Client) GetCustomerManagedPolicy(id string) (*CustomerManagedPolicy, error) {
	body, err := c.doRequestCustomerRaw("GET", fmt.Sprintf("/customer-managed-policies/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var result CustomerManagedPolicy
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *Client) UpdateCustomerManagedPolicy(id string, p *CustomerManagedPolicy) (*CustomerManagedPolicy, error) {
	body, err := c.doRequestCustomerRaw("PUT", fmt.Sprintf("/customer-managed-policies/%s", id), p)
	if err != nil {
		return nil, err
	}
	var result CustomerManagedPolicy
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

func (c *Client) DeleteCustomerManagedPolicy(id string) error {
	_, err := c.doRequestCustomerRaw("DELETE", fmt.Sprintf("/customer-managed-policies/%s", id), nil)
	return err
}
