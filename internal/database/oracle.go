package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/godror/godror" // Oracle Driver // Make sure this import is correct for your setup
	"go-webapi/internal/config"
	"go.uber.org/zap"
)

// InitOracle initializes the Oracle database connection pool.
// It returns the pool handle immediately and relies on database/sql
// for lazy connection establishment and reconnection.
func InitOracle(cfg *config.Config, logger *zap.Logger) (*sql.DB, error) {
	logger.Info("Initializing Oracle database connection pool...")

	// Open the database pool. This usually doesn't establish a connection yet.
	db, err := sql.Open("godror", cfg.OracleConnString) // Ensure driver name "godror" is correct
	if err != nil {
		logger.Error("Failed to open Oracle connection pool", zap.Error(err))
		// Return nil DB and the error
		return nil, fmt.Errorf("failed to configure oracle connection pool: %w", err)
	}

	// Configure connection pool (adjust as needed)
	// These settings control how the pool manages connections over time.
	db.SetMaxOpenConns(20)                  // Max number of open connections
	db.SetMaxIdleConns(5)                   // Max number of connections kept idle
	db.SetConnMaxLifetime(time.Hour)        // Max time a connection can be reused
	db.SetConnMaxIdleTime(10 * time.Minute) // Max time a connection can be idle

	// Optional: Perform an initial ping to check immediate connectivity.
	// This might block briefly. You can skip this if faster startup is critical.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Short timeout
	err = db.PingContext(ctx)
	cancel() // Release context resources

	if err != nil {
		// Log a warning but return the db handle anyway, allowing lazy connection.
		logger.Warn("Initial Oracle DB ping failed, pool created but connection may establish later", zap.Error(err))
		// Return the configured pool handle even if the first ping failed.
		return db, nil
	}

	logger.Info("Oracle database pool initialized and initial ping successful.")
	// Return the configured pool handle.
	return db, nil

	// Removed the internal retry loop and the monitorOracleConnection goroutine.
}
