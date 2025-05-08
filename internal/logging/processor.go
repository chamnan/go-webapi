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
		// Already closed, maybe called Stop() twice?
		p.fileLogger.Warn("Stop channel already closed for log processor.")
		return
	default:
		close(p.stopChan) // Signal the loop to stop
	}
	if p.ticker != nil {
		p.ticker.Stop() // Stop the ticker
	}
	p.isRunning = false // Mark as not running

	// Allow some time for the run() goroutine to potentially exit cleanly if it was processing a tick
	// This helps avoid race conditions if processBatch was running concurrently.
	time.Sleep(500 * time.Millisecond)

	// --- Added Diagnostic Logging ---
	p.fileLogger.Info("Processing final log batch before shutdown...")
	finalBatchStartTime := time.Now() // Record start time
	// --------------------------------

	oracleRetryDelayDuration := time.Duration(p.cfg.LogProcessorOracleRetryDelaySeconds) * time.Second
	// Give ample time for the final batch, considering retries.
	shutdownCtxTimeout := time.Duration(p.cfg.LogProcessorOracleRetryAttempts+1)*oracleRetryDelayDuration + (15 * time.Second)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownCtxTimeout)
	defer cancel()

	// Process one last time
	p.processBatch(shutdownCtx)

	// --- Added Diagnostic Logging ---
	finalBatchDuration := time.Since(finalBatchStartTime)
	p.fileLogger.Info("Final log batch processing complete.", zap.Duration("duration", finalBatchDuration))
	// --------------------------------

	p.fileLogger.Info("Log processor stopped.")
}

