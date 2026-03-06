package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var sqliteMigrationsFS embed.FS

//go:embed migrations_pg/*.sql
var pgMigrationsFS embed.FS

// Open opens a database from the given DSN and runs pending migrations.
// DSN starting with "postgres://" or "postgresql://" uses PostgreSQL;
// everything else is treated as a SQLite file path.
func Open(dsn string) (*sql.DB, string, error) {
	driver := detectDriver(dsn)

	var (
		db  *sql.DB
		err error
	)

	switch driver {
	case "postgres":
		db, err = sql.Open("pgx", dsn)
		if err == nil {
			db.SetMaxOpenConns(20)
			db.SetMaxIdleConns(5)
			db.SetConnMaxLifetime(5 * time.Minute)
		}
	default:
		connStr := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on&_timeout=5000", dsn)
		db, err = sql.Open("sqlite", connStr)
		if err == nil {
			db.SetMaxOpenConns(1) // SQLite: single writer
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf("open database: %w", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		return nil, "", fmt.Errorf("ping database: %w", err)
	}

	migrationsFS, migrationsDir := sqliteMigrationsFS, "migrations"
	if driver == "postgres" {
		migrationsFS, migrationsDir = pgMigrationsFS, "migrations_pg"
	}

	if err := runMigrations(db, driver, migrationsFS, migrationsDir); err != nil {
		return nil, "", fmt.Errorf("run migrations: %w", err)
	}

	return db, driver, nil
}

// detectDriver returns "postgres" for PostgreSQL DSNs, "sqlite" otherwise.
func detectDriver(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return "postgres"
	}
	return "sqlite"
}

// runMigrations applies all pending .sql migration files in order.
func runMigrations(db *sql.DB, driver string, migrFS embed.FS, migrDir string) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrFS, migrDir)
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
		err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE filename = ?`,
			filename).Scan(&count)
		if err != nil {
			// PostgreSQL uses $1 placeholder.
			err = db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE filename = $1`,
				filename).Scan(&count)
		}
		if err != nil {
			return fmt.Errorf("check migration %s: %w", filename, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrFS.ReadFile(migrDir + "/" + filename)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", filename, err)
		}

		// For SQLite: disable FK enforcement so structural migrations
		// (table drops/renames) can run without FK violations.
		if driver == "sqlite" {
			if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
				return fmt.Errorf("disable fk for migration %s: %w", filename, err)
			}
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", filename, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			if driver == "sqlite" {
				_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
			}
			return fmt.Errorf("execute migration %s: %w", filename, err)
		}

		insertSQL := `INSERT INTO schema_migrations(filename) VALUES(?)`
		if driver == "postgres" {
			insertSQL = `INSERT INTO schema_migrations(filename) VALUES($1)`
		}
		if _, err := tx.Exec(insertSQL, filename); err != nil {
			_ = tx.Rollback()
			if driver == "sqlite" {
				_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
			}
			return fmt.Errorf("record migration %s: %w", filename, err)
		}

		if err := tx.Commit(); err != nil {
			if driver == "sqlite" {
				_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
			}
			return fmt.Errorf("commit migration %s: %w", filename, err)
		}

		if driver == "sqlite" {
			if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
				return fmt.Errorf("re-enable fk after migration %s: %w", filename, err)
			}
		}
	}

	return nil
}
