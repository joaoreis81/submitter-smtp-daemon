package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
)

// DB wraps the SQLite database connection
type DB struct {
	conn   *sql.DB
	logger *logger.Logger
}

// Config contains database configuration
type Config struct {
	Path            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	Logger          *logger.Logger
}

// New creates a new database connection
func New(cfg Config) (*DB, error) {
	// Open database connection
	conn, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Enable WAL mode for better concurrency
	_, err = conn.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	_, err = conn.Exec("PRAGMA foreign_keys=ON")
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	db := &DB{
		conn:   conn,
		logger: cfg.Logger.WithComponent("database"),
	}

	// Initialize schema
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	db.logger.Info("Database initialized successfully")
	return db, nil
}

// initSchema creates all database tables
func (db *DB) initSchema() error {
	_, err := db.conn.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Ping checks if the database connection is alive
func (db *DB) Ping() error {
	return db.conn.Ping()
}

// GetConn returns the underlying database connection
func (db *DB) GetConn() *sql.DB {
	return db.conn
}

// BeginTx starts a new transaction
func (db *DB) BeginTx() (*sql.Tx, error) {
	return db.conn.Begin()
}

// Stats returns database statistics
func (db *DB) Stats() sql.DBStats {
	return db.conn.Stats()
}
