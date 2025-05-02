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
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	globalLogger *zap.Logger
	globalMu     sync.RWMutex // Mutex to protect globalLogger
)

// --- EXPORTED Encoders Helper --- // RENAMED TO START WITH UPPERCASE
// CreateFileConsoleEncoderConfigs creates standard encoder configs for console and file.
func CreateFileConsoleEncoderConfigs() (zapcore.EncoderConfig, zapcore.EncoderConfig) {
	// Console Encoder (human-readable, colored)
	consoleEncoderCfg := zap.NewDevelopmentEncoderConfig()
	consoleEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleEncoderCfg.EncodeCaller = zapcore.ShortCallerEncoder

	// File Encoder (JSON format for machine readability)
	fileEncoderCfg := zap.NewProductionEncoderConfig()
	fileEncoderCfg.TimeKey = "timestamp"
	fileEncoderCfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder // Use standard format
	fileEncoderCfg.EncodeCaller = zapcore.ShortCallerEncoder

	return consoleEncoderCfg, fileEncoderCfg
}

// --- EXPORTED Write Syncers Helper --- // RENAMED TO START WITH UPPERCASE
// CreateFileConsoleWriteSyncers creates standard write syncers for console and file.
func CreateFileConsoleWriteSyncers(cfg *config.Config) (zapcore.WriteSyncer, zapcore.WriteSyncer, error) {
	// Console Writer
	consoleWriter := zapcore.Lock(os.Stdout)

	// Rotating File Writer
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.LogFilePath,
		MaxSize:    cfg.LogMaxSize,    // megabytes
		MaxBackups: cfg.LogMaxBackups, // number of backups
		MaxAge:     cfg.LogMaxAge,     // days
		Compress:   cfg.LogCompress,   // disabled by default
		LocalTime:  true,              // use local time for timestamps in filenames
	})

	return consoleWriter, fileWriter, nil
}

// InitFileConsoleLogger initializes a Zap logger with only File and Console outputs.
// Suitable for early stages or as a base logger.
func InitFileConsoleLogger(cfg *config.Config) (*zap.Logger, error) {
	logLevel := zapcore.InfoLevel
	if cfg.AppEnv == "local" || cfg.AppEnv == "development" {
		logLevel = zapcore.DebugLevel
	}

	// Use exported helpers (calling exported versions now)
	consoleEncoderCfg, fileEncoderCfg := CreateFileConsoleEncoderConfigs()
	consoleWriter, fileWriter, err := CreateFileConsoleWriteSyncers(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create file/console writers: %w", err)
	}

	consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(consoleEncoderCfg), consoleWriter, logLevel)
	fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(fileEncoderCfg), fileWriter, logLevel)
	core := zapcore.NewTee(consoleCore, fileCore)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))

	logger.Info("Base file/console logger initialized",
		zap.String("environment", cfg.AppEnv),
		zap.String("level", logLevel.String()),
		zap.String("logFile", cfg.LogFilePath),
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
// Name starts with Uppercase - already exported.
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

	// --- Use MapObjectEncoder to build map ---
	fieldMap := make(map[string]interface{})
	mapEncoder := zapcore.NewMapObjectEncoder()
	for _, field := range allFields {
		field.AddTo(mapEncoder) // Add field directly to the map encoder
	}
	fieldMap = mapEncoder.Fields
	// --- End MapObjectEncoder logic ---

	logEntry := models.LogEntry{
		Timestamp: ent.Time.Local(), // Use Local time
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
			logEntry.Fields = fmt.Sprintf(`{"marshal_error": "%v"}`, err)
		}
	}

	// Insert into SQLite database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.repo.InsertSQLiteLog(ctx, logEntry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to insert log entry into SQLite: %v\n", err)
		return fmt.Errorf("failed to insert log into sqlite via repo: %w", err)
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

// --- Global Logger Access ---

// SetGlobalLogger sets the global logger instance (use during app init).
// ADDED EXPORTED function.
func SetGlobalLogger(l *zap.Logger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = l
}

// GetLogger returns the initialized global logger.
// Provides a fallback development logger if the global one hasn't been set.
// This function was already exported.
func GetLogger() *zap.Logger {
	globalMu.RLock()
	l := globalLogger
	globalMu.RUnlock()

	if l == nil {
		fallbackLogger, _ := zap.NewDevelopment()
		fallbackLogger.Warn("Global logger accessed before being set!")
		return fallbackLogger
	}
	return l
}
