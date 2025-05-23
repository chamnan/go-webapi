package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go-webapi/internal/config"

	_ "github.com/godror/godror"
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

	// --- Configure connection pool
	logger.Debug("Applying Oracle pool settings",
		zap.Int("MaxOpenConns", cfg.OracleMaxPoolOpenConns),
		zap.Int("MaxIdleConns", cfg.OracleMaxPoolIdleConns),
		zap.Int("ConnMaxLifetimeMinutes", cfg.OracleMaxPoolConnLifetimeMinutes),
		zap.Int("ConnMaxIdleTimeMinutes", cfg.OracleMaxPoolConnIdleTimeMinutes),
	)
	db.SetMaxOpenConns(cfg.OracleMaxPoolOpenConns)
	db.SetMaxIdleConns(cfg.OracleMaxPoolIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.OracleMaxPoolConnLifetimeMinutes) * time.Minute)
	db.SetConnMaxIdleTime(time.Duration(cfg.OracleMaxPoolConnIdleTimeMinutes) * time.Minute)
	// --- End Pool Configuration ---

	// Optional: Perform an initial ping to check immediate connectivity.
	// This might block briefly. You can skip this if faster startup is critical.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Short timeout
	err = db.PingContext(ctx)
	cancel() // Release context resources

	if err != nil {
		// Log a Error but return the db handle anyway, allowing lazy connection.
		logger.Error("Initial Oracle DB ping failed, pool created but connection may establish later", zap.Error(err))
		// Return the configured pool handle even if the first ping failed.
		return db, nil
	}

	logger.Info("Oracle database pool initialized and initial ping successful.")
	// Return the configured pool handle.
	return db, nil

}
