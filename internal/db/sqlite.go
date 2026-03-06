package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type sqliteStore struct {
	db *sql.DB
}

// NewStore creates a Store backed by the given *sql.DB.
func NewStore(db *sql.DB) Store {
	return &sqliteStore{db: db}
}

// --- Tokens ---

func (s *sqliteStore) CreateToken(ctx context.Context, name, encryptedToken string, expiresAt *time.Time) (Token, error) {
	id := uuid.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tokens(id, name, access_token, expires_at) VALUES(?,?,?,?)`,
		id, name, encryptedToken, timePtr(expiresAt),
	)
	if err != nil {
		return Token{}, fmt.Errorf("create token: %w", err)
	}
	return s.GetToken(ctx, id)
}

func (s *sqliteStore) ListTokens(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.access_token, t.is_active, t.expires_at,
		       t.last_error, t.created_at,
		       COUNT(a.id) AS account_count
		FROM tokens t
		LEFT JOIN ad_accounts a ON a.token_id = t.id
		GROUP BY t.id
		ORDER BY t.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *sqliteStore) GetToken(ctx context.Context, id string) (Token, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.name, t.access_token, t.is_active, t.expires_at,
		       t.last_error, t.created_at,
		       COUNT(a.id) AS account_count
		FROM tokens t
		LEFT JOIN ad_accounts a ON a.token_id = t.id
		WHERE t.id = ?
		GROUP BY t.id
	`, id)
	t, err := scanToken(row)
	if err != nil {
		return Token{}, fmt.Errorf("get token %s: %w", id, err)
	}
	return t, nil
}

func (s *sqliteStore) UpdateToken(ctx context.Context, id, name, encryptedToken string, expiresAt *time.Time) (Token, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tokens SET name=?, access_token=?, expires_at=?, last_error='' WHERE id=?`,
		name, encryptedToken, timePtr(expiresAt), id,
	)
	if err != nil {
		return Token{}, fmt.Errorf("update token: %w", err)
	}
	return s.GetToken(ctx, id)
}

func (s *sqliteStore) SetTokenError(ctx context.Context, id, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tokens SET last_error=? WHERE id=?`, errMsg, id,
	)
	return err
}

func (s *sqliteStore) SetTokenActive(ctx context.Context, id string, active bool) error {
	v := 0
	if active {
		v = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE tokens SET is_active=? WHERE id=?`, v, id,
	)
	return err
}

func (s *sqliteStore) DeleteToken(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tokens WHERE id=?`, id)
	return err
}

// --- Ad Accounts ---

func (s *sqliteStore) UpsertAdAccounts(ctx context.Context, accounts []AdAccount) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert accounts: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO ad_accounts(id, account_id, name, currency, status, token_id)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(account_id, token_id) DO UPDATE SET
			name=excluded.name,
			currency=excluded.currency,
			status=excluded.status,
			token_id=excluded.token_id
	`)
	if err != nil {
		return fmt.Errorf("prepare upsert: %w", err)
	}
	defer stmt.Close()

	for _, a := range accounts {
		id := uuid.New().String()
		if _, err := stmt.ExecContext(ctx, id, a.AccountID, a.Name, a.Currency, a.Status, a.TokenID); err != nil {
			return fmt.Errorf("upsert account %s: %w", a.AccountID, err)
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) ListAdAccounts(ctx context.Context) ([]AdAccount, error) {
	return s.queryAccounts(ctx, `
		SELECT id, account_id, name, currency, status, token_id, last_synced_at
		FROM ad_accounts
		ORDER BY name
	`)
}

func (s *sqliteStore) ListAdAccountsByToken(ctx context.Context, tokenID string) ([]AdAccount, error) {
	return s.queryAccounts(ctx, `
		SELECT id, account_id, name, currency, status, token_id, last_synced_at
		FROM ad_accounts WHERE token_id = ?
		ORDER BY name
	`, tokenID)
}

func (s *sqliteStore) MarkAccountSynced(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE ad_accounts SET last_synced_at=? WHERE account_id=?`,
		time.Now().UTC().Format("2006-01-02 15:04:05"), accountID,
	)
	return err
}

// --- Spend ---

