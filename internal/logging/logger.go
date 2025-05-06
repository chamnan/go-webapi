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

// InitFileConsoleLogger initializes a Zap logger with only File and Console outputs.
// MODIFIED: Accepts the shared fileSyncer
func InitFileConsoleLogger(cfg *config.Config, fileSyncer zapcore.WriteSyncer) (*zap.Logger, error) {

	// --- Determine Log Level from Config ---
	var logLevel zapcore.Level
	// Use Zap's built-in unmarshaler for robustness
	if err := logLevel.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		// Log warning to stderr as logger isn't fully built yet
		fmt.Fprintf(os.Stderr, "[WARN] Invalid LOG_LEVEL '%s' for base logger, defaulting to info: %v\n", cfg.LogLevel, err)
		logLevel = zapcore.InfoLevel // Default to Info level on error
	}
	// --- End Log Level Determination ---

	// Use exported helpers for encoders
	consoleEncoderCfg, fileEncoderCfg := CreateFileConsoleEncoderConfigs()

	// Create Console Syncer ONLY
	consoleSyncer := zapcore.Lock(os.Stdout)

	// Create Cores using the determined logLevel and the PASSED-IN fileSyncer
	consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(consoleEncoderCfg), consoleSyncer, logLevel)
	fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(fileEncoderCfg), fileSyncer, logLevel) // Use passed-in fileSyncer
	core := zapcore.NewTee(consoleCore, fileCore)

	// Build the logger instance
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))

	// Log initialization message using the newly created logger
	logger.Info("====================================")
	logger.Info("Base file/console logger initialized",
		zap.String("environment", cfg.AppEnv),
		zap.String("configuredLevel", cfg.LogLevel),     // Log the string level from config
		zap.String("effectiveLevel", logLevel.String()), // Log the actual level being used
		zap.String("logFile", cfg.LogFilePath),
		zap.Int("logMaxSizeMB", cfg.LogMaxSize), // Log MaxSize
		// zap.String("logRotationInterval", strconv.Itoa(cfg.LogRotateInterval)), // LogRotateInterval removed, wasn't in final config
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
			logEntry.Fields = fmt.Sprintf(`{"marshal_error": "%v", "original_message": "%s"}`, err, ent.Message)
		}
	}

	// Insert into SQLite database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.repo.InsertSQLiteLog(ctx, logEntry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "CRITICAL: Failed to insert log entry into SQLite: %v\n", err)
	}

	return nil // Generally return nil from Write
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

// --- Global Logger Access ---

// SetGlobalLogger sets the global logger instance (use during app init).
func SetGlobalLogger(l *zap.Logger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = l
}

// GetLogger returns the initialized global logger.
func GetLogger() *zap.Logger {
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()

	if l == nil {
		fallbackLogger, _ := zap.NewProduction() // Use Production for fallback safety
		fallbackLogger.Warn("Global logger accessed before being set!")
		return fallbackLogger
	}
	return l
}
