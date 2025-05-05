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

// LogProcessor handles the transfer of logs from SQLite to Oracle
type LogProcessor struct {
	cfg       *config.Config
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
	p.ticker = time.NewTicker(p.cfg.LogBatchInterval)
	p.isRunning = true
	go p.run()
	p.logger.Info("SQLite to Oracle log processor started",
		zap.Duration("interval", p.cfg.LogBatchInterval),
		zap.Int("batchSize", p.cfg.LogProcessorBatchSize),
		zap.Int("retryAttempts", p.cfg.LogProcessorOracleRetryAttempts),
		zap.Int("retryDelaySec", p.cfg.LogProcessorOracleRetryDelaySeconds),
	)
}

// Stop signals the log processing loop to terminate gracefully
func (p *LogProcessor) Stop() {
	if !p.isRunning {
		p.logger.Warn("Log processor not running")
		return
	}
	p.logger.Info("Stopping SQLite to Oracle log processor...")
	select {
	case <-p.stopChan:
	default:
		close(p.stopChan)
	}
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.isRunning = false
	time.Sleep(500 * time.Millisecond)
	p.logger.Info("Processing final log batch before shutdown...")
	oracleRetryDelayDuration := time.Duration(p.cfg.LogProcessorOracleRetryDelaySeconds) * time.Second
	shutdownCtxTimeout := oracleRetryDelayDuration + (5 * time.Second)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownCtxTimeout)
	defer cancel()
	p.processBatch(shutdownCtx)
	p.logger.Info("Log processor stopped.")
}

// run is the main loop that periodically processes log batches
func (p *LogProcessor) run() {
	for {
		select {
		case <-p.ticker.C:
			select {
			case <-p.stopChan:
				p.logger.Info("Stop signal received before processing tick, exiting loop.")
				return
			default:
				tickCtxTimeout := p.cfg.LogBatchInterval - (1 * time.Second)
				if tickCtxTimeout <= 0 {
					tickCtxTimeout = 30 * time.Second
				}
				tickCtx, cancel := context.WithTimeout(context.Background(), tickCtxTimeout)
				p.processBatch(tickCtx)
				cancel()
			}
		case <-p.stopChan:
			p.logger.Info("Received stop signal, exiting log processing loop.")
			return
		}
	}
}

// processBatch fetches logs from SQLite, attempts to insert them into Oracle.
func (p *LogProcessor) processBatch(ctx context.Context) {
	p.logger.Debug("Processing log batch...")

	oracleRetryDelayDuration := time.Duration(p.cfg.LogProcessorOracleRetryDelaySeconds) * time.Second

	// 1. Get logs from SQLite
	logs, err := p.logRepo.GetSQLiteLogs(ctx, p.cfg.LogProcessorBatchSize)
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
	// *** CORRECTED LOOP: Start at 1, use <= for condition ***
	for attempt := 1; attempt <= p.cfg.LogProcessorOracleRetryAttempts; attempt++ {
		// Check context before attempt
		if ctx.Err() != nil {
			insertErr = ctx.Err()
			break
		}
		select {
		case <-p.stopChan:
			insertErr = errors.New("processor stopped")
			goto StopProcessing
		default:
		}

		// Attempt insert
		insertCtx, cancelInsert := context.WithTimeout(ctx, 45*time.Second)
		insertErr = p.logRepo.InsertBatchOracle(insertCtx, logs)
		cancelInsert()

		if insertErr == nil {
			p.logger.Debug("Successfully inserted log batch into Oracle", zap.Int("count", len(logs)), zap.Int("attempt", attempt))
			success = true
			break // Exit retry loop on success
		}

		// --- Handle Insert Error ---
		isConnError := errors.Is(insertErr, repositories.ErrOracleConnection)

		// Check context/stop signal again after failed attempt
		if ctx.Err() != nil {
			insertErr = ctx.Err()
			break
		}
		select {
		case <-p.stopChan:
			insertErr = errors.New("processor stopped")
			goto StopProcessing
		default:
		}

		// --- Decide whether to retry or give up ---
		// *** CORRECTED CONDITION: Use '<' to check if MORE retries are possible ***
		if isConnError && attempt < p.cfg.LogProcessorOracleRetryAttempts {
			// Log message BEFORE attempting the next retry
			p.logger.Warn("Oracle insert failed (connection issue), will attempt recovery and retry.",
				zap.Error(insertErr),
				zap.Int("attempt_failed", attempt), // Log the attempt number that just failed
				zap.Int("next_attempt", attempt+1), // Log the upcoming attempt number
				zap.Int("max_attempts", p.cfg.LogProcessorOracleRetryAttempts),
			)

			// Attempt to Init/Re-init Oracle Connection
			p.logger.Info("Processor attempting to initialize/re-initialize Oracle connection...")
			newDb, initErr := database.InitOracle(p.cfg, p.logger)
			if initErr == nil && newDb != nil {
				pingCtx, cancelPing := context.WithTimeout(context.Background(), 10*time.Second)
				pingErr := newDb.PingContext(pingCtx)
				cancelPing()
				if pingErr == nil {
					p.logger.Info("Successfully established/verified Oracle connection via processor.", zap.Int("attempt_failed", attempt))
					p.logRepo.SetOracleDB(newDb)
					p.logger.Warn("Processor updated shared Oracle DB handle.")
				} else {
					p.logger.Error("Processor initialized Oracle handle, but immediate ping failed.", zap.Error(pingErr), zap.Int("attempt_failed", attempt))
					newDb.Close()
				}
			} else {
				if newDb != nil {
					newDb.Close()
				}
			}

			// Wait before next attempt
			p.logger.Info("Waiting before next Oracle insert attempt...", zap.Duration("retry_delay", oracleRetryDelayDuration))
			select {
			case <-time.After(oracleRetryDelayDuration):
				continue // Go to the next iteration (attempt will increment)
			case <-ctx.Done():
				p.logger.Info("Parent context cancelled during Oracle retry wait.", zap.Error(ctx.Err()))
				insertErr = ctx.Err()
				goto StopProcessing
			case <-p.stopChan:
				p.logger.Info("Stop signal received during Oracle retry wait.")
				insertErr = errors.New("processor stopped")
				goto StopProcessing
			}
		} else {             // Error is not connection error OR this was the final attempt (attempt == max_attempts)
			if isConnError { // Log specific message for max retries reached on connection error
				p.logger.Error("Oracle insert failed after max retries for connection issue",
					zap.Error(insertErr),
					zap.Int("attempts_made", attempt), // Use 'attempt' which equals max_attempts here
					zap.Int("max_attempts", p.cfg.LogProcessorOracleRetryAttempts),
				)
			} else { // Log message for non-connection errors
				p.logger.Error("Oracle insert failed (non-connection/non-retryable error?)",
					zap.Error(insertErr),
					zap.Int("attempt_failed", attempt), // Log the attempt number that failed
				)
			}
			break // Exit retry loop, no more retries
		}
	}
StopProcessing: // Label for goto jumps when context/stop occurs during wait

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
	err = p.logRepo.DeleteSQLiteLogsByID(ctx, logIDs)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.logger.Warn("Context cancelled/timed out during SQLite delete.", zap.Error(err), zap.Int("count", len(logIDs)))
		} else {
			p.logger.Error("CRITICAL: Failed to delete logs from SQLite after successful Oracle insert.", zap.Error(err), zap.Int64s("log_ids", logIDs))
		}
		return
	}

	p.logger.Info("Processed and transferred log batch", zap.Int("count", len(logs)))
}