func (s *sqliteStore) UpsertSpendRows(ctx context.Context, rows []SpendRow) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert spend: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO spend_raw(id, account_id, date, campaign_id, campaign_name,
		                      adset_id, adset_name, impressions, clicks, spend, currency)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(account_id, date, adset_id) DO UPDATE SET
			campaign_id=excluded.campaign_id,
			campaign_name=excluded.campaign_name,
			adset_name=excluded.adset_name,
			impressions=excluded.impressions,
			clicks=excluded.clicks,
			spend=excluded.spend,
			currency=excluded.currency,
			synced_at=CURRENT_TIMESTAMP
	`)
	if err != nil {
		return fmt.Errorf("prepare upsert spend: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		if r.ID == "" {
			r.ID = uuid.New().String()
		}
		_, err := stmt.ExecContext(ctx,
			r.ID, r.AccountID, r.Date, r.CampaignID, r.CampaignName,
			r.AdsetID, r.AdsetName, r.Impressions, r.Clicks, r.Spend, r.Currency,
		)
		if err != nil {
			return fmt.Errorf("upsert spend row: %w", err)
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) ListSpendByDate(ctx context.Context, date string) ([]SpendByAccount, error) {
	return s.querySpendAggregated(ctx,
		`WHERE sr.date = ?`, date,
	)
}

func (s *sqliteStore) ListSpendByDateRange(ctx context.Context, from, to string) ([]SpendByAccount, error) {
	return s.querySpendAggregated(ctx,
		`WHERE sr.date BETWEEN ? AND ?`, from, to,
	)
}

func (s *sqliteStore) ListSpendRaw(ctx context.Context, f SpendFilter) ([]SpendRow, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 50
	}

	var conds []string
	var args []any

	switch {
	case f.DateFrom != "" && f.DateTo != "":
		conds = append(conds, "sr.date BETWEEN ? AND ?")
		args = append(args, f.DateFrom, f.DateTo)
	case f.Date != "":
		conds = append(conds, "sr.date = ?")
		args = append(args, f.Date)
	}
	if f.AccountID != "" {
		conds = append(conds, "(sr.account_id = ? OR LOWER(a.name) LIKE LOWER(?))")
		args = append(args, f.AccountID, "%"+f.AccountID+"%")
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	// Use a subquery to get one name per account_id — the same Facebook
	// account may exist under multiple tokens, so a direct JOIN would
	// produce duplicate spend rows.
	accountSub := `(SELECT account_id, MAX(name) AS name FROM ad_accounts GROUP BY account_id)`

	var total int
	if err := s.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM spend_raw sr LEFT JOIN %s a ON a.account_id = sr.account_id %s`, accountSub, where),
		args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count spend: %w", err)
	}

	dataArgs := append(args, f.PageSize, (f.Page-1)*f.PageSize)
	query := fmt.Sprintf(`
		SELECT sr.id, sr.account_id, COALESCE(a.name,''), sr.date,
		       sr.campaign_id, sr.campaign_name, sr.adset_id, sr.adset_name,
		       sr.impressions, sr.clicks, sr.spend, sr.currency, sr.synced_at
		FROM spend_raw sr
		LEFT JOIN %s a ON a.account_id = sr.account_id
		%s
		ORDER BY sr.date DESC, sr.account_id, sr.spend DESC
		LIMIT ? OFFSET ?
	`, accountSub, where)

	rows, err := s.db.QueryContext(ctx, query, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query spend raw: %w", err)
	}
	defer rows.Close()

	var result []SpendRow
	for rows.Next() {
		var r SpendRow
		var syncedAt sql.NullString
		if err := rows.Scan(
			&r.ID, &r.AccountID, &r.AccountName, &r.Date,
			&r.CampaignID, &r.CampaignName, &r.AdsetID, &r.AdsetName,
			&r.Impressions, &r.Clicks, &r.Spend, &r.Currency, &syncedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan spend row: %w", err)
		}
		if syncedAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", syncedAt.String)
			r.SyncedAt = t
		}
		result = append(result, r)
	}
	return result, total, rows.Err()
}

// --- Sync state ---

func (s *sqliteStore) GetSyncState(ctx context.Context, accountID string) (SyncState, error) {
	var st SyncState
	var retryAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT account_id, last_ok_date, next_retry_at FROM sync_state WHERE account_id=?`,
		accountID,
	).Scan(&st.AccountID, &st.LastOkDate, &retryAt)
	if err == sql.ErrNoRows {
		return SyncState{AccountID: accountID}, nil
	}
	if err != nil {
		return SyncState{}, fmt.Errorf("get sync state: %w", err)
	}
	if retryAt.Valid {
		t, err := time.Parse("2006-01-02 15:04:05", retryAt.String)
		if err == nil {
			st.NextRetryAt = &t
		}
	}
	return st, nil
}

func (s *sqliteStore) UpdateSyncState(ctx context.Context, accountID, lastOkDate string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_state(account_id, last_ok_date, next_retry_at)
		VALUES(?,?,NULL)
		ON CONFLICT(account_id) DO UPDATE SET
			last_ok_date=excluded.last_ok_date,
			next_retry_at=NULL
	`, accountID, lastOkDate)
	return err
}

