// Package web provides HTTP handlers for the web UI.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"facebook-account-parser/internal/db"
	"facebook-account-parser/internal/sync"
	"facebook-account-parser/internal/token"
)

//go:embed static
var staticFS embed.FS

//go:embed templates
var templatesFS embed.FS

// StaticFS returns the embedded static filesystem.
func StaticFS() (fs.FS, error) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("get static sub-fs: %w", err)
	}
	return sub, nil
}

// Handler handles web UI requests.
type Handler struct {
	store        db.Store
	tokenMgr     *token.Manager
	pipeline     *sync.Pipeline
	tmpl         *template.Template
	authKey      []byte
	authUsername string
	authPassword string
}

// New creates a new Handler.
func New(store db.Store, tokenMgr *token.Manager, pipeline *sync.Pipeline, authKey []byte, authUsername, authPassword string) (*Handler, error) {
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Handler{
		store:        store,
		tokenMgr:     tokenMgr,
		pipeline:     pipeline,
		tmpl:         tmpl,
		authKey:      authKey,
		authUsername: authUsername,
		authPassword: authPassword,
	}, nil
}

// Register registers all routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// Static files
	staticSub, _ := StaticFS()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Auth
	mux.HandleFunc("GET /login", h.handleLoginPage)
	mux.HandleFunc("POST /login", h.handleLoginSubmit)
	mux.HandleFunc("GET /logout", h.handleLogout)

	// Pages
	mux.HandleFunc("GET /{$}", h.handleDashboard)
	mux.HandleFunc("GET /tokens", h.handleTokensPage)
	mux.HandleFunc("GET /accounts", h.handleAccountsPage)
	mux.HandleFunc("GET /spend", h.handleSpendPage)

	// Token CRUD (HTMX partials)
	mux.HandleFunc("GET /web/tokens", h.handleTokenTable)
	mux.HandleFunc("GET /web/tokens/new", h.handleTokenForm)
	mux.HandleFunc("GET /web/tokens/{id}/edit", h.handleTokenEditForm)
	mux.HandleFunc("POST /web/tokens", h.handleTokenCreate)
	mux.HandleFunc("PUT /web/tokens/{id}", h.handleTokenUpdate)
	mux.HandleFunc("DELETE /web/tokens/{id}", h.handleTokenDelete)
	mux.HandleFunc("POST /web/tokens/{id}/sync", h.handleTokenSync)

	// Accounts (HTMX partials)
	mux.HandleFunc("GET /web/accounts", h.handleAccountTable)
	mux.HandleFunc("POST /web/accounts/sync", h.handleAccountsSync)

	// Spend (HTMX partials)
	mux.HandleFunc("GET /web/spend", h.handleSpendTable)
	mux.HandleFunc("POST /web/sync", h.handleSyncRun)

	// Sync logs
	mux.HandleFunc("GET /sync-logs", h.handleSyncLogsPage)
	mux.HandleFunc("GET /web/sync-logs/{id}/errors", h.handleSyncErrors)

	// Dashboard
	mux.HandleFunc("GET /web/dashboard/stats", h.handleDashboardStats)

	// Theme
	mux.HandleFunc("POST /web/theme", h.handleThemeToggle)
}

// templateData holds all data passed to templates.
type templateData struct {
	Theme      string
	ActivePage string
	Error      string
	Success    string

	// dashboard
	Stats *db.DashboardStats

	// tokens
	Tokens []db.Token
	Token  *db.Token

	// accounts
	Accounts      []db.AdAccount
	TokenMap      map[string]string // tokenID → name
	FilterTokenID string            // selected token in the accounts filter

	// spend (aggregated, used by dashboard)
	SpendRows []db.SpendByAccount
	// spend (raw adset-level, used by spend page)
	SpendRawRows []db.SpendRow
	Date         string
	DateFrom     string
	DateTo       string
	AccountID    string

	// pagination
	Page       int
	PageSize   int
	TotalRows  int
	TotalPages int

	// sync logs
	SyncRuns   []db.SyncRun
	SyncErrors []db.SyncRunError
}

func (h *Handler) render(w http.ResponseWriter, name string, data templateData) {
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) renderError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	fmt.Fprintf(w, `<div class="error">%s</div>`, msg)
}

func (h *Handler) getTheme(r *http.Request) string {
	if c, err := r.Cookie("theme"); err == nil {
		return c.Value
	}
	return ""
}

func parseTemplates() (*template.Template, error) {
	funcs := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"min": func(a, b int) int {
			if a < b {
				return a
			}
			return b
		},
		"maskToken": func(s string) string {
			if len(s) <= 8 {
				return "****"
			}
			return s[:4] + "****" + s[len(s)-4:]
		},
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04")
		},
		"formatDate": func(t *time.Time) string {
			if t == nil {
				return "—"
			}
			return t.Format("2006-01-02")
		},
		"formatSpend": func(f float64) string {
			return fmt.Sprintf("%.2f", f)
		},
		"accountStatusLabel": func(status int) string {
			switch status {
			case 1:
				return "Active"
			case 2:
				return "Disabled"
			case 3:
				return "Unsettled"
			case 7:
				return "Pending"
			case 9:
				return "In Grace Period"
			case 100:
				return "Pending Closure"
			case 101:
				return "Closed"
			}
			return "Unknown"
		},
		"accountStatusClass": func(status int) string {
			if status == 1 {
				return "status-active"
			}
			return "status-inactive"
		},
		"runDuration": func(r db.SyncRun) string {
			if r.FinishedAt == nil {
				return "running..."
			}
			d := r.FinishedAt.Sub(r.StartedAt).Round(time.Second)
			if d < time.Minute {
				return fmt.Sprintf("%ds", int(d.Seconds()))
			}
			return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
		},
	}

	tmpl := template.New("").Funcs(funcs)

	partials := []string{
		"layout-start",
		"layout-end",
		"nav",
		"dashboard-stats",
		"token-table",
		"token-form",
		"account-table",
		"spend-table",
		"spend-raw-table",
		"status-badge",
		"sync-logs-table",
		"sync-errors-list",
	}

	for _, name := range partials {
		content, err := templatesFS.ReadFile("templates/partials/" + name + ".html")
		if err != nil {
			return nil, fmt.Errorf("read partial %s: %w", name, err)
		}
		if _, err := tmpl.New(name).Parse(string(content)); err != nil {
			return nil, fmt.Errorf("parse partial %s: %w", name, err)
		}
	}

	pages := []string{
		"dashboard.html",
		"tokens.html",
		"accounts.html",
		"spend.html",
		"sync-logs.html",
		"login.html",
	}

	for _, name := range pages {
		content, err := templatesFS.ReadFile("templates/" + name)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", name, err)
		}
		if _, err := tmpl.New(name).Parse(string(content)); err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
	}

	return tmpl, nil
}
