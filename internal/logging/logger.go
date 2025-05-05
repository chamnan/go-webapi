package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync" // Import sync for mutex
	"time"

	"go-webapi/internal/config"
	"go-webapi/internal/models"
	"go-webapi/internal/repositories"

	"github.com/DeRuina/timberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	globalLogger *zap.Logger
	globalMu     sync.RWMutex // Mutex to protect globalLogger
)

// CreateFileConsoleEncoderConfigs creates standard encoder configs for console and file.
func CreateFileConsoleEncoderConfigs() (zapcore.EncoderConfig, zapcore.EncoderConfig) {
	// Console Encoder (human-readable, colored)
	consoleEncoderCfg := zap.NewDevelopmentEncoderConfig()
	consoleEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleEncoderCfg.EncodeCaller = zapcore.ShortCallerEncoder

	// File Encoder (JSON format for machine readability)
	fileEncoderCfg := zap.NewProductionEncoderConfig()
	fileEncoderCfg.TimeKey = "timestamp"                       // Consistent key name
	fileEncoderCfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder // Use standard format
	fileEncoderCfg.EncodeCaller = zapcore.ShortCallerEncoder

	return consoleEncoderCfg, fileEncoderCfg
}

// CreateFileConsoleWriteSyncers creates standard write syncers for console and file using lumberjack.
func CreateFileConsoleWriteSyncers(cfg *config.Config) (zapcore.WriteSyncer, zapcore.WriteSyncer, error) {
	// Console Writer
	consoleWriter := zapcore.Lock(os.Stdout)

	// Rotating File Writer using lumberjack
	// Ensure the directory exists (handled in config loading, but good practice)
	if err := os.MkdirAll(filepath.Dir(cfg.LogFilePath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating log directory for lumberjack: %v\n", err)
		// Decide if this is fatal or if logging can continue to console only
		// return nil, nil, fmt.Errorf("failed to create log directory %s: %w", filepath.Dir(cfg.LogFilePath), err)
	}

	fileWriter := zapcore.AddSync(&timberjack.Logger{ // <-- Change lumberjack to timberjack here
		Filename:         cfg.LogFilePath,                                  // Path from config
		MaxSize:          cfg.LogMaxSize,                                   // Max size in MB from config
		MaxBackups:       cfg.LogMaxBackups,                                // Max backup files from config
		MaxAge:           cfg.LogMaxAge,                                    // Max days to retain from config
		Compress:         cfg.LogCompress,                                  // Compress flag from config
		LocalTime:        true,                                             // Use local time for backup filenames
		RotationInterval: time.Duration(cfg.LogRotateInterval) * time.Hour, // Rotate every x hours from config
	})

	return consoleWriter, fileWriter, nil
}

// InitFileConsoleLogger initializes a Zap logger with only File and Console outputs.
// It now respects the LogLevel set in the configuration.
func InitFileConsoleLogger(cfg *config.Config) (*zap.Logger, error) {

	// --- Determine Log Level from Config ---
	var logLevel zapcore.Level
	// Use Zap's built-in unmarshaler for robustness
	if err := logLevel.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Invalid LOG_LEVEL '%s' for base logger, defaulting to info: %v\n", cfg.LogLevel, err)
		logLevel = zapcore.InfoLevel // Default to Info level on error
	}
	// --- End Log Level Determination ---

	// Use exported helpers
	consoleEncoderCfg, fileEncoderCfg := CreateFileConsoleEncoderConfigs()
	consoleWriter, fileWriter, err := CreateFileConsoleWriteSyncers(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create file/console writers for base logger: %w", err)
	}

	// Create cores using the determined logLevel
	consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(consoleEncoderCfg), consoleWriter, logLevel)
	fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(fileEncoderCfg), fileWriter, logLevel)
	core := zapcore.NewTee(consoleCore, fileCore)

	// Build the logger instance
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))

	// Log initialization message using the newly created logger
	logger.Info("====================================") // Added separator line from your snippet
	logger.Info("Base file/console logger initialized",
		zap.String("environment", cfg.AppEnv),
		zap.String("configuredLevel", cfg.LogLevel),
		zap.String("effectiveLevel", logLevel.String()),
		zap.String("logFile", cfg.LogFilePath),
		zap.String("logMaxSize", strconv.Itoa(cfg.LogMaxSize)),
		zap.String("logRotationInterval", strconv.Itoa(cfg.LogRotateInterval)),
	)

	return logger, nil
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
	// Combine core fields (from logger.With) and contextual fields (from log call site)
	allFields := append(append([]zapcore.Field(nil), c.fields...), fields...)

	// Use MapObjectEncoder to build map for custom fields
	fieldMap := make(map[string]interface{})
	mapEncoder := zapcore.NewMapObjectEncoder()
	for _, field := range allFields {
		field.AddTo(mapEncoder)
	}
	fieldMap = mapEncoder.Fields

	logEntry := models.LogEntry{
		Timestamp: ent.Time.Local(), // Use Local time for DB consistency if desired
		Level:     ent.Level.String(),
		Message:   ent.Message,
		Fields:    "{}", // Default
	}

	// Marshal the collected custom fields map
	if len(fieldMap) > 0 {
		fieldBytes, err := json.Marshal(fieldMap)
		if err == nil {
			logEntry.Fields = string(fieldBytes)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to marshal custom fields map for SQLite: %v\n", err)
			// Encode the error itself into the fields
			logEntry.Fields = fmt.Sprintf(`{"marshal_error": "%v", "original_message": "%s"}`, err, ent.Message)
		}
	}

	// Insert into SQLite database
	// Use a short timeout for the insert operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.repo.InsertSQLiteLog(ctx, logEntry)
	if err != nil {
		// Log critical failure to insert log to stderr to avoid infinite loops
		fmt.Fprintf(os.Stderr, "CRITICAL: Failed to insert log entry into SQLite: %v\n", err)
		// Avoid returning error here to prevent Zap from trying to log the error itself?
		// Depending on Zap's internal error handling, returning error might cause issues.
		// return fmt.Errorf("failed to insert log into sqlite via repo: %w", err)
	}

	return nil // Generally return nil from Write unless it's a critical failure Zap should know about
}

func (c *sqliteCore) Sync() error {
	// Sync for SQLite is typically a no-op unless there's OS-level buffering concerns
	// which are rare for typical SQLite usage.
	return nil
}

func (c *sqliteCore) clone() *sqliteCore {
	return &sqliteCore{
		LevelEnabler: c.LevelEnabler,
		encoder:      c.encoder.Clone(),
		cfg:          c.cfg,
		repo:         c.repo,
		// Ensure deep copy of fields slice if necessary, simple append creates shallow copy
		// but Zap manages Field immutability well enough generally.
		fields: append([]zapcore.Field(nil), c.fields...),
	}
}

// --- Global Logger Access ---
// Added import "path/filepath" at the top

// SetGlobalLogger sets the global logger instance (use during app init).
func SetGlobalLogger(l *zap.Logger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = l
}

// GetLogger returns the initialized global logger.
// Provides a fallback development logger if the global one hasn't been set.
func GetLogger() *zap.Logger {
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()

	if l == nil {
		// Fallback logger logs a warning if accessed before initialization
		// Use NewProduction to avoid potential panics in Development logger if stderr is closed
		fallbackLogger, _ := zap.NewProduction()
		fallbackLogger.Warn("Global logger accessed before being set!")
		return fallbackLogger
	}
	return l
}
