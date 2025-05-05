package bootstrap

import (
	"database/sql" // Import database/sql

	"go-webapi/internal/config"
	"go-webapi/internal/handlers"
	"go-webapi/internal/logging"
	"go-webapi/internal/repositories"
	"go-webapi/internal/services"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore" // Import zapcore
)

// AppComponents holds the initialized components like handlers, processors, and repositories.
type AppComponents struct {
	AuthHandler    *handlers.AuthHandler
	ProfileHandler *handlers.ProfileHandler
	LogProcessor   *logging.LogProcessor
	LogRepo        repositories.LogRepository
	UserRepo       repositories.UserRepository
	// Add other components here as needed
}

// InitializeAppComponents creates and wires up the application's core components including
// repositories, the final logger, services, handlers, and processors.
func InitializeAppComponents(
	cfg *config.Config,
	baseLogger *zap.Logger, // Use base logger for repo init and final logger setup
	sqliteDB *sql.DB,
	oracleDB *sql.DB, // Can be nil if initialization failed
) (*AppComponents, *zap.Logger, error) {

	baseLogger.Info("Initializing application components: Repositories, Logger, Services, Handlers, Processors...")

	// --- 1. Initialize Repositories ---
	// Repositories depend on DB connections and baseLogger
	logRepo := repositories.NewLogRepository(sqliteDB, oracleDB, baseLogger)
	userRepo := repositories.NewOracleUserRepository(oracleDB, baseLogger) // Handles nil oracleDB internally
	baseLogger.Info("Repositories initialized.")

	// --- 2. Setup Final Logger (including SQLite Core) ---
	// This depends on logRepo being initialized
	var logLevel zapcore.Level
	err := logLevel.UnmarshalText([]byte(cfg.LogLevel)) // Parse level from config string
	if err != nil {
		baseLogger.Warn("Invalid log level..., defaulting to info")
		logLevel = zapcore.InfoLevel // Default to Info on parsing error
	}

	// Assume these helpers are exported from logging package
	consoleEncoderCfg, fileEncoderCfg := logging.CreateFileConsoleEncoderConfigs()
	consoleWriter, fileWriter, err := logging.CreateFileConsoleWriteSyncers(cfg)
	if err != nil {
		// Use baseLogger here as finalLogger isn't ready yet
		baseLogger.Error("Failed to create file/console writers for final logger", zap.Error(err))
		return nil, nil, err // Return error
	}
	consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(consoleEncoderCfg), consoleWriter, logLevel)
	fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(fileEncoderCfg), fileWriter, logLevel)
	// Create SQLite core using the initialized logRepo
	sqliteEncoderCfg := fileEncoderCfg
	sqliteJSONEncoder := zapcore.NewJSONEncoder(sqliteEncoderCfg)
	sqliteCore := logging.NewSQLiteCore(logLevel, sqliteJSONEncoder, sqliteEncoderCfg, logRepo) // Pass logRepo

	finalTeeCore := zapcore.NewTee(consoleCore, fileCore, sqliteCore)
	finalLogger := zap.New(finalTeeCore, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	finalLogger.Info("Final logger (Console, File, SQLite) initialized.")

	// --- 3. Initialize Services ---
	// Services depend on repositories, finalLogger, config
	authService := services.NewAuthService(userRepo, finalLogger, cfg.JWTSecret)
	profileService := services.NewProfileService(userRepo, finalLogger)
	finalLogger.Info("Services initialized.")

	// --- 4. Initialize Handlers ---
	// Handlers depend on services, finalLogger, config
	authHandler := handlers.NewAuthHandler(authService, finalLogger, cfg.UploadDir)
	profileHandler := handlers.NewProfileHandler(profileService, finalLogger)
	finalLogger.Info("Handlers initialized.")

	// --- 5. Initialize Processors ---
	// Processors depend on config, repositories, finalLogger
	logProcessor := logging.NewLogProcessor(cfg, logRepo, finalLogger)
	finalLogger.Info("Processors initialized.")

	finalLogger.Info("Application components initialization complete.")

	// Return all initialized components and the final logger
	components := &AppComponents{
		AuthHandler:    authHandler,
		ProfileHandler: profileHandler,
		LogProcessor:   logProcessor,
		LogRepo:        logRepo,
		UserRepo:       userRepo,
	}

	return components, finalLogger, nil // Return components, logger, nil error
}
