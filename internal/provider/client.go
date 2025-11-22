package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

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
			Timeout: 30 * time.Second,
		},
		Token: token,
	}
}

// doRequestRaw performs an HTTP request without customer path prefix
func (c *Client) doRequestRaw(method, path string, body interface{}) ([]byte, error) {
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

	resp, err := c.HTTPClient.Do(req)
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

	return respBody, nil
}

// doRequest performs an HTTP request with customer path prefix and unwraps the API response
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	url := fmt.Sprintf("%s/api/v1/customers/%s%s", c.BaseURL, c.PrismSubdomain, path)
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Token", c.Token)

	resp, err := c.HTTPClient.Do(req)
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

// ========== Customer Operations ==========

type Customer struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Domain      string `json:"domain"`
}

func (c *Client) CreateCustomer(customer *Customer) (*Customer, error) {
	body, err := c.doRequestRaw("POST", "/api/v1/customers", customer)
	if err != nil {
		return nil, err
	}

	var result Customer
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetCustomer(id string) (*Customer, error) {
	body, err := c.doRequestRaw("GET", fmt.Sprintf("/api/v1/customers/%s", id), nil)
	if err != nil {
		return nil, err
	}

	var result Customer
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) UpdateCustomer(id string, customer *Customer) (*Customer, error) {
	body, err := c.doRequestRaw("PUT", fmt.Sprintf("/api/v1/customers/%s", id), customer)
	if err != nil {
		return nil, err
	}

	var result Customer
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) DeleteCustomer(id string) error {
	_, err := c.doRequestRaw("DELETE", fmt.Sprintf("/api/v1/customers/%s", id), nil)
	return err
}

func (c *Client) ListCustomers() ([]Customer, error) {
	body, err := c.doRequestRaw("GET", "/api/v1/customers", nil)
	if err != nil {
		return nil, err
	}

	var result []Customer
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

// ========== AWS Account Operations ==========

type AWSAccount struct {
	ID          string `json:"id,omitempty"`
	CustomerID  string `json:"customerId"`
	AccountID   string `json:"accountId"`
	AccountName string `json:"accountName"`
	Region      string `json:"region,omitempty"`
	RoleArn     string `json:"roleArn,omitempty"`
}

func (c *Client) CreateAWSAccount(account *AWSAccount) (*AWSAccount, error) {
	body, err := c.doRequest("POST", "/aws-accounts", account)
	if err != nil {
		return nil, err
	}

	var result AWSAccount
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
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

	var result []PermissionSetAssignment
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
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
	Description string   `json:"description,omitempty"`
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
	Type        string                 `json:"type"` // google, microsoft, custom, keycloak
	Alias       string                 `json:"alias"`
	DisplayName string                 `json:"displayName,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Config      map[string]interface{} `json:"config"`
}

func (c *Client) CreateIdentityProvider(idpType string, idp *IdentityProvider) (*IdentityProvider, error) {
	body, err := c.doRequest("POST", fmt.Sprintf("/identity-providers/%s", idpType), idp)
	if err != nil {
		return nil, err
	}

	var result IdentityProvider
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetIdentityProvider(idpType, alias string) (*IdentityProvider, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/identity-providers/%s/%s", idpType, alias), nil)
	if err != nil {
		return nil, err
	}

	var result IdentityProvider
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) UpdateIdentityProvider(idpType, alias string, idp *IdentityProvider) (*IdentityProvider, error) {
	body, err := c.doRequest("PUT", fmt.Sprintf("/identity-providers/%s/%s", idpType, alias), idp)
	if err != nil {
		return nil, err
	}

	var result IdentityProvider
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) DeleteIdentityProvider(idpType, alias string) error {
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
