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

// LogRepository defines the interface for log data operations
type LogRepository interface {
	// SQLite Operations
	InsertSQLiteLog(ctx context.Context, entry models.LogEntry) error
	GetSQLiteLogs(ctx context.Context, limit int) ([]models.LogEntry, error)
	DeleteSQLiteLogsByID(ctx context.Context, ids []int64) error
	// Oracle Operations
	InsertBatchOracle(ctx context.Context, logs []models.LogEntry) error

	// SetOracleDB needs to be part of the interface for the processor to call it
	SetOracleDB(db *sql.DB)
}

// logRepositoryImpl implements LogRepository for both SQLite and Oracle
type logRepositoryImpl struct {
	sqliteDB *sql.DB
	oracleDB *sql.DB // Can be nil initially or if connection fails
	logger   *zap.Logger
	mu       sync.RWMutex // Mutex to protect concurrent access to oracleDB
}

// NewLogRepository creates a new LogRepository
func NewLogRepository(sqliteDB *sql.DB, oracleDB *sql.DB, logger *zap.Logger) LogRepository {
	if logger == nil {
		fallbackLogger, _ := zap.NewDevelopment()
		logger = fallbackLogger
		logger.Warn("NewLogRepository received nil logger, using fallback.")
	}
	return &logRepositoryImpl{
		sqliteDB: sqliteDB,
		oracleDB: oracleDB, // Initial handle (can be nil)
		logger:   logger,
	}
}

// --- SQLite Methods ---
func (r *logRepositoryImpl) InsertSQLiteLog(ctx context.Context, entry models.LogEntry) error {
	query := `INSERT INTO tbl_log (timestamp, level, message, fields) VALUES (?, ?, ?, ?)`
	fieldsJSON := entry.Fields
	if fieldsJSON == "" {
		fieldsJSON = "{}"
	}
	_, err := r.sqliteDB.ExecContext(ctx, query, entry.Timestamp, entry.Level, entry.Message, fieldsJSON)
	if err != nil {
		r.logger.Error("Failed to insert log into SQLite", zap.Error(err))
		return fmt.Errorf("sqlite insert failed: %w", err)
	}
	return nil
}
func (r *logRepositoryImpl) GetSQLiteLogs(ctx context.Context, limit int) ([]models.LogEntry, error) {
	query := `SELECT id, timestamp, level, message, fields FROM tbl_log ORDER BY id ASC LIMIT ?`
	rows, err := r.sqliteDB.QueryContext(ctx, query, limit)
	if err != nil {
		r.logger.Error("Failed to query logs from SQLite", zap.Error(err))
		return nil, fmt.Errorf("sqlite query failed: %w", err)
	}
	defer rows.Close()
	var logs []models.LogEntry
	for rows.Next() {
		var entry models.LogEntry
		var tsStr string
		var fields sql.NullString
		if err := rows.Scan(&entry.ID, &tsStr, &entry.Level, &entry.Message, &fields); err != nil {
			r.logger.Error("Failed to scan log row from SQLite", zap.Error(err))
			continue
		}
		entry.Timestamp, err = time.Parse(time.RFC3339Nano, tsStr)
		if err != nil {
			r.logger.Warn("Failed to parse timestamp from SQLite", zap.String("raw_ts", tsStr), zap.Error(err))
			entry.Timestamp = time.Now().UTC()
		}
		if fields.Valid {
			entry.Fields = fields.String
		} else {
			entry.Fields = "{}"
		}
		logs = append(logs, entry)
	}
	if err = rows.Err(); err != nil {
		r.logger.Error("Error during iteration over SQLite log rows", zap.Error(err))
		return nil, fmt.Errorf("sqlite row iteration error: %w", err)
	}
	return logs, nil
}
func (r *logRepositoryImpl) DeleteSQLiteLogsByID(ctx context.Context, ids []int64) error {
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
	result, err := r.sqliteDB.ExecContext(ctx, query, args...)
	if err != nil {
		r.logger.Error("Failed to delete logs from SQLite", zap.Error(err))
		return fmt.Errorf("sqlite delete failed: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	r.logger.Debug("Deleted logs from SQLite", zap.Int64("rows_affected", rowsAffected), zap.Int("id_count", len(ids)))
	return nil
}

// --- End SQLite Methods ---

// --- Oracle Operations ---

// InsertBatchOracle inserts a slice of LogEntry into Oracle tbl_log using a batch operation
func (r *logRepositoryImpl) InsertBatchOracle(ctx context.Context, logs []models.LogEntry) error {
	if len(logs) == 0 {
		return nil
	}

	// Use RLock for reading the handle safely
	r.mu.RLock()
	currentOracleDB := r.oracleDB
	r.mu.RUnlock()

	if currentOracleDB == nil {
		r.logger.Warn("Skipping Oracle insert: Oracle DB handle is currently nil in repository")
		return fmt.Errorf("repository oracle DB handle is nil: %w", ErrOracleConnection)
	}

	// Ping before transaction
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	err := currentOracleDB.PingContext(pingCtx) // Use the read handle
	cancelPing()
	if err != nil {
		r.logger.Warn("Oracle ping failed before batch insert", zap.Error(err))
		return fmt.Errorf("oracle ping failed: %w", ErrOracleConnection)
	}

	tx, err := currentOracleDB.BeginTx(ctx, nil) // Use the read handle
	if err != nil {
		r.logger.Error("Failed to begin Oracle transaction", zap.Error(err))
		if isConnectionError(err) {
			return fmt.Errorf("oracle begin tx failed: %w: %w", err, ErrOracleConnection)
		}
		return fmt.Errorf("oracle begin tx failed: %w", err)
	}
	defer tx.Rollback()

	query := `INSERT INTO tbl_log (log_timestamp, log_level, log_message, log_details) VALUES (:1, :2, :3, :4)`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		r.logger.Error("Failed to prepare Oracle batch insert statement", zap.Error(err))
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
			fieldsData = "{}"
		}
		_, err := stmt.ExecContext(ctx, entry.Timestamp, entry.Level, entry.Message, fieldsData)
		if err != nil {
			if firstExecError == nil {
				firstExecError = err
				firstExecErrorIsConn = isConnectionError(err)
				r.logger.Error("First error during Oracle batch insert execution", zap.Error(err), zap.Int64("sqlite_id", entry.ID))
			}
			break // Stop batch on first error
		}
	}

	if firstExecError != nil {
		baseErr := fmt.Errorf("oracle batch exec failed (first error: %w)", firstExecError)
		if firstExecErrorIsConn {
			return fmt.Errorf("%w: %w", baseErr, ErrOracleConnection)
		}
		return baseErr
	}

	if err := tx.Commit(); err != nil {
		r.logger.Error("Failed to commit Oracle transaction", zap.Error(err))
		if isConnectionError(err) {
			return fmt.Errorf("oracle commit failed: %w: %w", err, ErrOracleConnection)
		}
		return fmt.Errorf("oracle commit failed: %w", err)
	}

	r.logger.Debug("Successfully inserted log batch into Oracle", zap.Int("batch_size", len(logs)))
	return nil
}

