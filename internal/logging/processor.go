package logging

import (
	"context"
	"errors"
	"time"

	"go-webapi/internal/config"
	"go-webapi/internal/database"
	"go-webapi/internal/repositories"

	"go.uber.org/zap"
)

// LogProcessor handles the transfer of logs from SQLite to Oracle
type LogProcessor struct {
	cfg        *config.Config
	logRepo    repositories.LogRepository
	fileLogger *zap.Logger // Renamed from logger
	ticker     *time.Ticker
	stopChan   chan struct{}
	isRunning  bool
}

// NewLogProcessor creates a new LogProcessor instance
func NewLogProcessor(cfg *config.Config, logRepo repositories.LogRepository, fileLogger *zap.Logger) *LogProcessor {
	return &LogProcessor{
		cfg:        cfg,
		logRepo:    logRepo,
		fileLogger: fileLogger,
		stopChan:   make(chan struct{}),
	}
}

// Start begins the log processing loop in a separate goroutine
func (p *LogProcessor) Start() {
	if p.isRunning {
		p.fileLogger.Warn("Log processor already running")
		return
	}
	p.ticker = time.NewTicker(p.cfg.LogBatchInterval)
	p.isRunning = true
	go p.run()
	p.fileLogger.Info("SQLite to Oracle log processor started",
		zap.Duration("interval", p.cfg.LogBatchInterval),
		zap.Int("batchSize", p.cfg.LogProcessorBatchSize),
		zap.Int("retryAttempts", p.cfg.LogProcessorOracleRetryAttempts),
		zap.Int("retryDelaySec", p.cfg.LogProcessorOracleRetryDelaySeconds),
	)
}

// Stop signals the log processing loop to terminate gracefully
func (p *LogProcessor) Stop() {
	if !p.isRunning {
		p.fileLogger.Warn("Log processor not running")
		return
	}
	p.fileLogger.Info("Stopping SQLite to Oracle log processor...")
	select {
	case <-p.stopChan:
	default:
		close(p.stopChan)
	}
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.isRunning = false
	// Short delay to allow run() goroutine to exit if it was in ticker.C case
	time.Sleep(500 * time.Millisecond)
	p.fileLogger.Info("Processing final log batch before shutdown...")
	oracleRetryDelayDuration := time.Duration(p.cfg.LogProcessorOracleRetryDelaySeconds) * time.Second
	// Give ample time for the final batch, considering retries.
	// E.g., (max_retries + 1) * retry_delay + some_buffer
	shutdownCtxTimeout := time.Duration(p.cfg.LogProcessorOracleRetryAttempts+1)*oracleRetryDelayDuration + (15 * time.Second)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownCtxTimeout)
	defer cancel()
	p.processBatch(shutdownCtx) // Process one last time
	p.fileLogger.Info("Log processor stopped.")
}

// run is the main loop that periodically processes log batches
func (p *LogProcessor) run() {
	for {
		select {
		case <-p.ticker.C:
			select {
			case <-p.stopChan: // Check stopChan before processing
				p.fileLogger.Info("Stop signal received before processing tick, exiting loop.")
				return
			default:
				// Use a context for the batch processing that is shorter than the tick interval
				// to prevent overlap if a batch takes too long.
				tickCtxTimeout := p.cfg.LogBatchInterval - (5 * time.Second) // Give 5s buffer before next tick
				if tickCtxTimeout <= 0 {
					tickCtxTimeout = 30 * time.Second // Default if interval is too short
				}
				tickCtx, cancel := context.WithTimeout(context.Background(), tickCtxTimeout)
				p.processBatch(tickCtx)
				cancel()
			}
		case <-p.stopChan:
			p.fileLogger.Info("Received stop signal, exiting log processing loop.")
			return
		}
	}
}

