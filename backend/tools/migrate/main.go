package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	var (
		dir   string
		dsn   string
		steps int
	)
	flag.StringVar(&dir, "dir", "deploy/migrations", "path to migrations directory")
	flag.StringVar(&dsn, "dsn", "", "database connection string (overrides DSN env var)")
	flag.IntVar(&steps, "steps", 0, "number of migrations to apply (0 = all)")
	flag.Parse()

	command := flag.Arg(0)
	if command == "" {
		command = "up"
	}

	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		dsn = "postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ping database: %v\n", err)
		os.Exit(1)
	}

	// Ensure schema_migrations table exists
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create schema_migrations table: %v\n", err)
		os.Exit(1)
	}

	switch command {
	case "up":
		if err := migrateUp(db, dir, steps); err != nil {
			fmt.Fprintf(os.Stderr, "migration up failed: %v\n", err)
			os.Exit(1)
		}
	case "down":
		if err := migrateDown(db, dir, steps); err != nil {
			fmt.Fprintf(os.Stderr, "migration down failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s (use 'up' or 'down')\n", command)
		os.Exit(1)
	}
}

func migrateUp(db *sql.DB, dir string, steps int) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(files)

	applied := 0
	for _, f := range files {
		base := filepath.Base(f)
		version := strings.TrimSuffix(base, ".up.sql")

		var exists bool
		if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists {
			continue
		}

		content, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}

		fmt.Printf("applied: %s\n", version)
		applied++

		if steps > 0 && applied >= steps {
			break
		}
	}

	if applied == 0 {
		fmt.Println("no pending migrations")
	}
	return nil
}

func migrateDown(db *sql.DB, dir string, steps int) error {
	if steps == 0 {
		steps = 1 // default: roll back one migration
	}

	// Get applied migrations in reverse order
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version DESC")
	if err != nil {
		return fmt.Errorf("query migrations: %w", err)
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("scan migration: %w", err)
		}
		versions = append(versions, v)
	}

	reverted := 0
	for _, version := range versions {
		if reverted >= steps {
			break
		}

		downFile := filepath.Join(dir, version+".down.sql")
		content, err := os.ReadFile(downFile)
		if err != nil {
			return fmt.Errorf("read down migration %s: %w", version, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for rollback %s: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute rollback %s: %w", version, err)
		}

		if _, err := tx.Exec("DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("remove migration record %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit rollback %s: %w", version, err)
		}

		fmt.Printf("reverted: %s\n", version)
		reverted++
	}

	if reverted == 0 {
		fmt.Println("no migrations to revert")
	}
	return nil
}
