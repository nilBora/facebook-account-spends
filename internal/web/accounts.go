package web

import (
	"context"
	"log/slog"
	"net/http"

	"facebook-account-parser/internal/db"
)

func (h *Handler) handleAccountsPage(w http.ResponseWriter, r *http.Request) {
	tokenID := r.URL.Query().Get("token_id")

	var (
		accounts []db.AdAccount
		err      error
	)
	if tokenID != "" {
		accounts, err = h.store.ListAdAccountsByToken(r.Context(), tokenID)
	} else {
		accounts, err = h.store.ListAdAccounts(r.Context())
	}
	if err != nil {
		slog.Error("failed to load accounts", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load accounts")
		return
	}

	tokens, err := h.store.ListTokens(r.Context())
	if err != nil {
		slog.Error("failed to load tokens", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load tokens")
		return
	}

	tokenMap := make(map[string]string, len(tokens))
	for _, t := range tokens {
		tokenMap[t.ID] = t.Name
	}

	h.render(w, "accounts.html", templateData{
		Theme:         h.getTheme(r),
		ActivePage:    "accounts",
		Accounts:      accounts,
		Tokens:        tokens,
		TokenMap:      tokenMap,
		FilterTokenID: tokenID,
	})
}

func (h *Handler) handleAccountTable(w http.ResponseWriter, r *http.Request) {
	tokenID := r.URL.Query().Get("token_id")

	var (
		accounts []db.AdAccount
		err      error
	)

	if tokenID != "" {
		accounts, err = h.store.ListAdAccountsByToken(r.Context(), tokenID)
	} else {
		accounts, err = h.store.ListAdAccounts(r.Context())
	}

	if err != nil {
		slog.Error("failed to load accounts", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load accounts")
		return
	}

	tokenMap, _ := h.buildTokenMap(r.Context())
	h.render(w, "account-table", templateData{
		Accounts: accounts,
		TokenMap: tokenMap,
	})
}

func (h *Handler) handleAccountsSync(w http.ResponseWriter, r *http.Request) {
	go func() {
		if err := h.pipeline.DiscoverAccounts(context.Background()); err != nil {
			slog.Error("manual discovery failed", "err", err)
		}
	}()

	accounts, _ := h.store.ListAdAccounts(r.Context())
	tokenMap, _ := h.buildTokenMap(r.Context())
	h.render(w, "account-table", templateData{
		Accounts: accounts,
		TokenMap: tokenMap,
		Success:  "Discovery started",
	})
}

func (h *Handler) buildTokenMap(ctx context.Context) (map[string]string, error) {
	tokens, err := h.store.ListTokens(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(tokens))
	for _, t := range tokens {
		m[t.ID] = t.Name
	}
	return m, nil
}