// processBatch fetches logs from SQLite, attempts to insert them into Oracle.
func (p *LogProcessor) processBatch(ctx context.Context) {
	p.fileLogger.Debug("Processing log batch...")

	oracleRetryDelayDuration := time.Duration(p.cfg.LogProcessorOracleRetryDelaySeconds) * time.Second

	// 1. Get logs from SQLite
	logs, err := p.logRepo.GetSQLiteLogs(ctx, p.cfg.LogProcessorBatchSize)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.fileLogger.Info("Context cancelled/timed out during SQLite fetch.", zap.Error(err))
		} else {
			p.fileLogger.Error("Failed to get logs from SQLite", zap.Error(err))
		}
		return
	}
	if len(logs) == 0 {
		p.fileLogger.Debug("No logs in SQLite to process")
		return
	}
	p.fileLogger.Debug("Fetched logs from SQLite", zap.Int("count", len(logs)))

	// 2. Attempt to insert logs into Oracle with init/retry logic
	var insertErr error
	success := false
	for attempt := 1; attempt <= p.cfg.LogProcessorOracleRetryAttempts; attempt++ {
		if ctx.Err() != nil { // Check context before attempt
			insertErr = ctx.Err()
			p.fileLogger.Info("Context cancelled before Oracle insert attempt.", zap.Int("attempt", attempt), zap.Error(insertErr))
			break
		}
		select { // Check stop channel also
		case <-p.stopChan:
			insertErr = errors.New("processor stopped during oracle insert attempt")
			p.fileLogger.Info(insertErr.Error(), zap.Int("attempt", attempt))
			goto StopProcessing // Use goto to break outer loop if needed
		default:
		}

		// Attempt insert
		// Give each insert attempt its own timeout, shorter than the overall batch ctx timeout
		insertCtx, cancelInsert := context.WithTimeout(ctx, oracleRetryDelayDuration+15*time.Second) // e.g., retry_delay + 15s processing window
		insertErr = p.logRepo.InsertBatchOracle(insertCtx, logs)
		cancelInsert()

		if insertErr == nil {
			p.fileLogger.Debug("Successfully inserted log batch into Oracle", zap.Int("count", len(logs)), zap.Int("attempt", attempt))
			success = true
			break // Exit retry loop on success
		}

		// --- Handle Insert Error ---
		isConnError := errors.Is(insertErr, repositories.ErrOracleConnection)

		if ctx.Err() != nil { // Check context again after failed attempt
			insertErr = ctx.Err() // Prioritize context error
			p.fileLogger.Info("Context cancelled after failed Oracle insert attempt.", zap.Int("attempt", attempt), zap.Error(insertErr))
			break
		}
		select {
		case <-p.stopChan:
			insertErr = errors.New("processor stopped after failed oracle insert attempt")
			p.fileLogger.Info(insertErr.Error(), zap.Int("attempt", attempt))
			goto StopProcessing
		default:
		}

		if isConnError && attempt < p.cfg.LogProcessorOracleRetryAttempts {
			p.fileLogger.Warn("Oracle insert failed (connection issue), attempting recovery and retry.",
				zap.Error(insertErr),
				zap.Int("attempt_failed", attempt),
				zap.Int("next_attempt", attempt+1),
				zap.Int("max_attempts", p.cfg.LogProcessorOracleRetryAttempts),
			)
			// Try to re-initialize Oracle connection via the logRepo or a shared mechanism
			// This part depends on how you want to handle re-establishing the Oracle connection globally or for the repo.
			// For this example, we assume logRepo.InsertBatchOracle internally signals ErrOracleConnection
			// and we might try to refresh the connection at a higher level or have the repo manage it.
			// Here, we'll just log and wait.
			// A more robust solution might involve the processor asking a central DB manager to check/refresh the connection
			// and then telling logRepo to use the new handle.
			// The current logRepo.SetOracleDB is helpful if a new DB handle is obtained elsewhere.

			// Simplified: try re-init and update repo's DB
			p.fileLogger.Info("Processor attempting to initialize/re-initialize Oracle connection for LogRepository...")
			// The InitOracle function needs a logger; pass the processor's fileLogger.
			newDb, initErr := database.InitOracle(p.cfg, p.fileLogger) // Assuming InitOracle is safe to call multiple times
			if initErr == nil && newDb != nil {
				pingCtx, cancelPing := context.WithTimeout(context.Background(), 10*time.Second)
				pingErr := newDb.PingContext(pingCtx)
				cancelPing()
				if pingErr == nil {
					p.fileLogger.Info("Successfully established/verified Oracle connection via processor's attempt.", zap.Int("attempt_failed", attempt))
					p.logRepo.SetOracleDB(newDb) // Update the LogRepository's DB handle
					p.fileLogger.Warn("Processor updated shared Oracle DB handle in LogRepository.")
				} else {
					p.fileLogger.Error("Processor initialized Oracle handle, but immediate ping failed.", zap.Error(pingErr), zap.Int("attempt_failed", attempt))
					newDb.Close() // Close the newly created but non-functional DB handle
				}
			} else {
				if newDb != nil {
					newDb.Close()
				} // Ensure it's closed if init partially succeeded but returned error
				p.fileLogger.Error("Processor failed to re-initialize Oracle connection.", zap.Error(initErr), zap.Int("attempt_failed", attempt))
			}

			p.fileLogger.Info("Waiting before next Oracle insert attempt...", zap.Duration("retry_delay", oracleRetryDelayDuration))
			select {
			case <-time.After(oracleRetryDelayDuration):
				// continue to next attempt
			case <-ctx.Done():
				p.fileLogger.Info("Parent context cancelled during Oracle retry wait.", zap.Error(ctx.Err()))
				insertErr = ctx.Err() // Ensure insertErr reflects the context cancellation
				goto StopProcessing   // Break outer loop
			case <-p.stopChan:
				p.fileLogger.Info("Stop signal received during Oracle retry wait.")
				insertErr = errors.New("processor stopped during retry wait")
				goto StopProcessing // Break outer loop
			}
		} else {
			if isConnError { // Max retries reached for connection error
				p.fileLogger.Error("Oracle insert failed after max retries for connection issue.",
					zap.Error(insertErr),
					zap.Int("attempts_made", attempt),
					zap.Int("max_attempts", p.cfg.LogProcessorOracleRetryAttempts),
				)
			} else { // Non-connection error or other non-retryable error
				p.fileLogger.Error("Oracle insert failed (non-connection/non-retryable or final attempt for other error).",
					zap.Error(insertErr),
					zap.Int("attempt_failed", attempt),
				)
			}
			break // Exit retry loop
		}
	}
