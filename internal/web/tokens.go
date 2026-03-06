package web

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

func (h *Handler) handleTokensPage(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.store.ListTokens(r.Context())
	if err != nil {
		slog.Error("failed to load tokens", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load tokens")
		return
	}
	h.render(w, "tokens.html", templateData{
		Theme:      h.getTheme(r),
		ActivePage: "tokens",
		Tokens:     tokens,
	})
}

func (h *Handler) handleTokenTable(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.store.ListTokens(r.Context())
	if err != nil {
		slog.Error("failed to load tokens", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to load tokens")
		return
	}
	h.render(w, "token-table", templateData{Tokens: tokens})
}

func (h *Handler) handleTokenForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "token-form", templateData{})
}

func (h *Handler) handleTokenEditForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tok, err := h.store.GetToken(r.Context(), id)
	if err != nil {
		slog.Error("failed to get token", "id", id, "err", err)
		h.renderError(w, http.StatusNotFound, "token not found")
		return
	}
	// Decrypt token to pre-fill the form (user sees masked value).
	if plaintext, err := h.tokenMgr.DecryptToken(tok); err == nil {
		tok.AccessToken = plaintext
	}
	h.render(w, "token-form", templateData{Token: &tok})
}

func (h *Handler) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	accessToken := r.FormValue("access_token")
	expiresStr := r.FormValue("expires_at")

	if name == "" || accessToken == "" {
		h.renderError(w, http.StatusBadRequest, "name and access_token are required")
		return
	}

	encrypted, err := h.tokenMgr.EncryptToken(accessToken)
	if err != nil {
		slog.Error("failed to encrypt token", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to encrypt token")
		return
	}

	expiresAt := parseDate(expiresStr)

	if _, err := h.store.CreateToken(r.Context(), name, encrypted, expiresAt); err != nil {
		slog.Error("failed to create token", "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	tokens, _ := h.store.ListTokens(r.Context())
	h.render(w, "token-table", templateData{Tokens: tokens, Success: "Token added"})
}

func (h *Handler) handleTokenUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.FormValue("name")
	rawToken := r.FormValue("access_token")
	expiresStr := r.FormValue("expires_at")

	if name == "" {
		h.renderError(w, http.StatusBadRequest, "name is required")
		return
	}

	var encrypted string
	// Only re-encrypt if user provided a new, non-masked token.
	if rawToken != "" && !isMasked(rawToken) {
		var err error
		encrypted, err = h.tokenMgr.EncryptToken(rawToken)
		if err != nil {
			h.renderError(w, http.StatusInternalServerError, "failed to encrypt token")
			return
		}
	} else {
		existing, err := h.store.GetToken(r.Context(), id)
		if err != nil {
			slog.Error("failed to get token for update", "id", id, "err", err)
			h.renderError(w, http.StatusNotFound, "token not found")
			return
		}
		encrypted = existing.AccessToken
	}

	expiresAt := parseDate(expiresStr)

	if _, err := h.store.UpdateToken(r.Context(), id, name, encrypted, expiresAt); err != nil {
		slog.Error("failed to update token", "id", id, "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to update token")
		return
	}

	tokens, _ := h.store.ListTokens(r.Context())
	h.render(w, "token-table", templateData{Tokens: tokens, Success: "Token updated"})
}

func (h *Handler) handleTokenDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteToken(r.Context(), id); err != nil {
		slog.Error("failed to delete token", "id", id, "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to delete token")
		return
	}
	tokens, _ := h.store.ListTokens(r.Context())
	h.render(w, "token-table", templateData{Tokens: tokens})
}

func (h *Handler) handleTokenSync(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tok, err := h.store.GetToken(r.Context(), id)
	if err != nil {
		slog.Error("failed to get token for sync", "id", id, "err", err)
		h.renderError(w, http.StatusNotFound, "token not found")
		return
	}

	plaintext, err := h.tokenMgr.DecryptToken(tok)
	if err != nil {
		slog.Error("failed to decrypt token", "id", id, "err", err)
		h.renderError(w, http.StatusInternalServerError, "failed to decrypt token")
		return
	}

	// Use context.Background() so the sync is not cancelled if the browser
	// closes the connection before the Facebook API call finishes.
	// PostgreSQL/pgx strictly enforces context cancellation (unlike SQLite).
	if err := h.pipeline.DiscoverAccountsForToken(context.Background(), tok.ID, plaintext); err != nil {
		slog.Error("discovery failed", "token_id", tok.ID, "err", err)
		h.renderError(w, http.StatusBadGateway, "discovery failed")
		return
	}

	tokens, _ := h.store.ListTokens(r.Context())
	h.render(w, "token-table", templateData{Tokens: tokens, Success: "Accounts synced"})
}

// parseDate parses a YYYY-MM-DD string into a *time.Time.
func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// isMasked returns true if the token looks like a masked value (contains ****).
func isMasked(s string) bool {
	for i := 0; i+3 < len(s); i++ {
		if s[i] == '*' && s[i+1] == '*' && s[i+2] == '*' && s[i+3] == '*' {
			return true
		}
	}
	return false
}
