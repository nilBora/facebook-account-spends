package db

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type sqlStore struct {
	db     *sql.DB
	driver string // "sqlite" or "postgres"
}

// NewStore creates a Store backed by the given *sql.DB.
func NewStore(db *sql.DB, driver string) Store {
	return &sqlStore{db: db, driver: driver}
}

// rebind converts ? placeholders to $1, $2, ... for PostgreSQL.
func (s *sqlStore) rebind(q string) string {
	if s.driver != "postgres" {
		return q
	}
	n := 0
	var b strings.Builder
	b.Grow(len(q) + 16)
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		} else {
			b.WriteByte(q[i])
		}
	}
	return b.String()
}

// timeVal returns a time suitable for the current driver:
// string for SQLite, time.Time for PostgreSQL.
func (s *sqlStore) timeVal(t time.Time) any {
	if s.driver == "postgres" {
		return t.UTC()
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

// timePtrVal is like timeVal but for nullable times.
func (s *sqlStore) timePtrVal(t *time.Time) any {
	if t == nil {
		return nil
	}
	return s.timeVal(*t)
}

// flexTime scans DATETIME (SQLite → string) or TIMESTAMPTZ (PostgreSQL → time.Time).
type flexTime struct {
	T     time.Time
	Valid bool
}

func (f *flexTime) Scan(v any) error {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case time.Time:
		f.T, f.Valid = val.UTC(), true
	case string:
		if t := parseDateTime(val); t != nil {
			f.T, f.Valid = *t, true
		}
	case []byte:
		if t := parseDateTime(string(val)); t != nil {
			f.T, f.Valid = *t, true
		}
	}
	return nil
}

func (f *flexTime) TimePtr() *time.Time {
	if !f.Valid {
		return nil
	}
	return &f.T
}

// --- Tokens ---

func (s *sqlStore) CreateToken(ctx context.Context, name, encryptedToken string, expiresAt *time.Time) (Token, error) {
	id := uuid.New().String()
	_, err := s.db.ExecContext(ctx,
		s.rebind(`INSERT INTO tokens(id, name, access_token, expires_at) VALUES(?,?,?,?)`),
		id, name, encryptedToken, s.timePtrVal(expiresAt),
	)
	if err != nil {
		return Token{}, fmt.Errorf("create token: %w", err)
	}
	return s.GetToken(ctx, id)
}

func (s *sqlStore) ListTokens(ctx context.Context) ([]Token, error) {
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

func (s *sqlStore) GetToken(ctx context.Context, id string) (Token, error) {
	row := s.db.QueryRowContext(ctx,
		s.rebind(`
		SELECT t.id, t.name, t.access_token, t.is_active, t.expires_at,
		       t.last_error, t.created_at,
		       COUNT(a.id) AS account_count
		FROM tokens t
		LEFT JOIN ad_accounts a ON a.token_id = t.id
		WHERE t.id = ?
		GROUP BY t.id
	`), id)
	t, err := scanToken(row)
	if err != nil {
		return Token{}, fmt.Errorf("get token %s: %w", id, err)
	}
	return t, nil
}

func (s *sqlStore) UpdateToken(ctx context.Context, id, name, encryptedToken string, expiresAt *time.Time) (Token, error) {
	_, err := s.db.ExecContext(ctx,
		s.rebind(`UPDATE tokens SET name=?, access_token=?, expires_at=?, last_error='' WHERE id=?`),
		name, encryptedToken, s.timePtrVal(expiresAt), id,
	)
	if err != nil {
		return Token{}, fmt.Errorf("update token: %w", err)
	}
	return s.GetToken(ctx, id)
}

func (s *sqlStore) SetTokenError(ctx context.Context, id, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		s.rebind(`UPDATE tokens SET last_error=? WHERE id=?`), errMsg, id,
	)
	return err
}

func (s *sqlStore) SetTokenActive(ctx context.Context, id string, active bool) error {
	v := 0
	if active {
		v = 1
	}
	_, err := s.db.ExecContext(ctx,
		s.rebind(`UPDATE tokens SET is_active=? WHERE id=?`), v, id,
	)
	return err
}

func (s *sqlStore) DeleteToken(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM tokens WHERE id=?`), id)
	return err
}

// --- Ad Accounts ---

func (s *sqlStore) UpsertAdAccounts(ctx context.Context, accounts []AdAccount) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert accounts: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, s.rebind(`
		INSERT INTO ad_accounts(id, account_id, name, currency, status, token_id)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(account_id, token_id) DO UPDATE SET
			name=excluded.name,
			currency=excluded.currency,
			status=excluded.status,
			token_id=excluded.token_id
	`))
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

func (s *sqlStore) ListAdAccounts(ctx context.Context) ([]AdAccount, error) {
	return s.queryAccounts(ctx, `
		SELECT id, account_id, name, currency, status, token_id, last_synced_at
		FROM ad_accounts
		ORDER BY name
	`)
}

func (s *sqlStore) ListAdAccountsByToken(ctx context.Context, tokenID string) ([]AdAccount, error) {
	return s.queryAccounts(ctx,
		s.rebind(`
		SELECT id, account_id, name, currency, status, token_id, last_synced_at
		FROM ad_accounts WHERE token_id = ?
		ORDER BY name
	`), tokenID)
}

func (s *sqlStore) MarkAccountSynced(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		s.rebind(`UPDATE ad_accounts SET last_synced_at=? WHERE account_id=?`),
		s.timeVal(time.Now().UTC()), accountID,
	)
	return err
}

// --- Spend ---

func (s *sqlStore) UpsertSpendRows(ctx context.Context, rows []SpendRow) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert spend: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, s.rebind(`
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
	`))
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

func (s *sqlStore) ListSpendByDate(ctx context.Context, date string) ([]SpendByAccount, error) {
	return s.querySpendAggregated(ctx, s.rebind(`WHERE sr.date = ?`), date)
}

func (s *sqlStore) ListSpendByDateRange(ctx context.Context, from, to string) ([]SpendByAccount, error) {
	return s.querySpendAggregated(ctx, s.rebind(`WHERE sr.date BETWEEN ? AND ?`), from, to)
}

func (s *sqlStore) ListSpendRaw(ctx context.Context, f SpendFilter) ([]SpendRow, int, error) {
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

	accountSub := `(SELECT account_id, MAX(name) AS name FROM ad_accounts GROUP BY account_id)`

	var total int
	if err := s.db.QueryRowContext(ctx,
		s.rebind(fmt.Sprintf(`SELECT COUNT(*) FROM spend_raw sr LEFT JOIN %s a ON a.account_id = sr.account_id %s`, accountSub, where)),
		args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count spend: %w", err)
	}

	dataArgs := append(args, f.PageSize, (f.Page-1)*f.PageSize)
	query := s.rebind(fmt.Sprintf(`
		SELECT sr.id, sr.account_id, COALESCE(a.name,''), sr.date,
		       sr.campaign_id, sr.campaign_name, sr.adset_id, sr.adset_name,
		       sr.impressions, sr.clicks, sr.spend, sr.currency, sr.synced_at
		FROM spend_raw sr
		LEFT JOIN %s a ON a.account_id = sr.account_id
		%s
		ORDER BY sr.date DESC, sr.account_id, sr.spend DESC
		LIMIT ? OFFSET ?
	`, accountSub, where))

	rows, err := s.db.QueryContext(ctx, query, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query spend raw: %w", err)
	}
	defer rows.Close()

	var result []SpendRow
	for rows.Next() {
		var r SpendRow
		var syncedAt flexTime
		if err := rows.Scan(
			&r.ID, &r.AccountID, &r.AccountName, &r.Date,
			&r.CampaignID, &r.CampaignName, &r.AdsetID, &r.AdsetName,
			&r.Impressions, &r.Clicks, &r.Spend, &r.Currency, &syncedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan spend row: %w", err)
		}
		if syncedAt.Valid {
			r.SyncedAt = syncedAt.T
		}
		result = append(result, r)
	}
	return result, total, rows.Err()
}

// --- Sync state ---

func (s *sqlStore) GetSyncState(ctx context.Context, accountID string) (SyncState, error) {
	var st SyncState
	var retryAt flexTime
	err := s.db.QueryRowContext(ctx,
		s.rebind(`SELECT account_id, last_ok_date, next_retry_at FROM sync_state WHERE account_id=?`),
		accountID,
	).Scan(&st.AccountID, &st.LastOkDate, &retryAt)
	if err == sql.ErrNoRows {
		return SyncState{AccountID: accountID}, nil
	}
	if err != nil {
		return SyncState{}, fmt.Errorf("get sync state: %w", err)
	}
	st.NextRetryAt = retryAt.TimePtr()
	return st, nil
}

func (s *sqlStore) UpdateSyncState(ctx context.Context, accountID, lastOkDate string) error {
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO sync_state(account_id, last_ok_date, next_retry_at)
		VALUES(?,?,NULL)
		ON CONFLICT(account_id) DO UPDATE SET
			last_ok_date=excluded.last_ok_date,
			next_retry_at=NULL
	`), accountID, lastOkDate)
	return err
}

