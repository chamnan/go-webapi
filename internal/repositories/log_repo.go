package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync" // Import sync for mutex
	"time"

	"go-webapi/internal/models"

	"go.uber.org/zap"
	// Import driver-specific error details if needed
	// "github.com/godror/godror"
)

// ErrOracleConnection is returned when an operation fails due to Oracle connection issues.
var ErrOracleConnection = errors.New("oracle connection error")
var ErrSQLiteConnection = errors.New("sqlite connection error") // Added for consistency

// LogRepository defines the interface for log data operations
type LogRepository interface {
	// SQLite Operations
	InsertSQLiteLog(ctx context.Context, entry models.LogEntry) error
	GetSQLiteLogs(ctx context.Context, limit int) ([]models.LogEntry, error)
	DeleteSQLiteLogsByID(ctx context.Context, ids []int64) error
	// Oracle Operations
	InsertBatchOracle(ctx context.Context, logs []models.LogEntry) error

	// Mutators for DB handles and logger, to be set after initial creation
	SetOracleDB(db *sql.DB)
	SetSqliteDB(db *sql.DB)       // New
	SetLogger(logger *zap.Logger) // New
}

// logRepositoryImpl implements LogRepository for both SQLite and Oracle
type logRepositoryImpl struct {
	sqliteDB *sql.DB
	oracleDB *sql.DB // Can be nil initially or if connection fails
	logger   *zap.Logger
	mu       sync.RWMutex // Mutex to protect concurrent access to DB handles and logger
}

// NewLogRepository creates a new LogRepository
func NewLogRepository(sqliteDB *sql.DB, oracleDB *sql.DB, logger *zap.Logger) LogRepository {
	if logger == nil {
		fallbackLogger, _ := zap.NewDevelopment() // Use a simple fallback
		logger = fallbackLogger.Named("logRepo_fallback")
		logger.Warn("NewLogRepository received nil logger, using fallback.")
	}
	return &logRepositoryImpl{
		sqliteDB: sqliteDB, // Initial handle (can be nil)
		oracleDB: oracleDB, // Initial handle (can be nil)
		logger:   logger,
	}
}

// --- SQLite Methods ---
func (r *logRepositoryImpl) InsertSQLiteLog(ctx context.Context, entry models.LogEntry) error {
	r.mu.RLock()
	currentSqliteDB := r.sqliteDB
	currentLogger := r.logger
	r.mu.RUnlock()

	if currentSqliteDB == nil {
		// This case should ideally be rare and only during startup if called before SetSqliteDB
		errMsg := "CRITICAL: Attempted to insert log into SQLite via logRepo, but sqliteDB is nil."
		if currentLogger != nil {
			currentLogger.Error(errMsg, zap.Any("log_entry", entry))
		} else {
			fmt.Printf("%s Log: %+v\n", errMsg, entry) // Fallback to stdout if logger also nil
		}
		return fmt.Errorf("sqliteDB is not initialized in LogRepository: %w", ErrSQLiteConnection)
	}

	query := `INSERT INTO tbl_log (timestamp, level, message, fields) VALUES (?, ?, ?, ?)`
	fieldsJSON := entry.Fields
	if fieldsJSON == "" {
		fieldsJSON = "{}"
	}
	_, err := currentSqliteDB.ExecContext(ctx, query, entry.Timestamp, entry.Level, entry.Message, fieldsJSON)
	if err != nil {
		if currentLogger != nil {
			currentLogger.Error("Failed to insert log into SQLite", zap.Error(err))
		}
		return fmt.Errorf("sqlite insert failed: %w", err)
	}
	return nil
}

