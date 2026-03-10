package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"facebook-account-parser/internal/db"
)

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetDashboardStats(r.Context())
	if err != nil {
		slog.Error("failed to load dashboard stats", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load stats")
		return
	}

	today := time.Now().UTC().Format("2006-01-02")
	rows, _, _ := h.store.ListSpendRaw(r.Context(), db.SpendFilter{Date: today, PageSize: 100})

	h.render(w, "dashboard.html", templateData{
		Theme:        h.getTheme(r),
		ActivePage:   "dashboard",
		Stats:        &stats,
		SpendRawRows: rows,
		Date:         today,
	})
}

func (h *Handler) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetDashboardStats(r.Context())
	if err != nil {
		slog.Error("failed to load dashboard stats", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load stats")
		return
	}
	h.render(w, "dashboard-stats", templateData{Stats: &stats})
}

func (h *Handler) handleSyncRun(w http.ResponseWriter, r *http.Request) {
	date := r.FormValue("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	// Use background context for the entire sync lifecycle: the request context
	// may be canceled if the client disconnects while the sync is running,
	// which would silently drop FinishSyncRun / AddSyncErrors writes.
	bgCtx := context.Background()

	runID, err := h.store.CreateSyncRun(bgCtx, "manual", date)
	if err != nil {
		slog.Error("failed to create sync run", "err", err)
	}

	res := h.pipeline.SyncDate(bgCtx, date)

	if runID != "" {
		_ = h.store.FinishSyncRun(bgCtx, runID, res.Success, len(res.Errors))
		if len(res.Errors) > 0 {
			_ = h.store.AddSyncErrors(bgCtx, runID, res.Errors)
		}
	}

	rows, total, _ := h.store.ListSpendRaw(r.Context(), db.SpendFilter{Date: date, Page: 1, PageSize: 100})
	h.render(w, "spend-raw-table", templateData{
		SpendRawRows: rows,
		Date:         date,
		Page:         1,
		TotalRows:    total,
		TotalPages:   1,
		Success:      fmt.Sprintf("Sync done: %d ok, %d errors", res.Success, len(res.Errors)),
	})
}

func (h *Handler) handleThemeToggle(w http.ResponseWriter, r *http.Request) {
	theme := r.FormValue("theme")
	http.SetCookie(w, &http.Cookie{
		Name:     "theme",
		Value:    theme,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}