func (s *sqlStore) SetNextRetry(ctx context.Context, accountID string, retryAt time.Time) error {
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO sync_state(account_id, next_retry_at)
		VALUES(?,?)
		ON CONFLICT(account_id) DO UPDATE SET next_retry_at=excluded.next_retry_at
	`), accountID, s.timeVal(retryAt))
	return err
}

// --- FX rates ---

func (s *sqlStore) UpsertFXRate(ctx context.Context, date, currency string, rate float64) error {
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO fx_rates(date, currency, rate_to_usd)
		VALUES(?,?,?)
		ON CONFLICT(date, currency) DO UPDATE SET rate_to_usd=excluded.rate_to_usd
	`), date, currency, rate)
	return err
}

func (s *sqlStore) GetFXRate(ctx context.Context, date, currency string) (float64, error) {
	if currency == "USD" {
		return 1.0, nil
	}
	var rate float64
	err := s.db.QueryRowContext(ctx,
		s.rebind(`SELECT rate_to_usd FROM fx_rates WHERE date=? AND currency=?`),
		date, currency,
	).Scan(&rate)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("fx rate not found for %s on %s", currency, date)
	}
	return rate, err
}

// --- Dashboard ---

func (s *sqlStore) GetDashboardStats(ctx context.Context) (DashboardStats, error) {
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
		s.rebind(`SELECT COALESCE(SUM(spend),0) FROM spend_raw WHERE date=?`), today,
	).Scan(&stats.TodaySpend)

	_ = s.db.QueryRowContext(ctx,
		s.rebind(`SELECT COALESCE(SUM(spend),0) FROM spend_raw WHERE date=?`), yesterday,
	).Scan(&stats.YesterdaySpend)

	return stats, nil
}

