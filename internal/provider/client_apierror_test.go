package provider

import (
	"errors"
	"testing"
)

func TestAPIError_ErrorStringUnchanged(t *testing.T) {
	e := &APIError{StatusCode: 404, Body: "not found"}
	if got := e.Error(); got != "API error (404): not found" {
		t.Fatalf("Error() = %q, want the legacy format", got)
	}
}

func TestAPIError_ErrorsAs(t *testing.T) {
	var err error = &APIError{StatusCode: 403, Body: "denied"}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 403 {
		t.Fatalf("errors.As did not extract a 403 APIError")
	}
}
