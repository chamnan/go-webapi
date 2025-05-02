package logging

import (
	"context"
	"errors"
	"time"

	"go-webapi/internal/config"
	"go-webapi/internal/database" // Import database package
	"go-webapi/internal/repositories"

	"go.uber.org/zap"
)

const (
	batchSize           = 100              // Number of logs to fetch and insert in one batch
	oracleRetryAttempts = 3                // Max attempts to insert THIS BATCH if Oracle connection fails
	oracleRetryDelay    = 30 * time.Second // Retry delay for THIS BATCH after connection error
)

// LogProcessor handles the transfer of logs from SQLite to Oracle
type LogProcessor struct {
	cfg       *config.Config // Keep config for InitOracle
	logRepo   repositories.LogRepository
	logger    *zap.Logger
	ticker    *time.Ticker
	stopChan  chan struct{}
	isRunning bool
}

// NewLogProcessor creates a new LogProcessor instance
func NewLogProcessor(cfg *config.Config, logRepo repositories.LogRepository, logger *zap.Logger) *LogProcessor {
	return &LogProcessor{
		cfg:      cfg,
		logRepo:  logRepo,
		logger:   logger,
		stopChan: make(chan struct{}),
	}
}

// Start begins the log processing loop in a separate goroutine
func (p *LogProcessor) Start() {
	if p.isRunning {
		p.logger.Warn("Log processor already running")
		return
	}
	p.ticker = time.NewTicker(p.cfg.LogBatchInterval) // Main interval for checking SQLite
	p.isRunning = true
	go p.run()
	p.logger.Info("SQLite to Oracle log processor started", zap.Duration("interval", p.cfg.LogBatchInterval))
}

// Stop signals the log processing loop to terminate gracefully
func (p *LogProcessor) Stop() {
	if !p.isRunning {
		p.logger.Warn("Log processor not running")
		return
	}
	p.logger.Info("Stopping SQLite to Oracle log processor...")
	// Ensure stopChan is valid before closing
	select {
	case <-p.stopChan:
		// Already closed
	default:
		close(p.stopChan)
	}
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.isRunning = false
	// Wait briefly allows run() goroutine to exit gracefully after receiving signal
	time.Sleep(500 * time.Millisecond)
	p.logger.Info("Processing final log batch before shutdown...")
	// Allow slightly more time than a single retry delay
	shutdownCtx, cancel := context.WithTimeout(context.Background(), oracleRetryDelay+5*time.Second)
	defer cancel()
	p.processBatch(shutdownCtx) // Attempt one last batch
	p.logger.Info("Log processor stopped.")
}

// run is the main loop that periodically processes log batches
func (p *LogProcessor) run() {
	for {
		select {
		case <-p.ticker.C:
			// Check if stopped before processing
			select {
			case <-p.stopChan:
				p.logger.Info("Stop signal received before processing tick, exiting loop.")
				return
			default:
				// Continue processing
			}
			// Use a context for the entire batch processing attempt for this tick
			tickCtx, cancel := context.WithTimeout(context.Background(), p.cfg.LogBatchInterval-1*time.Second)
			p.processBatch(tickCtx)
			cancel()
		case <-p.stopChan:
			p.logger.Info("Received stop signal, exiting log processing loop.")
			return
		}
	}
}

