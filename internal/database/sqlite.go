package database

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3" // SQLite Driver
	"go-webapi/internal/config"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"time"
)

const createLogTableSQL = `
CREATE TABLE IF NOT EXISTS tbl_log (
id INTEGER PRIMARY KEY AUTOINCREMENT,
timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
level TEXT NOT NULL,
message TEXT NOT NULL,
fields TEXT -- Store additional zap fields as JSON string
);
`

// InitSQLite initializes the SQLite database connection and ensures the log table exists.
// It now also creates the necessary directory path if it does not exist.
func InitSQLite(cfg *config.Config, logger *zap.Logger) (*sql.DB, error) {
	logger.Info("Initializing SQLite database...", zap.String("requested_path", cfg.SQLiteDBPath))

	// --- Ensure Directory Exists ---
	dbDir := filepath.Dir(cfg.SQLiteDBPath) // Get the directory part of the path
	if dbDir != "." && dbDir != "/" {       // Avoid trying to create "." or "/"
		logger.Debug("Checking if SQLite directory exists", zap.String("path", dbDir))
		if _, err := os.Stat(dbDir); os.IsNotExist(err) {
			logger.Info("SQLite database directory does not exist, creating...", zap.String("path", dbDir))
			// Create the directory and any necessary parent directories (0755 permissions)
			if err := os.MkdirAll(dbDir, 0755); err != nil {
				logger.Error("Failed to create SQLite database directory", zap.String("path", dbDir), zap.Error(err))
				return nil, fmt.Errorf("failed to create sqlite db directory %s: %w", dbDir, err)
			}
			logger.Info("SQLite database directory created successfully", zap.String("path", dbDir))
		} else if err != nil {
			// Handle other potential errors from os.Stat (e.g., permission denied)
			logger.Error("Failed to check status of SQLite database directory", zap.String("path", dbDir), zap.Error(err))
			return nil, fmt.Errorf("failed to check status of sqlite db directory %s: %w", dbDir, err)
		} else {
			logger.Debug("SQLite directory already exists", zap.String("path", dbDir))
		}
	}
	// --- Directory should now exist ---

	// Open the database connection (file will be created if it doesn't exist)
	logger.Info("Opening SQLite database connection...", zap.String("path", cfg.SQLiteDBPath))
	db, err := sql.Open("sqlite3", cfg.SQLiteDBPath+"?_journal_mode=WAL&_busy_timeout=5000") // WAL mode is generally good for concurrent reads/writes
	if err != nil {
		logger.Error("Failed to open SQLite database", zap.String("path", cfg.SQLiteDBPath), zap.Error(err))
		return nil, fmt.Errorf("failed to open sqlite database at %s: %w", cfg.SQLiteDBPath, err)
	}

	// Configure connection pool (optional but good practice)
	db.SetMaxOpenConns(1) // Usually 1 is sufficient for logging writes
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close() // Close if ping fails
		logger.Error("Failed to ping SQLite database after open", zap.Error(err))
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}
	logger.Debug("SQLite ping successful.")

	// Ensure log table exists
	if _, err := db.Exec(createLogTableSQL); err != nil {
		db.Close() // Close if table creation fails
		logger.Error("Failed to create tbl_log in SQLite", zap.Error(err))
		return nil, fmt.Errorf("failed to create sqlite table tbl_log: %w", err)
	}
	logger.Debug("SQLite tbl_log verified/created.")

	logger.Info("SQLite database initialized successfully", zap.String("path", cfg.SQLiteDBPath))
	return db, nil
}