func (r *logRepositoryImpl) GetSQLiteLogs(ctx context.Context, limit int) ([]models.LogEntry, error) {
	r.mu.RLock()
	currentSqliteDB := r.sqliteDB
	currentLogger := r.logger
	r.mu.RUnlock()

	if currentSqliteDB == nil {
		if currentLogger != nil {
			currentLogger.Error("Attempted to get logs from SQLite, but sqliteDB is nil.")
		}
		return nil, fmt.Errorf("sqliteDB is not initialized in LogRepository: %w", ErrSQLiteConnection)
	}

	query := `SELECT id, timestamp, level, message, fields FROM tbl_log ORDER BY id ASC LIMIT ?`
	rows, err := currentSqliteDB.QueryContext(ctx, query, limit)
	if err != nil {
		if currentLogger != nil {
			currentLogger.Error("Failed to query logs from SQLite", zap.Error(err))
		}
		return nil, fmt.Errorf("sqlite query failed: %w", err)
	}
	defer rows.Close()
	var logs []models.LogEntry
	for rows.Next() {
		var entry models.LogEntry
		var tsStr string
		var fields sql.NullString
		if errScan := rows.Scan(&entry.ID, &tsStr, &entry.Level, &entry.Message, &fields); errScan != nil {
			if currentLogger != nil {
				currentLogger.Error("Failed to scan log row from SQLite", zap.Error(errScan))
			}
			return nil, fmt.Errorf("sqlite row scan failed: %w", errScan)
		}
		// Attempt to parse timestamp in RFC3339Nano format (common for Zap)
		// If parsing fails, try other common formats or default to current time.
		parsedTime, parseErr := time.Parse(time.RFC3339Nano, tsStr)
		if parseErr != nil {
			// Fallback to SQLite's typical DATETIME format if RFC3339Nano fails
			parsedTime, parseErr = time.Parse("2006-01-02 15:04:05.999999999-07:00", tsStr) // common full format
			if parseErr != nil {
				parsedTime, parseErr = time.Parse("2006-01-02 15:04:05", tsStr) // common short format
			}
		}

		if parseErr != nil {
			if currentLogger != nil {
				currentLogger.Warn("Failed to parse timestamp from SQLite, using current UTC time.",
					zap.String("raw_timestamp", tsStr),
					zap.Error(parseErr))
			}
			entry.Timestamp = time.Now().UTC() // Fallback to current time
		} else {
			entry.Timestamp = parsedTime.UTC() // Normalize to UTC for consistency
		}

		if fields.Valid {
			entry.Fields = fields.String
		} else {
			entry.Fields = "{}"
		}
		logs = append(logs, entry)
	}
	if errRows := rows.Err(); errRows != nil {
		if currentLogger != nil {
			currentLogger.Error("Error during iteration over SQLite log rows", zap.Error(errRows))
		}
		return nil, fmt.Errorf("sqlite row iteration error: %w", errRows)
	}
	return logs, nil
}
func (r *logRepositoryImpl) DeleteSQLiteLogsByID(ctx context.Context, ids []int64) error {
	r.mu.RLock()
	currentSqliteDB := r.sqliteDB
	currentLogger := r.logger
	r.mu.RUnlock()

	if currentSqliteDB == nil {
		if currentLogger != nil {
			currentLogger.Error("Attempted to delete logs from SQLite, but sqliteDB is nil.")
		}
		return fmt.Errorf("sqliteDB is not initialized in LogRepository: %w", ErrSQLiteConnection)
	}
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`DELETE FROM tbl_log WHERE id IN (%s)`, strings.Join(placeholders, ","))
	result, err := currentSqliteDB.ExecContext(ctx, query, args...)
	if err != nil {
		if currentLogger != nil {
			currentLogger.Error("Failed to delete logs from SQLite", zap.Error(err))
		}
		return fmt.Errorf("sqlite delete failed: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if currentLogger != nil {
		currentLogger.Debug("Deleted logs from SQLite", zap.Int64("rows_affected", rowsAffected), zap.Int("id_count", len(ids)))
	}
	return nil
}

// --- End SQLite Methods ---

// --- Oracle Operations ---

