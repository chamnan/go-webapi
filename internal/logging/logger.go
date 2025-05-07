package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync" // Import sync for mutex
	"time"

	"go-webapi/internal/config"
	"go-webapi/internal/models"
	"go-webapi/internal/repositories"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	globalFileLogger   *zap.Logger // Renamed from globalMainLogger
	globalSQLiteLogger *zap.Logger // Can be nil
	globalLoggersMu    sync.RWMutex
)

// AppLoggers holds the different logger instances for the application.
type AppLoggers struct {
	File   *zap.Logger // Renamed from Main: For general logging (console, file)
	SQLite *zap.Logger // For dedicated SQLite logging (can be nil if disabled)
}

// CreateFileConsoleEncoderConfigs remains the same.
func CreateFileConsoleEncoderConfigs() (zapcore.EncoderConfig, zapcore.EncoderConfig) {
	// Console Encoder (human-readable, colored)
	consoleEncoderCfg := zap.NewDevelopmentEncoderConfig()
	consoleEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleEncoderCfg.EncodeCaller = zapcore.ShortCallerEncoder

	// File Encoder (JSON format for machine readability)
	fileEncoderCfg := zap.NewProductionEncoderConfig()
	fileEncoderCfg.TimeKey = "timestamp"
	fileEncoderCfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	fileEncoderCfg.EncodeCaller = zapcore.ShortCallerEncoder

	return consoleEncoderCfg, fileEncoderCfg
}

// InitializeLoggers creates the file/console application logger
// and a dedicated SQLite logger.
func InitializeLoggers(cfg *config.Config, logRepo repositories.LogRepository, fileSyncer zapcore.WriteSyncer) (*AppLoggers, error) {
	appLoggers := &AppLoggers{}

	// --- Initialize File/Console Logger ---
	var fileLogLevel zapcore.Level // Renamed from mainLogLevel
	if err := fileLogLevel.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Invalid LOG_LEVEL '%s' for file/console logger, defaulting to info: %v\n", cfg.LogLevel, err)
		fileLogLevel = zapcore.InfoLevel
	}

	consoleEncoderCfg, fileEncoderCfg := CreateFileConsoleEncoderConfigs()
	consoleSyncer := zapcore.Lock(os.Stdout)

	consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(consoleEncoderCfg), consoleSyncer, fileLogLevel)
	fileOutputCore := zapcore.NewCore(zapcore.NewConsoleEncoder(fileEncoderCfg), fileSyncer, fileLogLevel)

	fileAndConsoleLoggerCore := zapcore.NewTee(consoleCore, fileOutputCore)
	appLoggers.File = zap.New(fileAndConsoleLoggerCore, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))

	appLoggers.File.Info("File/Console application logger initialized",
		zap.String("environment", cfg.AppEnv),
		zap.String("configuredLevel", cfg.LogLevel),
		zap.String("effectiveLevel", fileLogLevel.String()),
		zap.String("logFile", cfg.LogFilePath),
	)

	// --- Initialize Dedicated SQLite Logger ---
	if cfg.SQLLiteLogEnabled {
		var sqliteLogLevel zapcore.Level
		if err := sqliteLogLevel.UnmarshalText([]byte(cfg.SQLLiteLogLevel)); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] Invalid SQLITE_LOG_LEVEL '%s', defaulting to warn: %v\n", cfg.SQLLiteLogLevel, err)
			sqliteLogLevel = zapcore.WarnLevel
		}
		// Use the fileEncoderCfg for SQLite for consistency in JSON structure,
		// or define a separate one if needed.
		sqliteEncoderConfig := fileEncoderCfg // Re-using the file encoder config for JSON structure
		sqliteJSONEncoder := zapcore.NewJSONEncoder(sqliteEncoderConfig)
		sqliteOnlyCore := NewSQLiteCore(sqliteLogLevel, sqliteJSONEncoder, sqliteEncoderConfig, logRepo)

		appLoggers.SQLite = zap.New(sqliteOnlyCore, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
		appLoggers.File.Info("Dedicated SQLite logger initialized", // Log this info using the file/console logger
			zap.String("effectiveLevel", sqliteLogLevel.String()),
		)
	} else {
		appLoggers.File.Info("Dedicated SQLite logger is disabled by configuration.")
		appLoggers.SQLite = zap.NewNop() // Provide a no-op logger if disabled
	}

	return appLoggers, nil
}

// --- Custom SQLite Zap Core ---