// run is the main loop that periodically processes log batches
func (p *LogProcessor) run() {
	defer p.fileLogger.Info("Log processor run() goroutine finished.") // Log when loop exits
	for {
		select {
		case <-p.ticker.C:
			select {
			case <-p.stopChan: // Check stopChan before processing
				p.fileLogger.Info("Stop signal received before processing tick, exiting loop.")
				return
			default:
				// Use a context for the batch processing that is shorter than the tick interval
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
	// Add a defer function to catch panics within processBatch if they occur
	defer func() {
		if r := recover(); r != nil {
			p.fileLogger.Error("Panic recovered within processBatch", zap.Any("panicValue", r), zap.Stack("stack"))
		}
	}()

	p.fileLogger.Debug("Processing log batch...")

	oracleRetryDelayDuration := time.Duration(p.cfg.LogProcessorOracleRetryDelaySeconds) * time.Second

	// 1. Get logs from SQLite
	logs, err := p.logRepo.GetSQLiteLogs(ctx, p.cfg.LogProcessorBatchSize)
	if err != nil {
		// Check if the context was cancelled or timed out *first*
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.fileLogger.Info("Context cancelled/timed out during SQLite fetch.", zap.Error(err))
		} else {
			// Log other SQLite errors
			p.fileLogger.Error("Failed to get logs from SQLite", zap.Error(err))
		}
		return // Return on any error fetching from SQLite in this batch
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
		// Check context and stop channel before each attempt
		select {
		case <-ctx.Done():
			insertErr = ctx.Err()
			p.fileLogger.Info("Context cancelled before Oracle insert attempt.", zap.Int("attempt", attempt), zap.Error(insertErr))
			goto StopProcessing // Use goto to break outer loop cleanly
		case <-p.stopChan:
			insertErr = errors.New("processor stopped during oracle insert attempt")
			p.fileLogger.Info(insertErr.Error(), zap.Int("attempt", attempt))
			goto StopProcessing
		default:
			// Continue attempt
		}

		// Attempt insert
		insertCtxTimeout := oracleRetryDelayDuration + 15*time.Second // Timeout for the insert itself
		insertCtx, cancelInsert := context.WithTimeout(ctx, insertCtxTimeout)
		insertErr = p.logRepo.InsertBatchOracle(insertCtx, logs)
		cancelInsert() // Cancel insert context immediately after call returns

		if insertErr == nil {
			p.fileLogger.Debug("Successfully inserted log batch into Oracle", zap.Int("count", len(logs)), zap.Int("attempt", attempt))
			success = true
			break // Exit retry loop on success
		}

		// --- Handle Insert Error ---
		isConnError := errors.Is(insertErr, repositories.ErrOracleConnection)

		// Check context and stop channel again after failed attempt
		select {
		case <-ctx.Done():
			insertErr = ctx.Err() // Prioritize context error
			p.fileLogger.Info("Context cancelled after failed Oracle insert attempt.", zap.Int("attempt", attempt), zap.Error(insertErr))
			goto StopProcessing
		case <-p.stopChan:
			insertErr = errors.New("processor stopped after failed oracle insert attempt")
			p.fileLogger.Info(insertErr.Error(), zap.Int("attempt", attempt))
			goto StopProcessing
		default:
			// Continue error handling
		}

		if isConnError && attempt < p.cfg.LogProcessorOracleRetryAttempts {
			p.fileLogger.Warn("Oracle insert failed (connection issue), attempting recovery and retry.",
				zap.Error(insertErr),
				zap.Int("attempt_failed", attempt),
				zap.Int("next_attempt", attempt+1),
				zap.Int("max_attempts", p.cfg.LogProcessorOracleRetryAttempts),
			)

			// Try to re-init Oracle connection
			p.fileLogger.Info("Processor attempting to initialize/re-initialize Oracle connection for LogRepository...")
			newDb, initErr := database.InitOracle(p.cfg, p.fileLogger)
			if initErr == nil && newDb != nil {
				pingCtx, cancelPing := context.WithTimeout(context.Background(), 10*time.Second)
				pingErr := newDb.PingContext(pingCtx)
				cancelPing()
				if pingErr == nil {
					p.fileLogger.Info("Successfully established/verified Oracle connection via processor's attempt.", zap.Int("attempt_failed", attempt))
					p.logRepo.SetOracleDB(newDb)
					p.fileLogger.Warn("Processor updated shared Oracle DB handle in LogRepository.")
				} else {
					p.fileLogger.Error("Processor initialized Oracle handle, but immediate ping failed.", zap.Error(pingErr), zap.Int("attempt_failed", attempt))
					newDb.Close()
				}
			} else {
				if newDb != nil {
					newDb.Close()
				}
				p.fileLogger.Error("Processor failed to re-initialize Oracle connection.", zap.Error(initErr), zap.Int("attempt_failed", attempt))
			}

			// Wait before next attempt, respecting context/stop signal
			p.fileLogger.Info("Waiting before next Oracle insert attempt...", zap.Duration("retry_delay", oracleRetryDelayDuration))
			select {
			case <-time.After(oracleRetryDelayDuration):
				// continue to next attempt
			case <-ctx.Done():
				p.fileLogger.Info("Parent context cancelled during Oracle retry wait.", zap.Error(ctx.Err()))
				insertErr = ctx.Err()
				goto StopProcessing
			case <-p.stopChan:
				p.fileLogger.Info("Stop signal received during Oracle retry wait.")
				insertErr = errors.New("processor stopped during retry wait")
				goto StopProcessing
			}
		} else { // Non-connection error or max retries reached
			if isConnError {
				p.fileLogger.Error("Oracle insert failed after max retries for connection issue.",
					zap.Error(insertErr),
					zap.Int("attempts_made", attempt),
					zap.Int("max_attempts", p.cfg.LogProcessorOracleRetryAttempts),
				)
			} else {
				p.fileLogger.Error("Oracle insert failed (non-connection/non-retryable or final attempt).",
					zap.Error(insertErr),
					zap.Int("attempt_failed", attempt),
				)
			}
			break // Exit retry loop
		}
	}
StopProcessing: // Label for goto jumps

	if !success {
		p.fileLogger.Warn("Failed to insert log batch into Oracle after all attempts or due to stop/cancellation.", zap.Error(insertErr), zap.Int("log_count", len(logs)))
		return // Do NOT delete from SQLite
	}

	// 4. Delete logs from SQLite (only if Oracle insert succeeded)
	logIDs := make([]int64, len(logs))
	for i, log := range logs {
		logIDs[i] = log.ID
	}

	// Check context/stop signal before deleting
	select {
	case <-ctx.Done():
		p.fileLogger.Warn("Skipping SQLite delete because parent context is done.", zap.Error(ctx.Err()), zap.Int("count", len(logIDs)))
		return
	case <-p.stopChan:
		p.fileLogger.Warn("Skipping SQLite delete because processor received stop signal.", zap.Int("count", len(logIDs)))
		return
	default:
		// Continue deletion
	}

	deleteCtx, cancelDelete := context.WithTimeout(context.Background(), 10*time.Second) // Use a fresh, short timeout for delete
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
