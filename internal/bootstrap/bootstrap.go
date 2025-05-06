package bootstrap

import (
	"database/sql"
	"os"

	"go-webapi/internal/config"
	"go-webapi/internal/handlers"
	"go-webapi/internal/logging"
	"go-webapi/internal/repositories"
	"go-webapi/internal/services"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// AppComponents holds the initialized components like handlers, processors, and repositories.
type AppComponents struct {
	AuthHandler    *handlers.AuthHandler
	ProfileHandler *handlers.ProfileHandler
	LogProcessor   *logging.LogProcessor
	LogRepo        repositories.LogRepository
	UserRepo       repositories.UserRepository
}

// InitializeAppComponents creates and wires up the application's core components including
// repositories, the final logger, services, handlers, and processors.
// MODIFIED: Accepts fileSyncer
func InitializeAppComponents(
	cfg *config.Config,
	baseLogger *zap.Logger, // Used for repo init logs
	sqliteDB *sql.DB,
	oracleDB *sql.DB,               // Can be nil if initialization failed
	fileSyncer zapcore.WriteSyncer, // <-- ADDED shared file syncer
) (*AppComponents, *zap.Logger, error) {

	baseLogger.Info("Initializing application components: Repositories, Logger, Services, Handlers, Processors...")

	// --- 1. Initialize Repositories ---
	logRepo := repositories.NewLogRepository(sqliteDB, oracleDB, baseLogger)
	userRepo := repositories.NewOracleUserRepository(oracleDB, baseLogger)
	baseLogger.Info("Repositories initialized.")

	// --- 2. Setup Final Logger (including SQLite Core) ---
	var logLevel zapcore.Level
	if err := logLevel.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		baseLogger.Warn("Invalid log level specified in config for final logger, defaulting to info",
			zap.String("configuredLevel", cfg.LogLevel),
			zap.Error(err),
		)
		logLevel = zapcore.InfoLevel
	}
	baseLogger.Info("Final logger level set", zap.String("level", logLevel.String()))

	// Get encoders
	consoleEncoderCfg, fileEncoderCfg := logging.CreateFileConsoleEncoderConfigs()

	// Create Console Syncer ONLY
	consoleSyncer := zapcore.Lock(os.Stdout)

	// --- REMOVED call to logging.CreateFileConsoleWriteSyncers ---

	// Create Cores using passed-in fileSyncer
	consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(consoleEncoderCfg), consoleSyncer, logLevel)
	fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(fileEncoderCfg), fileSyncer, logLevel) // <-- Use passed-in fileSyncer
	sqliteEncoderCfg := fileEncoderCfg
	sqliteJSONEncoder := zapcore.NewJSONEncoder(sqliteEncoderCfg)
	sqliteCore := logging.NewSQLiteCore(logLevel, sqliteJSONEncoder, sqliteEncoderCfg, logRepo)

	finalTeeCore := zapcore.NewTee(consoleCore, fileCore, sqliteCore)
	finalLogger := zap.New(finalTeeCore, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	finalLogger.Info("Final logger (Console, File, SQLite) initialized.")

	// --- 3. Initialize Services ---
	authService := services.NewAuthService(userRepo, finalLogger, cfg.JWTSecret)
	profileService := services.NewProfileService(userRepo, finalLogger)
	finalLogger.Info("Services initialized.")

	// --- 4. Initialize Handlers ---
	authHandler := handlers.NewAuthHandler(authService, finalLogger, cfg.UploadDir)
	profileHandler := handlers.NewProfileHandler(profileService, finalLogger)
	finalLogger.Info("Handlers initialized.")

	// --- 5. Initialize Processors ---
	logProcessor := logging.NewLogProcessor(cfg, logRepo, finalLogger)
	finalLogger.Info("Processors initialized.")

	finalLogger.Info("Application components initialization complete.")

	components := &AppComponents{
		AuthHandler:    authHandler,
		ProfileHandler: profileHandler,
		LogProcessor:   logProcessor,
		LogRepo:        logRepo,
		UserRepo:       userRepo,
	}

	return components, finalLogger, nil
}