// sqliteCore implements zapcore.Core and writes logs to SQLite via a LogRepository.
type sqliteCore struct {
	zapcore.LevelEnabler
	encoder zapcore.Encoder
	cfg     zapcore.EncoderConfig // Stored config for accessing keys
	repo    repositories.LogRepository
	fields  []zapcore.Field // Fields added via logger.With()
}

// NewSQLiteCore creates a new core for writing logs to SQLite.
func NewSQLiteCore(enab zapcore.LevelEnabler, enc zapcore.Encoder, cfg zapcore.EncoderConfig, repo repositories.LogRepository) zapcore.Core {
	return &sqliteCore{
		LevelEnabler: enab,
		encoder:      enc.Clone(),
		cfg:          cfg,
		repo:         repo,
		fields:       make([]zapcore.Field, 0),
	}
}

func (c *sqliteCore) Enabled(level zapcore.Level) bool {
	return c.LevelEnabler.Enabled(level)
}

func (c *sqliteCore) With(fields []zapcore.Field) zapcore.Core {
	clone := c.clone()
	clone.fields = append(clone.fields, fields...)
	return clone
}

func (c *sqliteCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

// Write uses MapObjectEncoder to correctly extract and marshal custom fields.
func (c *sqliteCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	allFields := append(append([]zapcore.Field(nil), c.fields...), fields...)

	fieldMap := make(map[string]interface{})
	mapEncoder := zapcore.NewMapObjectEncoder()
	for _, field := range allFields {
		field.AddTo(mapEncoder)
	}
	fieldMap = mapEncoder.Fields

	logEntry := models.LogEntry{
		Timestamp: ent.Time.Local(),
		Level:     ent.Level.String(),
		Message:   ent.Message,
		Fields:    "{}",
	}

	if len(fieldMap) > 0 {
		fieldBytes, err := json.Marshal(fieldMap)
		if err == nil {
			logEntry.Fields = string(fieldBytes)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to marshal custom fields map for SQLite: %v\n", err)
			logEntry.Fields = fmt.Sprintf(`{"marshal_error": "%v", "original_message": "%s"}`, err, ent.Message)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.repo.InsertSQLiteLog(ctx, logEntry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "CRITICAL: Failed to insert log entry into SQLite: %v\n", err)
	}

	return nil
}

func (c *sqliteCore) Sync() error {
	return nil
}

func (c *sqliteCore) clone() *sqliteCore {
	return &sqliteCore{
		LevelEnabler: c.LevelEnabler,
		encoder:      c.encoder.Clone(),
		cfg:          c.cfg,
		repo:         c.repo,
		fields:       append([]zapcore.Field(nil), c.fields...),
	}
}

// --- Global Logger Access (Updated) ---

// SetGlobalLoggers sets the global logger instances.
func SetGlobalLoggers(fileLogger, sqliteLogger *zap.Logger) { // Renamed mainLogger to fileLogger
	globalLoggersMu.Lock()
	defer globalLoggersMu.Unlock()
	globalFileLogger = fileLogger // Renamed globalMainLogger
	if sqliteLogger != nil {
		globalSQLiteLogger = sqliteLogger
	} else {
		globalSQLiteLogger = zap.NewNop() // Ensure it's not nil
	}
}

// GetFileLogger returns the initialized global file/console logger.
func GetFileLogger() *zap.Logger { // Renamed from GetMainLogger
	globalLoggersMu.RLock()
	l := globalFileLogger // Renamed globalMainLogger
	globalLoggersMu.RUnlock()

	if l == nil {
		fallbackLogger, _ := zap.NewProduction()
		fallbackLogger.Warn("Global file/console logger accessed before being set!")
		return fallbackLogger
	}
	return l
}

// GetSQLiteLogger returns the initialized global SQLite logger.
// Returns a Nop logger if SQLite logging was disabled or not initialized.
func GetSQLiteLogger() *zap.Logger {
	globalLoggersMu.RLock()
	l := globalSQLiteLogger
	globalLoggersMu.RUnlock()

	if l == nil {
		// This case should ideally be handled by SetGlobalLoggers ensuring it's a Nop logger.
		return zap.NewNop()
	}
	return l
}

// GetLogger can be deprecated or changed to return FileLogger for backward compatibility
// if only one logger was accessed globally previously.
// For clarity, using GetFileLogger() or GetSQLiteLogger() is preferred.
func GetLogger() *zap.Logger {
	fmt.Fprintln(os.Stderr, "[WARN] logging.GetLogger() is deprecated. Use GetFileLogger() or GetSQLiteLogger(). Returning file/console logger.")
	return GetFileLogger() // Renamed from GetMainLogger
}
