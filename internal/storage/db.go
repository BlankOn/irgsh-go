package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection
type DB struct {
	*sql.DB
}

// NewDB creates a new SQLite database connection with WAL mode enabled
func NewDB(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database with WAL mode and other optimizations
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for SQLite (single writer)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	wrappedDB := &DB{DB: db}

	// Initialize schema
	if err := wrappedDB.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return wrappedDB, nil
}

// initSchema creates the database tables if they don't exist, then brings
// existing tables up to date.
func (db *DB) initSchema() error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	return db.applyMigrations()
}

// applyMigrations adds columns that were introduced after a database was
// created. CREATE TABLE IF NOT EXISTS leaves an existing table untouched, so
// a chief that has already run cannot pick up a new column without this.
func (db *DB) applyMigrations() error {
	for _, migration := range columnMigrations {
		exists, err := db.columnExists(migration.table, migration.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", migration.table, migration.column, migration.definition)
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to add %s.%s: %w", migration.table, migration.column, err)
		}
	}
	return nil
}

// columnExists reports whether a table already has a column.
func (db *DB) columnExists(table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("failed to inspect %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			ctype      string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultVal, &pk); err != nil {
			return false, fmt.Errorf("failed to inspect %s: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}