StopProcessing: // Label for goto

	if !success {
		p.fileLogger.Warn("Failed to insert log batch into Oracle after all attempts or due to stop/cancellation.", zap.Error(insertErr), zap.Int("log_count", len(logs)))
		return // Do NOT delete from SQLite
	}

	// 4. Delete logs from SQLite (only if Oracle insert succeeded)
	logIDs := make([]int64, len(logs))
	for i, log := range logs {
		logIDs[i] = log.ID
	}

	// Use a new context for deletion as the previous one might have timed out or been cancelled.
	// However, if the overall batch context (ctx) is already done, we should respect that.
	if ctx.Err() != nil {
		p.fileLogger.Warn("Skipping SQLite delete because parent context is done.", zap.Error(ctx.Err()), zap.Int("count", len(logIDs)))
		return
	}
	deleteCtx, cancelDelete := context.WithTimeout(context.Background(), 10*time.Second) // Short timeout for delete
	defer cancelDelete()

	err = p.logRepo.DeleteSQLiteLogsByID(deleteCtx, logIDs)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.fileLogger.Warn("Context cancelled/timed out during SQLite delete.", zap.Error(err), zap.Int("count", len(logIDs)))
		} else {
			p.fileLogger.Error("CRITICAL: Failed to delete logs from SQLite after successful Oracle insert.", zap.Error(err), zap.Int64s("log_ids", logIDs))
		}
		return
	}

	p.fileLogger.Info("Processed and transferred log batch", zap.Int("count", len(logs)))
}