// SetOracleDB allows updating the Oracle DB handle after initialization (e.g., by processor).
// It is concurrency-safe using a RWMutex.
func (r *logRepositoryImpl) SetOracleDB(db *sql.DB) {
	r.mu.Lock() // Use full lock for writing the handle
	defer r.mu.Unlock()

	// Decide if you want to close the old one. This is risky if the handle is shared
	// and another goroutine might still be using the old one.
	// For simplicity, we'll just replace it here. The old pool might get garbage collected
	// or closed eventually if the handle passed from InitOracle is closed in app.go.
	// if r.oracleDB != nil {
	//  r.logger.Warn("Replacing existing Oracle DB handle in repository.")
	//  // r.oracleDB.Close() // Consider implications carefully
	// }

	r.oracleDB = db // Update the handle
	status := "nil"
	if db != nil {
		status = "set/updated"
	}
	r.logger.Info("LogRepository Oracle DB handle updated via SetOracleDB", zap.String("status", status))
}

// isConnectionError helper function (keep as before or enhance)
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, ErrOracleConnection) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	// Add more specific error string checks based on your driver/environment
	if strings.Contains(errStr, "ora-03113") || strings.Contains(errStr, "ora-03114") || strings.Contains(errStr, "ora-125") ||
		strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "network error") || strings.Contains(errStr, "i/o error") ||
		strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "reset by peer") || strings.Contains(errStr, "timeout") {
		return true
	}
	return false
}

// REMOVED SetLogger implementation (logger set only via NewLogRepository)
// REMOVED IsOracleConnected implementation
