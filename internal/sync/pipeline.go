package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"facebook-account-parser/internal/db"
	"facebook-account-parser/internal/facebook"
	"facebook-account-parser/internal/queue"
	"facebook-account-parser/internal/token"
)

const workerConcurrency = 4

// Pipeline orchestrates the full sync: discover → fetch spend → backfill.
type Pipeline struct {
	store       db.Store
	fb          *facebook.Client
	tokenMgr    *token.Manager
	backfillDays int
}

// New creates a Pipeline.
func New(store db.Store, fb *facebook.Client, tokenMgr *token.Manager, backfillDays int) *Pipeline {
	return &Pipeline{
		store:        store,
		fb:           fb,
		tokenMgr:     tokenMgr,
		backfillDays: backfillDays,
	}
}

// DiscoverAccounts refreshes the list of ad accounts for all active tokens.
func (p *Pipeline) DiscoverAccounts(ctx context.Context) error {
	tokens, err := p.tokenMgr.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active tokens: %w", err)
	}

	for _, tok := range tokens {
		accounts, err := p.fb.GetAdAccounts(ctx, tok.ID, tok.AccessToken)
		if err != nil {
			slog.Error("failed to get ad accounts", "token_id", tok.ID, "name", tok.Name, "err", err)
			_ = p.store.SetTokenError(ctx, tok.ID, err.Error())
			continue
		}

		dbAccounts := make([]db.AdAccount, 0, len(accounts))
		for _, a := range accounts {
			dbAccounts = append(dbAccounts, db.AdAccount{
				AccountID: a.ID,
				Name:      a.Name,
				Currency:  a.Currency,
				Status:    a.AccountStatus,
				TokenID:   tok.ID,
			})
		}

		if err := p.store.UpsertAdAccounts(ctx, dbAccounts); err != nil {
			slog.Error("failed to upsert accounts", "token_id", tok.ID, "err", err)
			continue
		}

		_ = p.store.SetTokenError(ctx, tok.ID, "") // clear any previous error
		slog.Info("discovered accounts", "token", tok.Name, "count", len(dbAccounts))
	}

	return nil
}

// SyncDate fetches spend for all active accounts for the given date.
func (p *Pipeline) SyncDate(ctx context.Context, date string) error {
	tokens, err := p.tokenMgr.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active tokens: %w", err)
	}

	// Build a map of tokenID → plaintext token for quick lookup.
	tokenMap := make(map[string]string, len(tokens))
	for _, t := range tokens {
		tokenMap[t.ID] = t.AccessToken
	}

	accounts, err := p.store.ListAdAccounts(ctx)
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}

	// Skip only permanently closed accounts (101=closed, 102=any_closed).
	// Disabled (2), unsettled (3), in_grace_period (9) etc. still have
	// spend data accessible via the Insights API.
	var active []db.AdAccount
	for _, a := range accounts {
		if isClosed(a.Status) {
			continue
		}
		if _, ok := tokenMap[a.TokenID]; ok {
			active = append(active, a)
		}
	}

	if len(active) == 0 {
		slog.Info("no accounts to sync", "date", date)
		return nil
	}

	slog.Info("syncing accounts", "date", date, "count", len(active))
	start := time.Now()

	pool := queue.New(workerConcurrency)
	pool.Start(ctx)

	for _, acc := range active {
		acc := acc // capture
		accessToken := tokenMap[acc.TokenID]
		pool.Submit(&insightsJob{
			store:       p.store,
			fb:          p.fb,
			account:     acc,
			tokenID:     acc.TokenID,
			accessToken: accessToken,
			date:        date,
		})
	}

	pool.Wait()
	pool.Close()
	slog.Info("syncing accounts done", "date", date, "count", len(active), "elapsed", time.Since(start).Round(time.Millisecond))
	return nil
}

// FB returns the underlying Facebook client (used by web handlers for on-demand calls).
func (p *Pipeline) FB() *facebook.Client {
	return p.fb
}