// --- Sync runs ---

func (s *sqlStore) CreateSyncRun(ctx context.Context, trigger, syncDate string) (string, error) {
	id := uuid.New().String()
	_, err := s.db.ExecContext(ctx,
		s.rebind(`INSERT INTO sync_runs(id, trigger, sync_date, started_at) VALUES(?,?,?,?)`),
		id, trigger, syncDate, s.timeVal(time.Now().UTC()),
	)
	if err != nil {
		return "", fmt.Errorf("create sync run: %w", err)
	}
	return id, nil
}

func (s *sqlStore) FinishSyncRun(ctx context.Context, id string, success, errCount int) error {
	_, err := s.db.ExecContext(ctx,
		s.rebind(`UPDATE sync_runs SET finished_at=?, success_count=?, error_count=? WHERE id=?`),
		s.timeVal(time.Now().UTC()), success, errCount, id,
	)
	return err
}

func (s *sqlStore) AddSyncErrors(ctx context.Context, runID string, errs []SyncRunError) error {
	if len(errs) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin add sync errors: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		s.rebind(`INSERT INTO sync_errors(id, run_id, account_id, message) VALUES(?,?,?,?)`),
	)
	if err != nil {
		return fmt.Errorf("prepare sync errors: %w", err)
	}
	defer stmt.Close()

	for _, e := range errs {
		id := e.ID
		if id == "" {
			id = uuid.New().String()
		}
		if _, err := stmt.ExecContext(ctx, id, runID, e.AccountID, e.Message); err != nil {
			return fmt.Errorf("insert sync error: %w", err)
		}
	}
	return tx.Commit()
}

func (s *sqlStore) ListSyncRuns(ctx context.Context, limit int) ([]SyncRun, error) {
	rows, err := s.db.QueryContext(ctx,
		s.rebind(`SELECT id, trigger, sync_date, started_at, finished_at, success_count, error_count
		          FROM sync_runs ORDER BY started_at DESC LIMIT ?`),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list sync runs: %w", err)
	}
	defer rows.Close()

	var result []SyncRun
	for rows.Next() {
		var r SyncRun
		var startedAt, finishedAt flexTime
		if err := rows.Scan(&r.ID, &r.Trigger, &r.SyncDate, &startedAt, &finishedAt,
			&r.SuccessCount, &r.ErrorCount); err != nil {
			return nil, fmt.Errorf("scan sync run: %w", err)
		}
		if startedAt.Valid {
			r.StartedAt = startedAt.T
		}
		r.FinishedAt = finishedAt.TimePtr()
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *sqlStore) ListSyncErrors(ctx context.Context, runID string) ([]SyncRunError, error) {
	rows, err := s.db.QueryContext(ctx,
		s.rebind(`SELECT id, run_id, account_id, message, created_at
		          FROM sync_errors WHERE run_id=? ORDER BY created_at`),
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sync errors: %w", err)
	}
	defer rows.Close()

	var result []SyncRunError
	for rows.Next() {
		var e SyncRunError
		var createdAt flexTime
		if err := rows.Scan(&e.ID, &e.RunID, &e.AccountID, &e.Message, &createdAt); err != nil {
			return nil, fmt.Errorf("scan sync error: %w", err)
		}
		if createdAt.Valid {
			e.CreatedAt = createdAt.T
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// --- helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanToken(s scanner) (Token, error) {
	var t Token
	var isActive int
	var expiresAt flexTime
	var createdAt flexTime

	err := s.Scan(
		&t.ID, &t.Name, &t.AccessToken, &isActive,
		&expiresAt, &t.LastError, &createdAt, &t.AccountCount,
	)
	if err != nil {
		return Token{}, fmt.Errorf("scan token: %w", err)
	}

	t.IsActive = isActive == 1
	t.ExpiresAt = expiresAt.TimePtr()
	if createdAt.Valid {
		t.CreatedAt = createdAt.T
	}
	return t, nil
}

func (s *sqlStore) queryAccounts(ctx context.Context, query string, args ...any) ([]AdAccount, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []AdAccount
	for rows.Next() {
		var a AdAccount
		var syncedAt flexTime
		if err := rows.Scan(&a.ID, &a.AccountID, &a.Name, &a.Currency, &a.Status, &a.TokenID, &syncedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		a.LastSyncedAt = syncedAt.TimePtr()
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *sqlStore) querySpendAggregated(ctx context.Context, where string, args ...any) ([]SpendByAccount, error) {
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
