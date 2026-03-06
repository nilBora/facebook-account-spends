package facebook

import (
	"encoding/json"
	"fmt"
	"time"
)

// APIError represents a Facebook Graph API error response.
type APIError struct {
	Code    int    `json:"code"`
	Subcode int    `json:"error_subcode"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (e *APIError) Error() string {
	if e.Subcode != 0 {
		return fmt.Sprintf("facebook api error %d/%d: %s", e.Code, e.Subcode, e.Message)
	}
	return fmt.Sprintf("facebook api error %d: %s", e.Code, e.Message)
}

// IsRateLimit reports whether this error is any kind of rate limit.
func (e *APIError) IsRateLimit() bool {
	switch e.Code {
	case 4, 17, 32, 80000, 80004:
		return true
	}
	return false
}

// RetryAfter returns how long to wait before retrying after this error.
func (e *APIError) RetryAfter() time.Duration {
	switch e.Code {
	case 4:
		return 10 * time.Minute // app-level — affects all tokens
	case 17:
		return 5 * time.Minute // token-level
	case 32:
		return 5 * time.Minute
	case 80000, 80004:
		return 2 * time.Minute // ad account BDC
	case 18:
		return 1 * time.Minute // resource limits
	}
	return 30 * time.Second
}

// apiResponse is the envelope for all FB graph responses.
type apiResponse struct {
	Error *struct {
		Code    int    `json:"code"`
		Subcode int    `json:"error_subcode"`
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// parseError extracts an APIError from raw JSON if present.
func parseError(body []byte) *APIError {
	var r apiResponse
	if err := json.Unmarshal(body, &r); err != nil || r.Error == nil {
		return nil
	}
	return &APIError{
		Code:    r.Error.Code,
		Subcode: r.Error.Subcode,
		Message: r.Error.Message,
		Type:    r.Error.Type,
	}
}
