package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookie   = "session"
	sessionDuration = 24 * time.Hour
)

// AuthMiddleware wraps the mux, redirecting unauthenticated requests to /login.
// Public paths: /login, /static/. Auth is skipped entirely when no password
// is configured.
func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.authPassword == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/login" || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || !h.verifySession(c.Value) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if h.authPassword == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	h.render(w, "login.html", templateData{})
}

func (h *Handler) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	okUser := hmac.Equal([]byte(username), []byte(h.authUsername))
	okPass := hmac.Equal([]byte(password), []byte(h.authPassword))

	if !okUser || !okPass {
		slog.Warn("failed login attempt", "username", username, "ip", r.RemoteAddr)
		h.render(w, "login.html", templateData{Error: "invalid username or password"})
		return
	}

	token := h.createSession(username)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// createSession returns a signed session token: "username:expires:hmac".
func (h *Handler) createSession(username string) string {
	expires := time.Now().Add(sessionDuration).Unix()
	payload := fmt.Sprintf("%s:%d", username, expires)
	sig := h.sign(payload)
	return payload + ":" + sig
}

// verifySession validates a session token's signature and expiry.
func (h *Handler) verifySession(value string) bool {
	idx := strings.LastIndex(value, ":")
	if idx < 0 {
		return false
	}
	payload, sig := value[:idx], value[idx+1:]

	// Verify signature.
	if !hmac.Equal([]byte(sig), []byte(h.sign(payload))) {
		return false
	}

	// Verify expiry from "username:expires" payload.
	parts := strings.SplitN(payload, ":", 2)
	if len(parts) != 2 {
		return false
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	return true
}

func (h *Handler) sign(payload string) string {
	mac := hmac.New(sha256.New, h.authKey)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
