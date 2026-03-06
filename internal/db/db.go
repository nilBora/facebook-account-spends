package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens a SQLite database and runs any pending migrations.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on&_timeout=5000", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite: single writer

	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

// runMigrations applies all pending .sql migration files in order.
func runMigrations(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, filename := range files {
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE filename = ?`, filename).Scan(&count)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", filename, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + filename)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", filename, err)
		}

		// Disable FK enforcement before the transaction so structural
		// migrations (table drops/renames) can run without FK violations.
		if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
			return fmt.Errorf("disable fk for migration %s: %w", filename, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", filename, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
			return fmt.Errorf("execute migration %s: %w", filename, err)
		}

		if _, err := tx.Exec(`INSERT INTO schema_migrations(filename) VALUES(?)`, filename); err != nil {
			_ = tx.Rollback()
			_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
			return fmt.Errorf("record migration %s: %w", filename, err)
		}

		if err := tx.Commit(); err != nil {
			_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
			return fmt.Errorf("commit migration %s: %w", filename, err)
		}

		if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
			return fmt.Errorf("re-enable fk after migration %s: %w", filename, err)
		}
	}

	return nil
}
