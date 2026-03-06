package token

import (
	"context"
	"log/slog"
	"time"

	"facebook-account-parser/internal/crypto"
	"facebook-account-parser/internal/db"
	"facebook-account-parser/internal/facebook"
)

// Manager wraps the store and provides token operations with quota awareness.
type Manager struct {
	store   db.Store
	limiter *facebook.RateLimiter
	encKey  []byte
}

// New creates a new Manager.
func New(store db.Store, limiter *facebook.RateLimiter, encKey []byte) *Manager {
	return &Manager{
		store:   store,
		limiter: limiter,
		encKey:  encKey,
	}
}

// ActiveToken holds a token with its decrypted access token.
type ActiveToken struct {
	ID          string
	Name        string
	AccessToken string // plaintext
}

// ListActive returns all active tokens with decrypted access tokens.
func (m *Manager) ListActive(ctx context.Context) ([]ActiveToken, error) {
	tokens, err := m.store.ListTokens(ctx)
	if err != nil {
		return nil, err
	}

	var active []ActiveToken
	for _, t := range tokens {
		if !t.IsActive {
			continue
		}
		plaintext, err := crypto.Decrypt(t.AccessToken, m.encKey)
		if err != nil {
			slog.Warn("failed to decrypt token, skipping", "token_id", t.ID, "name", t.Name)
			continue
		}
		active = append(active, ActiveToken{
			ID:          t.ID,
			Name:        t.Name,
			AccessToken: plaintext,
		})
	}
	return active, nil
}

// DecryptToken decrypts the access token for a single db.Token.
func (m *Manager) DecryptToken(t db.Token) (string, error) {
	return crypto.Decrypt(t.AccessToken, m.encKey)
}

// EncryptToken encrypts a plaintext access token for storage.
func (m *Manager) EncryptToken(plaintext string) (string, error) {
	return crypto.Encrypt(plaintext, m.encKey)
}

// CheckExpiring logs warnings for tokens nearing expiration.
func (m *Manager) CheckExpiring(ctx context.Context) {
	tokens, err := m.store.ListTokens(ctx)
	if err != nil {
		slog.Error("failed to list tokens for expiry check", "err", err)
		return
	}

	for _, t := range tokens {
		if !t.IsActive || t.ExpiresAt == nil {
			continue
		}

		daysLeft := time.Until(*t.ExpiresAt).Hours() / 24

		switch {
		case daysLeft <= 0:
			slog.Error("token expired", "name", t.Name, "token_id", t.ID)
			_ = m.store.SetTokenActive(ctx, t.ID, false)
			_ = m.store.SetTokenError(ctx, t.ID, "token expired")
		case daysLeft <= 7:
			slog.Warn("token expiring soon", "name", t.Name,
				"days_left", int(daysLeft), "token_id", t.ID)
		case daysLeft <= 30:
			slog.Info("token expiry notice", "name", t.Name,
				"days_left", int(daysLeft), "token_id", t.ID)
		}
	}
}
