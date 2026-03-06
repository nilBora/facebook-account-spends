package facebook

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	baseURL        = "https://graph.facebook.com"
	// pauseThreshold is the usage % at which we slow down proactively.
	pauseThreshold = 75.0
)

// Client is a rate-limit-aware Facebook Graph API client.
type Client struct {
	http       *http.Client
	limiter    *RateLimiter
	apiVersion string
}

// NewClient creates a new Facebook API client.
func NewClient(apiVersion string) *Client {
	return &Client{
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
		limiter:    NewRateLimiter(),
		apiVersion: apiVersion,
	}
}

// Limiter exposes the underlying RateLimiter for external quota tracking.
func (c *Client) Limiter() *RateLimiter {
	return c.limiter
}

// get performs a GET request to the Graph API, handling rate limits and errors.
// tokenID is used only for quota tracking; token is the actual access token.
func (c *Client) get(ctx context.Context, tokenID, path string, params url.Values) ([]byte, error) {
	// Check if token is currently blocked.
	state := c.limiter.GetState(tokenID)
	if state.IsBlocked() {
		wait := time.Until(state.BlockedUntil)
		slog.Info("token blocked, waiting", "token_id", tokenID, "wait", wait.Round(time.Second))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Proactive slow-down when approaching quota.
	if usage := state.UsagePct(); usage > pauseThreshold {
		slog.Warn("quota usage high, slowing down", "token_id", tokenID, "usage_pct", usage)
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	rawURL := fmt.Sprintf("%s/%s/%s", baseURL, c.apiVersion, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.URL.RawQuery = params.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// Update quota state from response headers.
	c.limiter.UpdateFromResponse(tokenID, resp)

	// HTTP 429: respect Retry-After header.
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := 60 * time.Second
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				wait = time.Duration(secs) * time.Second
			}
		}
		c.limiter.BlockToken(tokenID, wait)
		return nil, &APIError{Code: 429, Message: fmt.Sprintf("rate limited, retry after %s", wait)}
	}

	// Parse FB-level errors from body.
	if apiErr := parseError(body); apiErr != nil {
		if apiErr.IsRateLimit() {
			c.limiter.BlockToken(tokenID, apiErr.RetryAfter())
			slog.Warn("facebook rate limit", "token_id", tokenID,
				"code", apiErr.Code, "wait", apiErr.RetryAfter())
		}
		return nil, apiErr
	}

	return body, nil
}
