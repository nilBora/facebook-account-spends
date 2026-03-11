package facebook

import (
	"testing"
	"time"
)

func TestAPIError_IsRateLimit(t *testing.T) {
	rateLimitCodes := []int{4, 17, 32, 80000, 80004}
	for _, code := range rateLimitCodes {
		e := &APIError{Code: code}
		if !e.IsRateLimit() {
			t.Errorf("code %d: expected IsRateLimit()=true", code)
		}
	}

	notRateLimitCodes := []int{1, 2, 100, 190, 200, 368}
	for _, code := range notRateLimitCodes {
		e := &APIError{Code: code}
		if e.IsRateLimit() {
			t.Errorf("code %d: expected IsRateLimit()=false", code)
		}
	}
}

func TestAPIError_RetryAfter(t *testing.T) {
	cases := []struct {
		code int
		want time.Duration
	}{
		{4, 10 * time.Minute},
		{17, 5 * time.Minute},
		{32, 5 * time.Minute},
		{80000, 2 * time.Minute},
		{80004, 2 * time.Minute},
		{18, 1 * time.Minute},
		{1, 30 * time.Second},   // default
		{999, 30 * time.Second}, // unknown code → default
	}
	for _, c := range cases {
		e := &APIError{Code: c.code}
		got := e.RetryAfter()
		if got != c.want {
			t.Errorf("code %d: RetryAfter()=%v, want %v", c.code, got, c.want)
		}
	}
}

func TestAPIError_Error(t *testing.T) {
	e := &APIError{Code: 17, Message: "token expired", Type: "OAuthException"}
	msg := e.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}

	// With subcode
	e2 := &APIError{Code: 17, Subcode: 458, Message: "app not authorized"}
	msg2 := e2.Error()
	if msg2 == "" {
		t.Error("Error() with subcode should not be empty")
	}
}

func TestParseError_ValidJSON(t *testing.T) {
	body := []byte(`{"error":{"code":17,"error_subcode":458,"message":"App not authorized","type":"OAuthException"}}`)
	e := parseError(body)
	if e == nil {
		t.Fatal("expected non-nil APIError")
	}
	if e.Code != 17 {
		t.Errorf("Code = %d, want 17", e.Code)
	}
	if e.Subcode != 458 {
		t.Errorf("Subcode = %d, want 458", e.Subcode)
	}
	if e.Message != "App not authorized" {
		t.Errorf("Message = %q", e.Message)
	}
}

func TestParseError_NoError(t *testing.T) {
	body := []byte(`{"data":[{"id":"act_123"}]}`)
	if e := parseError(body); e != nil {
		t.Errorf("expected nil for non-error response, got %+v", e)
	}
}

func TestParseError_InvalidJSON(t *testing.T) {
	if e := parseError([]byte("not json")); e != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", e)
	}
}
