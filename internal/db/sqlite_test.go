package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"facebook-account-parser/internal/db"
)

func newTestStore(t *testing.T) db.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	sqlDB, driver, err := db.Open(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db.NewStore(sqlDB, driver)
}

// --- Tokens ---

func TestTokenCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Create
	tok, err := s.CreateToken(ctx, "Farmer Vasya", "EAAencrypted", nil)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if tok.ID == "" {
		t.Error("expected non-empty token ID")
	}
	if tok.Name != "Farmer Vasya" {
		t.Errorf("Name = %q, want %q", tok.Name, "Farmer Vasya")
	}
	if !tok.IsActive {
		t.Error("new token should be active")
	}

	// Get
	got, err := s.GetToken(ctx, tok.ID)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got.AccessToken != "EAAencrypted" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "EAAencrypted")
	}

	// List
	tokens, err := s.ListTokens(ctx)
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Errorf("ListTokens len = %d, want 1", len(tokens))
	}

	// Update
	expires := time.Now().UTC().Add(30 * 24 * time.Hour)
	updated, err := s.UpdateToken(ctx, tok.ID, "Farmer Petro", "EAAupdated", &expires)
	if err != nil {
		t.Fatalf("UpdateToken: %v", err)
	}
	if updated.Name != "Farmer Petro" {
		t.Errorf("updated Name = %q, want %q", updated.Name, "Farmer Petro")
	}
	if updated.ExpiresAt == nil {
		t.Error("expected non-nil ExpiresAt after update")
	}

	// SetTokenError
	if err := s.SetTokenError(ctx, tok.ID, "rate limit hit"); err != nil {
		t.Fatalf("SetTokenError: %v", err)
	}
	got, _ = s.GetToken(ctx, tok.ID)
	if got.LastError != "rate limit hit" {
		t.Errorf("LastError = %q, want %q", got.LastError, "rate limit hit")
	}

	// SetTokenActive false
	if err := s.SetTokenActive(ctx, tok.ID, false); err != nil {
		t.Fatalf("SetTokenActive: %v", err)
	}
	got, _ = s.GetToken(ctx, tok.ID)
	if got.IsActive {
		t.Error("token should be inactive after SetTokenActive(false)")
	}

	// Delete
	if err := s.DeleteToken(ctx, tok.ID); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	tokens, _ = s.ListTokens(ctx)
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens after delete, got %d", len(tokens))
	}
}

// --- Ad Accounts ---

func TestAdAccountUpsertAndList(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	tok, _ := s.CreateToken(ctx, "Farmer", "EAAtoken", nil)

	accounts := []db.AdAccount{
		{AccountID: "act_111", Name: "Account One", Currency: "USD", Status: 1, TokenID: tok.ID},
		{AccountID: "act_222", Name: "Account Two", Currency: "EUR", Status: 2, TokenID: tok.ID},
	}
	if err := s.UpsertAdAccounts(ctx, accounts); err != nil {
		t.Fatalf("UpsertAdAccounts: %v", err)
	}

	list, err := s.ListAdAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAdAccounts: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len = %d, want 2", len(list))
	}

	// Upsert again (update) — name change.
	accounts[0].Name = "Account One Renamed"
	if err := s.UpsertAdAccounts(ctx, accounts); err != nil {
		t.Fatalf("UpsertAdAccounts (update): %v", err)
	}
	list, _ = s.ListAdAccounts(ctx)
	if len(list) != 2 {
		t.Errorf("upsert should not create duplicates: len = %d", len(list))
	}

	byToken, err := s.ListAdAccountsByToken(ctx, tok.ID)
	if err != nil {
		t.Fatalf("ListAdAccountsByToken: %v", err)
	}
	if len(byToken) != 2 {
		t.Errorf("ListAdAccountsByToken len = %d, want 2", len(byToken))
	}
}