// processBatch fetches logs from SQLite, attempts to insert them into Oracle.
// If Oracle connection handle is nil or insert fails due to connection issue,
// it attempts to initialize/reinitialize the connection and updates the repo.
func (p *LogProcessor) processBatch(ctx context.Context) {
	p.logger.Debug("Processing log batch...")

	// 1. Get logs from SQLite
	logs, err := p.logRepo.GetSQLiteLogs(ctx, batchSize)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.logger.Info("Context cancelled/timed out during SQLite fetch.", zap.Error(err))
		} else {
			p.logger.Error("Failed to get logs from SQLite", zap.Error(err))
		}
		return
	}
	if len(logs) == 0 {
		p.logger.Debug("No logs in SQLite to process")
		return
	}
	p.logger.Debug("Fetched logs from SQLite", zap.Int("count", len(logs)))

	// 2. Attempt to insert logs into Oracle with init/retry logic
	var insertErr error
	success := false
	for attempt := 1; attempt <= oracleRetryAttempts; attempt++ {
		// Check context before attempt
		if ctx.Err() != nil {
			p.logger.Info("Context cancelled before Oracle insert attempt.", zap.Error(ctx.Err()))
			insertErr = ctx.Err()
			break
		}
		select {
		case <-p.stopChan:
			p.logger.Info("Stop signal received before Oracle insert attempt.")
			insertErr = errors.New("processor stopped")
			goto StopProcessing // Use goto to break outer logic flow cleanly
		default:
		}

		// --- Attempt insert ---
		// Use a shorter timeout for the actual DB operation within the overall tick context
		insertCtx, cancelInsert := context.WithTimeout(ctx, 45*time.Second) // Timeout for this attempt
		insertErr = p.logRepo.InsertBatchOracle(insertCtx, logs)
		cancelInsert() // Cancel insert context immediately after return

		if insertErr == nil {
			p.logger.Debug("Successfully inserted log batch into Oracle", zap.Int("count", len(logs)), zap.Int("attempt", attempt))
			success = true
			break // Exit retry loop
		}

		// --- Handle Insert Error ---
		isConnError := errors.Is(insertErr, repositories.ErrOracleConnection)

		// Check context/stop signal again after failed attempt
		if ctx.Err() != nil {
			p.logger.Info("Context cancelled after Oracle insert attempt failed.", zap.Error(ctx.Err()))
			insertErr = ctx.Err()
			break
		}
		select {
		case <-p.stopChan:
			p.logger.Info("Stop signal received after Oracle insert attempt failed.")
			insertErr = errors.New("processor stopped")
			goto StopProcessing
		default:
		}

		// --- Decide whether to retry or give up ---
		if isConnError && attempt <= oracleRetryAttempts {
			p.logger.Warn("Oracle insert failed (connection issue), attempting to establish/verify connection before retry...",
				zap.Error(insertErr),
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", oracleRetryAttempts),
			)

			// === Attempt to Init/Re-init Oracle Connection ===
			// Call simplified InitOracle which just sets up pool handle
			p.logger.Info("Processor attempting to initialize/re-initialize Oracle connection...")
			newDb, initErr := database.InitOracle(p.cfg, p.logger) // Assumes InitOracle is quick

			if initErr == nil && newDb != nil {
				// Ping the new handle to be more certain before updating
				pingCtx, cancelPing := context.WithTimeout(context.Background(), 10*time.Second)
				pingErr := newDb.PingContext(pingCtx)
				cancelPing()

				if pingErr == nil {
					p.logger.Info("Successfully established/verified Oracle connection via processor.", zap.Int("attempt", attempt))
					// Update the SHARED repository handle - REQUIRES SetOracleDB on repo + mutex
					p.logRepo.SetOracleDB(newDb)
					p.logger.Warn("Processor updated shared Oracle DB handle. Ensure other components (e.g., UserRepository) are aware or updated if necessary.")
				} else {
					p.logger.Error("Processor initialized Oracle handle, but immediate ping failed.", zap.Error(pingErr), zap.Int("attempt", attempt))
					newDb.Close() // Close the handle if ping failed
				}
			} else {
				//p.logger.Error("Processor failed to initialize Oracle connection", zap.Error(initErr), zap.Int("attempt", attempt))
				if newDb != nil {
					newDb.Close()
				} // Close if Open succeeded but Ping failed within InitOracle
			}
			// === End Connection Attempt ===

			p.logger.Info("Waiting before retrying Oracle insert...", zap.Duration("retry_delay", oracleRetryDelay))
			// Wait for the specific retry delay, respecting context cancellation and stopChan
			select {
			case <-time.After(oracleRetryDelay):
				continue // Retry the insert attempt
			case <-ctx.Done():
				p.logger.Info("Parent context cancelled during Oracle retry wait.", zap.Error(ctx.Err()))
				insertErr = ctx.Err()
				goto StopProcessing
			case <-p.stopChan:
				p.logger.Info("Stop signal received during Oracle retry wait.")
				insertErr = errors.New("processor stopped")
				goto StopProcessing
			}
		} else { // Error is not connection error OR max retries reached
			if isConnError {
				p.logger.Error("Oracle insert failed after max retries for connection issue", zap.Error(insertErr), zap.Int("attempts", attempt))
			} else {
				p.logger.Error("Oracle insert failed (non-connection/non-retryable error?)", zap.Error(insertErr), zap.Int("attempt", attempt))
			}
			break // Exit retry loop
		}
	}
StopProcessing: // Label for goto

	// 3. Check if insert eventually succeeded
	if !success {
		p.logger.Warn("Failed to insert log batch into Oracle after all attempts or cancellation/stop", zap.Error(insertErr), zap.Int("log_count", len(logs)))
		return // Do NOT delete from SQLite
	}

	// 4. Delete logs from SQLite (only if Oracle insert succeeded)
	logIDs := make([]int64, len(logs))
	for i, log := range logs {
		logIDs[i] = log.ID
	}

	// Reuse parent ctx for deletion
	err = p.logRepo.DeleteSQLiteLogsByID(ctx, logIDs)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.logger.Warn("Context cancelled/timed out during SQLite delete.", zap.Error(err), zap.Int("count", len(logIDs)))
		} else {
			p.logger.Error("CRITICAL: Failed to delete logs from SQLite after successful Oracle insert. Logs are duplicated.", zap.Error(err), zap.Int64s("log_ids", logIDs))
		}
		return
	}

	p.logger.Info("Processed and transferred log batch", zap.Int("count", len(logs)))
}
