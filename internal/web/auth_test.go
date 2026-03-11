package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newAuthTestHandler creates a minimal Handler with only auth fields populated.
func newAuthTestHandler() *Handler {
	return &Handler{
		authKey:      []byte("test-key-must-be-at-least-32byt!"),
		authUsername: "admin",
		authPassword: "secret",
	}
}

// --- Session create / verify ---

func TestCreateAndVerifySession(t *testing.T) {
	h := newAuthTestHandler()
	token := h.createSession("admin")
	if token == "" {
		t.Fatal("createSession returned empty string")
	}
	if !h.verifySession(token) {
		t.Error("freshly created session should be valid")
	}
}

func TestVerifySession_Tampered(t *testing.T) {
	h := newAuthTestHandler()
	token := h.createSession("admin")

	tampered := token + "x"
	if h.verifySession(tampered) {
		t.Error("tampered session should be rejected")
	}
}

func TestVerifySession_WrongKey(t *testing.T) {
	h1 := newAuthTestHandler()
	h2 := &Handler{
		authKey:      []byte("different-key-must-be-32-bytes!!"),
		authUsername: "admin",
		authPassword: "secret",
	}

	token := h1.createSession("admin")
	if h2.verifySession(token) {
		t.Error("session signed with different key should be rejected")
	}
}

func TestVerifySession_Expired(t *testing.T) {
	h := newAuthTestHandler()

	// Manually build an expired session.
	expired := time.Now().Add(-2 * time.Hour).Unix()
	payload := "admin:" + itoa(expired)
	sig := h.sign(payload)
	token := payload + ":" + sig

	if h.verifySession(token) {
		t.Error("expired session should be rejected")
	}
}

func TestVerifySession_Malformed(t *testing.T) {
	h := newAuthTestHandler()
	cases := []string{
		"",
		"noseparator",
		"only:two",
	}
	for _, c := range cases {
		if h.verifySession(c) {
			t.Errorf("malformed session %q should be rejected", c)
		}
	}
}

// --- AuthMiddleware ---

func TestAuthMiddleware_NoPassword(t *testing.T) {
	h := &Handler{authKey: []byte("key"), authPassword: ""}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.AuthMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("no-auth mode: status = %d, want 200", rec.Code)
	}
}

func TestAuthMiddleware_PublicPaths(t *testing.T) {
	h := newAuthTestHandler()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, path := range []string{"/login", "/static/style.css", "/static/htmx.min.js"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		h.AuthMiddleware(next).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("path %q: status = %d, want 200 (public path)", path, rec.Code)
		}
	}
}

func TestAuthMiddleware_RedirectsWithoutSession(t *testing.T) {
	h := newAuthTestHandler()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.AuthMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("unauthenticated request: status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("redirect location = %q, want /login", loc)
	}
}

func TestAuthMiddleware_ValidSession(t *testing.T) {
	h := newAuthTestHandler()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	session := h.createSession("admin")
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: session})
	rec := httptest.NewRecorder()
	h.AuthMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("valid session: status = %d, want 200", rec.Code)
	}
}

func TestAuthMiddleware_InvalidSession(t *testing.T) {
	h := newAuthTestHandler()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "bad:session:value"})
	rec := httptest.NewRecorder()
	h.AuthMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("invalid session: status = %d, want 302", rec.Code)
	}
}

// itoa converts int64 to string without importing strconv.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
