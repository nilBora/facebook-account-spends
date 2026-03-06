package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"facebook-account-parser/internal/db"
)

const spendPageSize = 50

func (h *Handler) handleSpendPage(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	f := db.SpendFilter{Date: date, Page: 1, PageSize: spendPageSize}
	rows, total, err := h.store.ListSpendRaw(r.Context(), f)
	if err != nil {
		slog.Error("failed to load spend", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load spend")
		return
	}

	h.render(w, "spend.html", templateData{
		Theme:        h.getTheme(r),
		ActivePage:   "spend",
		SpendRawRows: rows,
		Date:         date,
		Page:         1,
		PageSize:     spendPageSize,
		TotalRows:    total,
		TotalPages:   totalPages(total, spendPageSize),
	})
}

func (h *Handler) handleSpendTable(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	date := q.Get("date")
	from := q.Get("from")
	to := q.Get("to")
	accountID := q.Get("account_id")
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	if date == "" && (from == "" || to == "") {
		date = time.Now().UTC().Format("2006-01-02")
	}

	f := db.SpendFilter{
		Date:      date,
		DateFrom:  from,
		DateTo:    to,
		AccountID: accountID,
		Page:      page,
		PageSize:  spendPageSize,
	}

	rows, total, err := h.store.ListSpendRaw(r.Context(), f)
	if err != nil {
		slog.Error("failed to load spend", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load spend")
		return
	}

	h.render(w, "spend-raw-table", templateData{
		SpendRawRows: rows,
		Date:         date,
		DateFrom:     from,
		DateTo:       to,
		AccountID:    accountID,
		Page:         page,
		PageSize:     spendPageSize,
		TotalRows:    total,
		TotalPages:   totalPages(total, spendPageSize),
	})
}

func totalPages(total, pageSize int) int {
	if total == 0 || pageSize == 0 {
		return 1
	}
	p := total / pageSize
	if total%pageSize != 0 {
		p++
	}
	return p
}
