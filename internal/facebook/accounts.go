package facebook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// AdAccountInfo holds data returned by /me/adaccounts.
type AdAccountInfo struct {
	ID            string `json:"id"`     // "act_XXXXXXX"
	Name          string `json:"name"`
	Currency      string `json:"currency"`
	AccountStatus int    `json:"account_status"`
}

type adAccountsResponse struct {
	Data   []AdAccountInfo `json:"data"`
	Paging *struct {
		Next string `json:"next"`
	} `json:"paging"`
}

// GetAdAccounts fetches all ad accounts for the given token using cursor pagination.
// tokenID is used for quota tracking; token is the plaintext access token.
func (c *Client) GetAdAccounts(ctx context.Context, tokenID, token string) ([]AdAccountInfo, error) {
	var all []AdAccountInfo

	params := url.Values{
		"access_token": {token},
		"fields":       {"id,name,currency,account_status"},
		"limit":        {"500"},
	}

	path := "me/adaccounts"

	for {
		body, err := c.get(ctx, tokenID, path, params)
		if err != nil {
			return nil, fmt.Errorf("get ad accounts: %w", err)
		}

		var resp adAccountsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode ad accounts response: %w", err)
		}

		all = append(all, resp.Data...)

		// No more pages.
		if resp.Paging == nil || resp.Paging.Next == "" {
			break
		}

		// The next URL already contains all parameters, so we call it directly
		// by extracting just the path+query relative to the versioned base.
		nextURL, err := parseNextPage(resp.Paging.Next, c.apiVersion)
		if err != nil {
			break // best-effort: stop pagination on parse error
		}
		path = nextURL.path
		params = nextURL.params
	}

	return all, nil
}

// nextPage holds a parsed next-page path and its query params.
type nextPage struct {
	path   string
	params url.Values
}

// parseNextPage extracts the path and query from a FB pagination "next" URL.
// FB returns absolute URLs like https://graph.facebook.com/v20.0/me/adaccounts?...
func parseNextPage(nextURL, apiVersion string) (*nextPage, error) {
	// Strip the base + version prefix to get a relative path.
	prefix := fmt.Sprintf("%s/%s/", baseURL, apiVersion)
	if len(nextURL) <= len(prefix) {
		return nil, fmt.Errorf("unexpected next url format: %s", nextURL)
	}

	rest := nextURL[len(prefix):]
	// rest is like "me/adaccounts?access_token=...&after=..."
	idx := -1
	for i, c := range rest {
		if c == '?' {
			idx = i
			break
		}
	}

	if idx < 0 {
		return &nextPage{path: rest, params: url.Values{}}, nil
	}

	path := rest[:idx]
	params, err := url.ParseQuery(rest[idx+1:])
	if err != nil {
		return nil, fmt.Errorf("parse next page query: %w", err)
	}

	return &nextPage{path: path, params: params}, nil
}
