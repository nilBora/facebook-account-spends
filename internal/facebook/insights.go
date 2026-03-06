package facebook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// InsightRow is one row of adset-level spend data from the Insights API.
type InsightRow struct {
	AccountID    string
	CampaignID   string
	CampaignName string
	AdsetID      string
	AdsetName    string
	Impressions  int64
	Clicks       int64
	Spend        float64
	Date         string // YYYY-MM-DD
}

type insightsResponse struct {
	Data   []insightData `json:"data"`
	Paging *struct {
		Cursors struct {
			After string `json:"after"`
		} `json:"cursors"`
		Next string `json:"next"`
	} `json:"paging"`
}

type insightData struct {
	AccountID    string `json:"account_id"`
	CampaignID   string `json:"campaign_id"`
	CampaignName string `json:"campaign_name"`
	AdsetID      string `json:"adset_id"`
	AdsetName    string `json:"adset_name"`
	Impressions  string `json:"impressions"`
	Clicks       string `json:"clicks"`
	Spend        string `json:"spend"`
	DateStart    string `json:"date_start"`
}

// asyncJobResponse is returned when starting an async insights job.
type asyncJobResponse struct {
	ReportRunID string `json:"report_run_id"`
}

// asyncStatusResponse is the job status polling response.
type asyncStatusResponse struct {
	AsyncStatus            string `json:"async_status"`
	AsyncPercentCompletion int    `json:"async_percent_completion"`
}

const (
	insightsFields = "account_id,campaign_id,campaign_name,adset_id,adset_name,impressions,clicks,spend"
	insightsLevel  = "adset"
)

// FetchInsights fetches adset-level spend for a single account and date.
// Tries sync first; on data-too-large errors, falls back to async.
func (c *Client) FetchInsights(ctx context.Context, tokenID, token, accountID, date string) ([]InsightRow, error) {
	rows, err := c.fetchInsightsSync(ctx, tokenID, token, accountID, date)
	if err == nil {
		return rows, nil
	}

	// Fall back to async if the data set is too large.
	if !isDataTooLargeError(err) {
		return nil, err
	}

	return c.fetchInsightsAsync(ctx, tokenID, token, accountID, date)
}

func (c *Client) fetchInsightsSync(ctx context.Context, tokenID, token, accountID, date string) ([]InsightRow, error) {
	params := url.Values{
		"access_token":   {token},
		"fields":         {insightsFields},
		"level":          {insightsLevel},
		"time_range":     {fmt.Sprintf(`{"since":"%s","until":"%s"}`, date, date)},
		"time_increment": {"1"},
		"limit":          {"500"},
	}

	path := fmt.Sprintf("%s/insights", accountID)
	var all []InsightRow

	for {
		body, err := c.get(ctx, tokenID, path, params)
		if err != nil {
			return nil, err
		}

		var resp insightsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode insights: %w", err)
		}

		rows, err := parseInsightRows(resp.Data)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)

		if resp.Paging == nil || resp.Paging.Next == "" {
			break
		}

		params.Set("after", resp.Paging.Cursors.After)
	}

	return all, nil
}

func (c *Client) fetchInsightsAsync(ctx context.Context, tokenID, token, accountID, date string) ([]InsightRow, error) {
	// POST to start async job.
	params := url.Values{
		"access_token":   {token},
		"fields":         {insightsFields},
		"level":          {insightsLevel},
		"time_range":     {fmt.Sprintf(`{"since":"%s","until":"%s"}`, date, date)},
		"time_increment": {"1"},
		"is_async":       {"true"},
	}

	body, err := c.get(ctx, tokenID, fmt.Sprintf("%s/insights", accountID), params)
	if err != nil {
		return nil, fmt.Errorf("start async job: %w", err)
	}

	var jobResp asyncJobResponse
	if err := json.Unmarshal(body, &jobResp); err != nil || jobResp.ReportRunID == "" {
		return nil, fmt.Errorf("parse async job response: %w", err)
	}

	// Poll until complete.
	jobID := jobResp.ReportRunID
	return c.pollAndFetchAsyncJob(ctx, tokenID, token, jobID)
}

func (c *Client) pollAndFetchAsyncJob(ctx context.Context, tokenID, token, jobID string) ([]InsightRow, error) {
	deadline := time.Now().Add(15 * time.Minute)
	backoff := 10 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		statusBody, err := c.get(ctx, tokenID, jobID, url.Values{"access_token": {token}})
		if err != nil {
			return nil, fmt.Errorf("poll job %s: %w", jobID, err)
		}

		var status asyncStatusResponse
		if err := json.Unmarshal(statusBody, &status); err != nil {
			return nil, fmt.Errorf("parse job status: %w", err)
		}

		switch status.AsyncStatus {
		case "Job Completed":
			return c.fetchJobResults(ctx, tokenID, token, jobID)
		case "Job Failed", "Job Skipped":
			return nil, fmt.Errorf("async job %s ended with status: %s", jobID, status.AsyncStatus)
		}

		// Increase backoff up to 60s.
		backoff = time.Duration(float64(backoff) * 1.5)
		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
	}

	return nil, fmt.Errorf("async job %s timed out after 15 minutes", jobID)
}

func (c *Client) fetchJobResults(ctx context.Context, tokenID, token, jobID string) ([]InsightRow, error) {
	params := url.Values{
		"access_token": {token},
		"limit":        {"500"},
	}

	path := fmt.Sprintf("%s/insights", jobID)
	var all []InsightRow

	for {
		body, err := c.get(ctx, tokenID, path, params)
		if err != nil {
			return nil, fmt.Errorf("fetch job results: %w", err)
		}

		var resp insightsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode job results: %w", err)
		}

		rows, err := parseInsightRows(resp.Data)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)

		if resp.Paging == nil || resp.Paging.Next == "" {
			break
		}
		params.Set("after", resp.Paging.Cursors.After)
	}

	return all, nil
}

func parseInsightRows(data []insightData) ([]InsightRow, error) {
	rows := make([]InsightRow, 0, len(data))
	for _, d := range data {
		impressions, _ := strconv.ParseInt(d.Impressions, 10, 64)
		clicks, _ := strconv.ParseInt(d.Clicks, 10, 64)
		spend, _ := strconv.ParseFloat(d.Spend, 64)

		rows = append(rows, InsightRow{
			AccountID:    "act_" + d.AccountID,
			CampaignID:   d.CampaignID,
			CampaignName: d.CampaignName,
			AdsetID:      d.AdsetID,
			AdsetName:    d.AdsetName,
			Impressions:  impressions,
			Clicks:       clicks,
			Spend:        spend,
			Date:         d.DateStart,
		})
	}
	return rows, nil
}

func isDataTooLargeError(err error) bool {
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}
	// Code 1 with subcode 1487534 or "reduce the amount of data" message.
	return apiErr.Code == 1 || apiErr.Code == 100
}