func TestMarkAccountSynced(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	tok, _ := s.CreateToken(ctx, "Farmer", "token", nil)
	_ = s.UpsertAdAccounts(ctx, []db.AdAccount{
		{AccountID: "act_999", Name: "Test", Currency: "USD", Status: 1, TokenID: tok.ID},
	})

	list, _ := s.ListAdAccounts(ctx)
	acc := list[0]
	if acc.LastSyncedAt != nil {
		t.Error("LastSyncedAt should be nil before first sync")
	}

	if err := s.MarkAccountSynced(ctx, "act_999"); err != nil {
		t.Fatalf("MarkAccountSynced: %v", err)
	}
	list, _ = s.ListAdAccounts(ctx)
	if list[0].LastSyncedAt == nil {
		t.Error("LastSyncedAt should be set after MarkAccountSynced")
	}
}

// --- Spend ---

func TestSpendUpsertDedup(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	tok, _ := s.CreateToken(ctx, "Farmer", "token", nil)
	_ = s.UpsertAdAccounts(ctx, []db.AdAccount{
		{AccountID: "act_100", Name: "Acct", Currency: "USD", Status: 1, TokenID: tok.ID},
	})

	rows := []db.SpendRow{
		{AccountID: "act_100", Date: "2026-03-10", AdsetID: "adset_1", AdsetName: "Adset A", Spend: 10.5, Currency: "USD"},
		{AccountID: "act_100", Date: "2026-03-10", AdsetID: "adset_2", AdsetName: "Adset B", Spend: 5.0, Currency: "USD"},
	}
	if err := s.UpsertSpendRows(ctx, rows); err != nil {
		t.Fatalf("UpsertSpendRows: %v", err)
	}

	// Insert same rows again with updated spend — should dedup.
	rows[0].Spend = 20.0
	if err := s.UpsertSpendRows(ctx, rows); err != nil {
		t.Fatalf("UpsertSpendRows (dedup): %v", err)
	}

	result, total, err := s.ListSpendRaw(ctx, db.SpendFilter{Date: "2026-03-10", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListSpendRaw: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 (dedup)", total)
	}

	// Find adset_1 and check updated spend.
	var found bool
	for _, r := range result {
		if r.AdsetID == "adset_1" {
			found = true
			if r.Spend != 20.0 {
				t.Errorf("adset_1 Spend = %v, want 20.0 (updated)", r.Spend)
			}
		}
	}
	if !found {
		t.Error("adset_1 not found in results")
	}
}

func TestListSpendByDate(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	tok, _ := s.CreateToken(ctx, "Farmer", "token", nil)
	_ = s.UpsertAdAccounts(ctx, []db.AdAccount{
		{AccountID: "act_200", Name: "Brand", Currency: "USD", Status: 1, TokenID: tok.ID},
	})
	_ = s.UpsertSpendRows(ctx, []db.SpendRow{
		{AccountID: "act_200", Date: "2026-03-10", AdsetID: "a1", Spend: 100, Currency: "USD"},
		{AccountID: "act_200", Date: "2026-03-10", AdsetID: "a2", Spend: 200, Currency: "USD"},
		{AccountID: "act_200", Date: "2026-03-11", AdsetID: "a3", Spend: 50, Currency: "USD"},
	})

	rows, err := s.ListSpendByDate(ctx, "2026-03-10")
	if err != nil {
		t.Fatalf("ListSpendByDate: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 aggregated row, got %d", len(rows))
	}
	if rows[0].TotalSpend != 300 {
		t.Errorf("TotalSpend = %v, want 300", rows[0].TotalSpend)
	}
}

func TestListSpendByDateRange(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	tok, _ := s.CreateToken(ctx, "Farmer", "token", nil)
	_ = s.UpsertAdAccounts(ctx, []db.AdAccount{
		{AccountID: "act_300", Name: "Biz", Currency: "USD", Status: 1, TokenID: tok.ID},
	})
	_ = s.UpsertSpendRows(ctx, []db.SpendRow{
		{AccountID: "act_300", Date: "2026-03-09", AdsetID: "x1", Spend: 10, Currency: "USD"},
		{AccountID: "act_300", Date: "2026-03-10", AdsetID: "x2", Spend: 20, Currency: "USD"},
		{AccountID: "act_300", Date: "2026-03-11", AdsetID: "x3", Spend: 30, Currency: "USD"},
		{AccountID: "act_300", Date: "2026-03-12", AdsetID: "x4", Spend: 40, Currency: "USD"},
	})

	rows, err := s.ListSpendByDateRange(ctx, "2026-03-10", "2026-03-11")
	if err != nil {
		t.Fatalf("ListSpendByDateRange: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestListSpendRaw_Pagination(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	tok, _ := s.CreateToken(ctx, "Farmer", "token", nil)
	_ = s.UpsertAdAccounts(ctx, []db.AdAccount{
		{AccountID: "act_400", Name: "Pg", Currency: "USD", Status: 1, TokenID: tok.ID},
	})

	rows := make([]db.SpendRow, 15)
	for i := range rows {
		rows[i] = db.SpendRow{
			AccountID: "act_400",
			Date:      "2026-03-10",
			AdsetID:   "adset_" + string(rune('A'+i)),
			Spend:     float64(i + 1),
			Currency:  "USD",
		}
	}
	_ = s.UpsertSpendRows(ctx, rows)

	page1, total, err := s.ListSpendRaw(ctx, db.SpendFilter{Date: "2026-03-10", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListSpendRaw page 1: %v", err)
	}
	if total != 15 {
		t.Errorf("total = %d, want 15", total)
	}
	if len(page1) != 10 {
		t.Errorf("page 1 len = %d, want 10", len(page1))
	}

	page2, _, _ := s.ListSpendRaw(ctx, db.SpendFilter{Date: "2026-03-10", Page: 2, PageSize: 10})
	if len(page2) != 5 {
		t.Errorf("page 2 len = %d, want 5", len(page2))
	}
}

// --- FX rates ---

func TestFXRates(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if err := s.UpsertFXRate(ctx, "2026-03-10", "EUR", 1.08); err != nil {
		t.Fatalf("UpsertFXRate: %v", err)
	}

	rate, err := s.GetFXRate(ctx, "2026-03-10", "EUR")
	if err != nil {
		t.Fatalf("GetFXRate: %v", err)
	}
	if rate != 1.08 {
		t.Errorf("rate = %v, want 1.08", rate)
	}

	// USD always returns 1.0.
	usd, err := s.GetFXRate(ctx, "2026-03-10", "USD")
	if err != nil {
		t.Fatalf("GetFXRate USD: %v", err)
	}
	if usd != 1.0 {
		t.Errorf("USD rate = %v, want 1.0", usd)
	}

	// Missing rate → error.
	if _, err := s.GetFXRate(ctx, "2026-03-10", "JPY"); err == nil {
		t.Error("expected error for missing FX rate")
	}

	// Upsert (update) existing rate.
	_ = s.UpsertFXRate(ctx, "2026-03-10", "EUR", 1.09)
	rate, _ = s.GetFXRate(ctx, "2026-03-10", "EUR")
	if rate != 1.09 {
		t.Errorf("updated rate = %v, want 1.09", rate)
	}
}

// --- Sync state ---

func TestSyncState(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	tok, _ := s.CreateToken(ctx, "Farmer", "token", nil)
	_ = s.UpsertAdAccounts(ctx, []db.AdAccount{
		{AccountID: "act_500", Name: "Test", Currency: "USD", Status: 1, TokenID: tok.ID},
	})

	// Initial state should be empty.
	st, err := s.GetSyncState(ctx, "act_500")
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	if st.LastOkDate != "" {
		t.Errorf("initial LastOkDate = %q, want empty", st.LastOkDate)
	}

	if err := s.UpdateSyncState(ctx, "act_500", "2026-03-10"); err != nil {
		t.Fatalf("UpdateSyncState: %v", err)
	}
	st, _ = s.GetSyncState(ctx, "act_500")
	if st.LastOkDate != "2026-03-10" {
		t.Errorf("LastOkDate = %q, want 2026-03-10", st.LastOkDate)
	}

	retryAt := time.Now().UTC().Add(5 * time.Minute)
	if err := s.SetNextRetry(ctx, "act_500", retryAt); err != nil {
		t.Fatalf("SetNextRetry: %v", err)
	}
	st, _ = s.GetSyncState(ctx, "act_500")
	if st.NextRetryAt == nil {
		t.Error("NextRetryAt should be set after SetNextRetry")
	}
}

// --- Sync runs ---

func TestSyncRunLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Create run.
	id, err := s.CreateSyncRun(ctx, "manual", "2026-03-11")
	if err != nil {
		t.Fatalf("CreateSyncRun: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty run ID")
	}

	// List — should appear with nil FinishedAt.
	runs, err := s.ListSyncRuns(ctx, 10)
	if err != nil {
		t.Fatalf("ListSyncRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len = %d, want 1", len(runs))
	}
	if runs[0].FinishedAt != nil {
		t.Error("FinishedAt should be nil before FinishSyncRun")
	}
	if runs[0].SyncDate != "2026-03-11" {
		t.Errorf("SyncDate = %q, want 2026-03-11", runs[0].SyncDate)
	}

	// Finish run.
	if err := s.FinishSyncRun(ctx, id, 10, 2); err != nil {
		t.Fatalf("FinishSyncRun: %v", err)
	}
	runs, _ = s.ListSyncRuns(ctx, 10)
	if runs[0].FinishedAt == nil {
		t.Error("FinishedAt should be set after FinishSyncRun")
	}
	if runs[0].SuccessCount != 10 {
		t.Errorf("SuccessCount = %d, want 10", runs[0].SuccessCount)
	}
	if runs[0].ErrorCount != 2 {
		t.Errorf("ErrorCount = %d, want 2", runs[0].ErrorCount)
	}
}

func TestSyncErrors(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	runID, _ := s.CreateSyncRun(ctx, "cron_today", "2026-03-11")

	errs := []db.SyncRunError{
		{ID: "e1", AccountID: "act_111", Message: "token expired"},
		{ID: "e2", AccountID: "act_222", Message: "rate limit hit"},
	}
	if err := s.AddSyncErrors(ctx, runID, errs); err != nil {
		t.Fatalf("AddSyncErrors: %v", err)
	}

	list, err := s.ListSyncErrors(ctx, runID)
	if err != nil {
		t.Fatalf("ListSyncErrors: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].AccountID != "act_111" {
		t.Errorf("AccountID = %q, want act_111", list[0].AccountID)
	}
	if list[1].Message != "rate limit hit" {
		t.Errorf("Message = %q, want 'rate limit hit'", list[1].Message)
	}
}

func TestAddSyncErrors_Empty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	runID, _ := s.CreateSyncRun(ctx, "manual", "2026-03-11")
	// Should not return error for empty slice.
	if err := s.AddSyncErrors(ctx, runID, nil); err != nil {
		t.Fatalf("AddSyncErrors(nil): %v", err)
	}
	if err := s.AddSyncErrors(ctx, runID, []db.SyncRunError{}); err != nil {
		t.Fatalf("AddSyncErrors(empty): %v", err)
	}
}

func TestListSyncRuns_Limit(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	for i := 0; i < 5; i++ {
		_, _ = s.CreateSyncRun(ctx, "manual", "2026-03-11")
	}

	runs, err := s.ListSyncRuns(ctx, 3)
	if err != nil {
		t.Fatalf("ListSyncRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("len = %d, want 3 (limit respected)", len(runs))
	}
}

// --- Dashboard stats ---

func TestGetDashboardStats(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	stats, err := s.GetDashboardStats(ctx)
	if err != nil {
		t.Fatalf("GetDashboardStats (empty): %v", err)
	}
	if stats.TotalTokens != 0 || stats.TotalAccounts != 0 {
		t.Errorf("expected zeros on empty db, got %+v", stats)
	}

	tok, _ := s.CreateToken(ctx, "F1", "tok", nil)
	_, _ = s.CreateToken(ctx, "F2", "tok2", nil)
	_ = s.SetTokenActive(ctx, tok.ID, false)

	_ = s.UpsertAdAccounts(ctx, []db.AdAccount{
		{AccountID: "act_1", Name: "A", Currency: "USD", Status: 1, TokenID: tok.ID},
		{AccountID: "act_2", Name: "B", Currency: "USD", Status: 2, TokenID: tok.ID},
	})

	stats, err = s.GetDashboardStats(ctx)
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 1 { // only active tokens
		t.Errorf("TotalTokens = %d, want 1 (active only)", stats.TotalTokens)
	}
	if stats.TotalAccounts != 2 {
		t.Errorf("TotalAccounts = %d, want 2", stats.TotalAccounts)
	}
	if stats.ActiveAccounts != 1 { // status=1
		t.Errorf("ActiveAccounts = %d, want 1", stats.ActiveAccounts)
	}
}