// DiscoverAccountsForToken refreshes ad accounts for a single token.
func (p *Pipeline) DiscoverAccountsForToken(ctx context.Context, tokenID, accessToken string) error {
	accounts, err := p.fb.GetAdAccounts(ctx, tokenID, accessToken)
	if err != nil {
		_ = p.store.SetTokenError(ctx, tokenID, err.Error())
		return fmt.Errorf("get ad accounts for token %s: %w", tokenID, err)
	}

	dbAccounts := make([]db.AdAccount, 0, len(accounts))
	for _, a := range accounts {
		dbAccounts = append(dbAccounts, db.AdAccount{
			AccountID: a.ID,
			Name:      a.Name,
			Currency:  a.Currency,
			Status:    a.AccountStatus,
			TokenID:   tokenID,
		})
	}

	if err := p.store.UpsertAdAccounts(ctx, dbAccounts); err != nil {
		return fmt.Errorf("upsert accounts: %w", err)
	}
	_ = p.store.SetTokenError(ctx, tokenID, "")
	slog.Info("discovered accounts", "token_id", tokenID, "count", len(dbAccounts))
	return nil
}

// RunDaily runs discovery + today's sync + backfill for attribution lag.
func (p *Pipeline) RunDaily(ctx context.Context) {
	today := time.Now().UTC().Format("2006-01-02")
	slog.Info("starting daily sync", "date", today)

	if err := p.DiscoverAccounts(ctx); err != nil {
		slog.Error("discovery failed", "err", err)
	}

	if err := p.SyncDate(ctx, today); err != nil {
		slog.Error("daily sync failed", "date", today, "err", err)
	}

	// Backfill last N days for attribution lag.
	for i := 1; i <= p.backfillDays; i++ {
		backfillDate := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		if err := p.SyncDate(ctx, backfillDate); err != nil {
			slog.Error("backfill failed", "date", backfillDate, "err", err)
		}
	}

	slog.Info("daily sync complete")
}

// insightsJob fetches spend for one account on one date.
type insightsJob struct {
	store       db.Store
	fb          *facebook.Client
	account     db.AdAccount
	tokenID     string
	accessToken string
	date        string
}

func (j *insightsJob) Name() string {
	return fmt.Sprintf("insights:%s:%s", j.account.AccountID, j.date)
}

func (j *insightsJob) Execute(ctx context.Context) error {
	rows, err := j.fb.FetchInsights(ctx, j.tokenID, j.accessToken, j.account.AccountID, j.date)
	if err != nil {
		_ = j.store.SetNextRetry(ctx, j.account.AccountID, time.Now().Add(5*time.Minute))
		return fmt.Errorf("fetch insights %s/%s: %w", j.account.AccountID, j.date, err)
	}

	dbRows := make([]db.SpendRow, 0, len(rows))
	for _, r := range rows {
		dbRows = append(dbRows, db.SpendRow{
			AccountID:    j.account.AccountID,
			Date:         r.Date,
			CampaignID:   r.CampaignID,
			CampaignName: r.CampaignName,
			AdsetID:      r.AdsetID,
			AdsetName:    r.AdsetName,
			Impressions:  r.Impressions,
			Clicks:       r.Clicks,
			Spend:        r.Spend,
			Currency:     j.account.Currency,
		})
	}

	if err := j.store.UpsertSpendRows(ctx, dbRows); err != nil {
		return fmt.Errorf("save spend %s/%s: %w", j.account.AccountID, j.date, err)
	}

	_ = j.store.UpdateSyncState(ctx, j.account.AccountID, j.date)
	_ = j.store.MarkAccountSynced(ctx, j.account.AccountID)

	slog.Info("synced spend", "account", j.account.AccountID, "date", j.date, "rows", len(dbRows))
	return nil
}

// isClosed returns true for permanently closed Facebook account statuses.
// Disabled (2), unsettled (3), in_grace_period (9) are NOT closed —
// they still return spend data via the Insights API.
func isClosed(status int) bool {
	return status == 101 || status == 102
}
