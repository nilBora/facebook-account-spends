package web

import (
	"log/slog"
	"net/http"
)

const syncLogsLimit = 100

func (h *Handler) handleSyncLogsPage(w http.ResponseWriter, r *http.Request) {
	runs, err := h.store.ListSyncRuns(r.Context(), syncLogsLimit)
	if err != nil {
		slog.Error("failed to list sync runs", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load sync logs")
		return
	}

	h.render(w, "sync-logs.html", templateData{
		Theme:      h.getTheme(r),
		ActivePage: "sync-logs",
		SyncRuns:   runs,
	})
}

func (h *Handler) handleSyncErrors(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.renderError(w, http.StatusBadRequest, "missing run id")
		return
	}

	errs, err := h.store.ListSyncErrors(r.Context(), id)
	if err != nil {
		slog.Error("failed to list sync errors", "run_id", id, "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load errors")
		return
	}

	h.render(w, "sync-errors-list", templateData{
		SyncErrors: errs,
	})
}