func (s *sqliteStore) SetNextRetry(ctx context.Context, accountID string, retryAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_state(account_id, next_retry_at)
		VALUES(?,?)
		ON CONFLICT(account_id) DO UPDATE SET next_retry_at=excluded.next_retry_at
	`, accountID, retryAt.UTC().Format("2006-01-02 15:04:05"))
	return err
}

// --- FX rates ---

func (s *sqliteStore) UpsertFXRate(ctx context.Context, date, currency string, rate float64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO fx_rates(date, currency, rate_to_usd)
		VALUES(?,?,?)
		ON CONFLICT(date, currency) DO UPDATE SET rate_to_usd=excluded.rate_to_usd
	`, date, currency, rate)
	return err
}

func (s *sqliteStore) GetFXRate(ctx context.Context, date, currency string) (float64, error) {
	if currency == "USD" {
		return 1.0, nil
	}
	var rate float64
	err := s.db.QueryRowContext(ctx,
		`SELECT rate_to_usd FROM fx_rates WHERE date=? AND currency=?`,
		date, currency,
	).Scan(&rate)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("fx rate not found for %s on %s", currency, date)
	}
	return rate, err
}

// --- Dashboard ---

func (s *sqliteStore) GetDashboardStats(ctx context.Context) (DashboardStats, error) {
	var stats DashboardStats

	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM tokens WHERE is_active=1),
			(SELECT COUNT(*) FROM ad_accounts),
			(SELECT COUNT(*) FROM ad_accounts WHERE status=1)
	`).Scan(&stats.TotalTokens, &stats.TotalAccounts, &stats.ActiveAccounts)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("get dashboard stats: %w", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(spend),0) FROM spend_raw WHERE date=?`, today,
	).Scan(&stats.TodaySpend)

	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(spend),0) FROM spend_raw WHERE date=?`, yesterday,
	).Scan(&stats.YesterdaySpend)

	return stats, nil
}

// --- helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanToken(s scanner) (Token, error) {
	var t Token
	var isActive int
	var expiresAt sql.NullString
	var createdAt string

	err := s.Scan(
		&t.ID, &t.Name, &t.AccessToken, &isActive,
		&expiresAt, &t.LastError, &createdAt, &t.AccountCount,
	)
	if err != nil {
		return Token{}, fmt.Errorf("scan token: %w", err)
	}

	t.IsActive = isActive == 1
	if expiresAt.Valid {
		t.ExpiresAt = parseDateTime(expiresAt.String)
	}
	if parsed := parseDateTime(createdAt); parsed != nil {
		t.CreatedAt = *parsed
	}
	return t, nil
}

func (s *sqliteStore) queryAccounts(ctx context.Context, query string, args ...any) ([]AdAccount, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []AdAccount
	for rows.Next() {
		var a AdAccount
		var syncedAt sql.NullString
		if err := rows.Scan(&a.ID, &a.AccountID, &a.Name, &a.Currency, &a.Status, &a.TokenID, &syncedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		if syncedAt.Valid {
			if t := parseDateTime(syncedAt.String); t != nil {
				a.LastSyncedAt = t
			}
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *sqliteStore) querySpendAggregated(ctx context.Context, where string, args ...any) ([]SpendByAccount, error) {
	accountSub := `(SELECT account_id, MAX(name) AS name, MAX(currency) AS currency FROM ad_accounts GROUP BY account_id)`
	query := fmt.Sprintf(`
		SELECT sr.account_id, COALESCE(a.name,''), COALESCE(a.currency,'USD'),
		       sr.date, SUM(sr.spend)
		FROM spend_raw sr
		LEFT JOIN %s a ON a.account_id = sr.account_id
		%s
		GROUP BY sr.account_id, sr.date
		ORDER BY sr.date DESC, SUM(sr.spend) DESC
	`, accountSub, where)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query spend: %w", err)
	}
	defer rows.Close()

	var result []SpendByAccount
	for rows.Next() {
		var r SpendByAccount
		if err := rows.Scan(&r.AccountID, &r.AccountName, &r.Currency, &r.Date, &r.TotalSpend); err != nil {
			return nil, fmt.Errorf("scan spend: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func timePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

// parseDateTime tries common SQLite datetime string formats and returns nil on failure.
func parseDateTime(s string) *time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return &t
		}
	}
	return nil
}
