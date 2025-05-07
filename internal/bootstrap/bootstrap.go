package bootstrap

import (
	"database/sql"
	// "fmt"
	// "os"

	"go-webapi/internal/config"
	"go-webapi/internal/handlers"
	"go-webapi/internal/logging"
	"go-webapi/internal/repositories"
	"go-webapi/internal/services"

	"go.uber.org/zap"
	// "go.uber.org/zap/zapcore" // No longer needed here for logger creation
)

type AppComponents struct {
	AuthHandler    *handlers.AuthHandler
	ProfileHandler *handlers.ProfileHandler
	LogProcessor   *logging.LogProcessor
	UserRepo       repositories.UserRepository
	FileLogger     *zap.Logger // Renamed from MainLogger
	SQLiteLogger   *zap.Logger // Could be nil or Nop logger
}

// InitializeAppComponents now uses the file/console application logger (passed as fileLogger).
func InitializeAppComponents(
	cfg *config.Config,
	fileLogger *zap.Logger, // Renamed from mainLogger
	sqliteLogger *zap.Logger,
	sqliteDB *sql.DB,
	oracleDB *sql.DB,
	logRepo repositories.LogRepository,
) (*AppComponents, error) {

	fileLogger.Info("Initializing application components (Services, Handlers, Processor)...")
	sqliteLogger.Info("Initializing application components (Services, Handlers, Processor)...")
	// --- Initialize User Repository ---
	userRepo := repositories.NewOracleUserRepository(oracleDB, fileLogger) // Use fileLogger for repo's operational logs
	fileLogger.Info("User Repository initialized.")

	// --- Initialize Services ---
	// Services get the fileLogger for their general logs.
	// If a service needs to write specific audit events to SQLite, it uses the passed sqliteLogger.
	authService := services.NewAuthService(userRepo, fileLogger, sqliteLogger, cfg.JWTSecret)
	profileService := services.NewProfileService(userRepo, fileLogger) // Assuming profile service only needs general logs
	fileLogger.Info("Services initialized.")

	// --- Initialize Handlers ---
	authHandler := handlers.NewAuthHandler(authService, fileLogger, cfg.UploadDir) // Handlers use fileLogger for request-cycle logs
	profileHandler := handlers.NewProfileHandler(profileService, fileLogger)
	fileLogger.Info("Handlers initialized.")

	// --- Initialize Processors ---
	logProcessor := logging.NewLogProcessor(cfg, logRepo, fileLogger) // LogProcessor uses fileLogger for its operational logs
	fileLogger.Info("Processors initialized.")

	fileLogger.Info("Application components initialization complete.")

	components := &AppComponents{
		AuthHandler:    authHandler,
		ProfileHandler: profileHandler,
		LogProcessor:   logProcessor,
		UserRepo:       userRepo,
		FileLogger:     fileLogger,
		SQLiteLogger:   sqliteLogger,
	}

	return components, nil
}
