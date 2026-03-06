package db

import (
	"context"
	"time"
)

// Token represents a Facebook API token from a farmer.
type Token struct {
	ID           string
	Name         string
	AccessToken  string // plaintext (decrypted before returning)
	IsActive     bool
	ExpiresAt    *time.Time
	LastError    string
	CreatedAt    time.Time
	AccountCount int // populated by ListTokens
}

// AdAccount represents a Facebook ad account discovered via /me/adaccounts.
type AdAccount struct {
	ID           string // UUID (internal primary key)
	AccountID    string // act_XXXXXXX (Facebook account ID)
	Name         string
	Currency     string
	Status       int
	TokenID      string
	LastSyncedAt *time.Time
}

// SpendRow represents one row of adset-level spend data.
type SpendRow struct {
	ID          string
	AccountID   string
	AccountName string // populated by join queries, not stored in DB
	Date        string // YYYY-MM-DD
	CampaignID   string
	CampaignName string
	AdsetID      string
	AdsetName   string
	Impressions int64
	Clicks      int64
	Spend       float64
	Currency    string
	SyncedAt    time.Time
}

// SyncState tracks per-account sync progress.
type SyncState struct {
	AccountID   string
	LastOkDate  string
	NextRetryAt *time.Time
}

// DashboardStats holds aggregated metrics for the dashboard.
type DashboardStats struct {
	TotalTokens    int
	TotalAccounts  int
	ActiveAccounts int
	TodaySpend     float64
	YesterdaySpend float64
}

// SpendFilter holds parameters for querying spend_raw rows.
type SpendFilter struct {
	Date      string // single day (YYYY-MM-DD); ignored when DateFrom+DateTo set
	DateFrom  string // range start
	DateTo    string // range end
	AccountID string // optional: filter by a single account
	Page      int    // 1-based; 0 treated as 1
	PageSize  int    // rows per page; 0 defaults to 50
}

// SpendByAccount is spend aggregated per account for a date range.
type SpendByAccount struct {
	AccountID   string
	AccountName string
	Currency    string
	Date        string
	TotalSpend  float64
}

// Store defines all persistence operations.
type Store interface {
	// Tokens
	CreateToken(ctx context.Context, name, encryptedToken string, expiresAt *time.Time) (Token, error)
	ListTokens(ctx context.Context) ([]Token, error)
	GetToken(ctx context.Context, id string) (Token, error)
	UpdateToken(ctx context.Context, id, name, encryptedToken string, expiresAt *time.Time) (Token, error)
	SetTokenError(ctx context.Context, id, errMsg string) error
	SetTokenActive(ctx context.Context, id string, active bool) error
	DeleteToken(ctx context.Context, id string) error

	// Ad Accounts
	UpsertAdAccounts(ctx context.Context, accounts []AdAccount) error
	ListAdAccounts(ctx context.Context) ([]AdAccount, error)
	ListAdAccountsByToken(ctx context.Context, tokenID string) ([]AdAccount, error)
	MarkAccountSynced(ctx context.Context, id string) error

	// Spend
	UpsertSpendRows(ctx context.Context, rows []SpendRow) error
	ListSpendByDate(ctx context.Context, date string) ([]SpendByAccount, error)
	ListSpendByDateRange(ctx context.Context, from, to string) ([]SpendByAccount, error)
	// ListSpendRaw returns paginated raw rows and the total matching count.
	ListSpendRaw(ctx context.Context, f SpendFilter) (rows []SpendRow, total int, err error)

	// Sync state
	GetSyncState(ctx context.Context, accountID string) (SyncState, error)
	UpdateSyncState(ctx context.Context, accountID, lastOkDate string) error
	SetNextRetry(ctx context.Context, accountID string, retryAt time.Time) error

	// FX rates
	UpsertFXRate(ctx context.Context, date, currency string, rate float64) error
	GetFXRate(ctx context.Context, date, currency string) (float64, error)

	// Dashboard
	GetDashboardStats(ctx context.Context) (DashboardStats, error)
}
