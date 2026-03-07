// Package store implements SQLite + sqlite-vec storage operations for Cerebro.
// It handles schema management, node/edge CRUD, vector search, and composite scoring.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	sqlite_vec.Auto()
}

// Store wraps a SQLite database with vector search capabilities.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens an existing brain database at the given path.
// Returns an error if the database does not exist.
func Open(path string) (*Store, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("brain not found at %s — run 'cerebro init' first", path)
	}
	return open(path)
}

// Init creates and initializes a new brain database at the given path.
// If the database already exists, it validates the schema version.
func Init(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating directory %s: %w", dir, err)
	}

	s, err := open(path)
	if err != nil {
		return nil, err
	}

	if err := s.applySchema(); err != nil {
		s.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	return s, nil
}

func open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON&_cache_size=-65536", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	return &Store{db: db, path: path}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB returns the underlying *sql.DB for advanced operations.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Path returns the file path of the database.
func (s *Store) Path() string {
	return s.path
}