// InsertBatchOracle inserts a slice of LogEntry into Oracle tbl_log using a batch operation
func (r *logRepositoryImpl) InsertBatchOracle(ctx context.Context, logs []models.LogEntry) error {
	r.mu.RLock()
	currentOracleDB := r.oracleDB
	currentLogger := r.logger
	r.mu.RUnlock()

	if len(logs) == 0 {
		return nil
	}

	if currentOracleDB == nil {
		if currentLogger != nil {
			currentLogger.Warn("Skipping Oracle insert: Oracle DB handle is currently nil in repository")
		}
		return fmt.Errorf("repository oracle DB handle is nil: %w", ErrOracleConnection)
	}

	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	err := currentOracleDB.PingContext(pingCtx)
	cancelPing()
	if err != nil {
		if currentLogger != nil {
			currentLogger.Warn("Oracle ping failed before batch insert", zap.Error(err))
		}
		return fmt.Errorf("oracle ping failed: %w: %w", err, ErrOracleConnection)
	}

	tx, err := currentOracleDB.BeginTx(ctx, nil)
	if err != nil {
		if currentLogger != nil {
			currentLogger.Error("Failed to begin Oracle transaction", zap.Error(err))
		}
		if isConnectionError(err) { // isConnectionError needs to be defined or imported
			return fmt.Errorf("oracle begin tx failed: %w: %w", err, ErrOracleConnection)
		}
		return fmt.Errorf("oracle begin tx failed: %w", err)
	}
	defer tx.Rollback() // Ensure rollback on error

	// Ensure your Oracle tbl_log has columns like: LOG_TIMESTAMP, LOG_LEVEL, LOG_MESSAGE, LOG_DETAILS
	// LOG_TIMESTAMP should be TIMESTAMP, LOG_LEVEL VARCHAR2, LOG_MESSAGE CLOB/VARCHAR2, LOG_DETAILS CLOB (for JSON string)
	query := `INSERT INTO tbl_log (log_timestamp, log_level, log_message, log_details) VALUES (:1, :2, :3, :4)`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		if currentLogger != nil {
			currentLogger.Error("Failed to prepare Oracle batch insert statement", zap.Error(err))
		}
		if isConnectionError(err) {
			return fmt.Errorf("oracle prepare failed: %w: %w", err, ErrOracleConnection)
		}
		return fmt.Errorf("oracle prepare statement failed: %w", err)
	}
	defer stmt.Close()

	var firstExecError error
	var firstExecErrorIsConn bool
	for _, entry := range logs {
		fieldsData := entry.Fields
		if fieldsData == "" {
			fieldsData = "{}" // Ensure valid JSON, even if empty
		}
		// Oracle might require specific timestamp format or conversion if not directly compatible.
		// entry.Timestamp is time.Time, godror should handle it.
		_, execErr := stmt.ExecContext(ctx, entry.Timestamp, entry.Level, entry.Message, fieldsData)
		if execErr != nil {
			if firstExecError == nil { // Store only the first error for reporting
				firstExecError = execErr
				firstExecErrorIsConn = isConnectionError(execErr)
				if currentLogger != nil {
					currentLogger.Error("Error during Oracle batch insert execution (first error in batch)",
						zap.Error(execErr),
						zap.Int64("sqlite_log_id", entry.ID), // Assuming LogEntry has an ID from SQLite
					)
				}
			}
			// Depending on requirements, you might break or continue.
			// Breaking on first error is often safer for batch operations.
			break
		}
	}

	if firstExecError != nil {
		// Rollback is handled by defer.
		baseErr := fmt.Errorf("oracle batch exec failed (first error: %w)", firstExecError)
		if firstExecErrorIsConn {
			return fmt.Errorf("%w: %w", baseErr, ErrOracleConnection)
		}
		return baseErr
	}

	if errCommit := tx.Commit(); errCommit != nil {
		if currentLogger != nil {
			currentLogger.Error("Failed to commit Oracle transaction", zap.Error(errCommit))
		}
		if isConnectionError(errCommit) {
			return fmt.Errorf("oracle commit failed: %w: %w", errCommit, ErrOracleConnection)
		}
		return fmt.Errorf("oracle commit failed: %w", errCommit)
	}

	if currentLogger != nil {
		currentLogger.Debug("Successfully inserted log batch into Oracle", zap.Int("batch_size", len(logs)))
	}
	return nil
}

// SetOracleDB allows updating the Oracle DB handle after initialization.
func (r *logRepositoryImpl) SetOracleDB(db *sql.DB) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Optionally close the old r.oracleDB if it's being replaced and managed by logRepo exclusively
	// if r.oracleDB != nil { r.oracleDB.Close() }
	r.oracleDB = db
	if r.logger != nil {
		status := "nil"
		if db != nil {
			status = "set/updated"
		}
		r.logger.Info("LogRepository Oracle DB handle updated via SetOracleDB.", zap.String("status", status))
	}
}

// SetSqliteDB allows updating the SQLite DB handle.
func (r *logRepositoryImpl) SetSqliteDB(db *sql.DB) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Optionally close the old r.sqliteDB if it's being replaced
	// if r.sqliteDB != nil { r.sqliteDB.Close() }
	r.sqliteDB = db
	if r.logger != nil {
		status := "nil"
		if db != nil {
			status = "set/updated"
		}
		r.logger.Info("LogRepository SQLite DB handle updated via SetSqliteDB.", zap.String("status", status))
	}
}

// SetLogger allows updating the logger instance used by the repository.
func (r *logRepositoryImpl) SetLogger(logger *zap.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger = logger
	if r.logger != nil { // Log this with the new logger if possible
		r.logger.Info("LogRepository internal logger has been updated.")
	} else { // Fallback if new logger is nil
		fmt.Println("[INFO] LogRepository internal logger has been set (possibly to nil).")
	}
}

// isConnectionError helper function (copied from your existing code or simplified)
// You should have a robust way to identify connection errors.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Basic checks, expand as needed based on godror errors
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	// Attempt to check for Oracle specific error codes if using a driver like godror
	// godror.OraErr has a Code field.
	type oracleError interface {
		Code() int
	}
	if oraErr, ok := err.(oracleError); ok {
		// Common Oracle connection-related error codes
		// List might need adjustment based on specific Oracle versions and configurations
		switch oraErr.Code() {
		case 3113, 3114, 12170, 12537, 12541, 12545, 12560: // ORA-03113, ORA-03114, ORA-12170, etc.
			return true
		}
	}

	// Fallback to string matching for broader cases or other drivers
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "ora-03113") ||
		strings.Contains(errStr, "ora-03114") ||
		strings.Contains(errStr, "ora-12537") ||
		strings.Contains(errStr, "ora-12170") ||
		strings.Contains(errStr, "ora-12541") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "network error") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "i/o error") ||
		strings.Contains(errStr, "timeout") {
		return true
	}
	return false
}
