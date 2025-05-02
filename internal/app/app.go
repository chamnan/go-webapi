package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt" // For initial error printing and final logging
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go-webapi/internal/config"
	"go-webapi/internal/database"
	"go-webapi/internal/handlers"
	"go-webapi/internal/logging" // Need for logger setup helpers
	"go-webapi/internal/repositories"
	routes "go-webapi/internal/routes" // <-- Import for routes
	"go-webapi/internal/services"

	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors" // <-- CORS middleware import
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore" // Import zapcore for manual core setup
)

// Run initializes and starts the application
func Run() {
	// Declare variables early
	var baseLogger *zap.Logger
	var finalLogger *zap.Logger
	var logProcessor *logging.LogProcessor
	var oracleDB *sql.DB
	var sqliteDB *sql.DB
	var cfg *config.Config
	var err error
	var logRepo repositories.LogRepository
	var userRepo repositories.UserRepository
	var app *fiber.App

	// --- Load Configuration ---
	cfg, err = config.LoadConfig(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// --- Initialize Base Logger (File/Console) ---
	baseLogger, err = logging.InitFileConsoleLogger(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize base logger: %v\n", err)
		os.Exit(1)
	}
	baseLogger.Info("Base file/console logger initialized.")
	finalLogger = baseLogger // Temporarily assign baseLogger, will be replaced

	// --- Initialize SQLite Database ---
	sqliteDB, err = database.InitSQLite(cfg, baseLogger)
	if err != nil {
		baseLogger.Fatal("Failed to initialize SQLite database", zap.Error(err))
	}
	baseLogger.Info("SQLite database initialized successfully.")

	// --- Initialize Oracle DB Handle (Non-blocking Setup) ---
	oracleDB, err = database.InitOracle(cfg, baseLogger)
	if err != nil {
		baseLogger.Error("Error during Oracle DB pool initialization (continuing)", zap.Error(err))
	} else if oracleDB != nil {
		baseLogger.Info("Oracle database pool initialized.")
	}

	// --- Initialize Repositories ---
	logRepo = repositories.NewLogRepository(sqliteDB, oracleDB, baseLogger)
	userRepo = repositories.NewOracleUserRepository(oracleDB, baseLogger)

	// --- Setup Final Logger with SQLite Core ---
	// Assuming CreateFileConsoleEncoderConfigs, CreateFileConsoleWriteSyncers, NewSQLiteCore are exported from logging
	logLevel := zapcore.InfoLevel
	if cfg.AppEnv == "local" || cfg.AppEnv == "development" {
		logLevel = zapcore.DebugLevel
	}
	consoleEncoderCfg, fileEncoderCfg := logging.CreateFileConsoleEncoderConfigs()
	consoleWriter, fileWriter, err := logging.CreateFileConsoleWriteSyncers(cfg)
	if err != nil {
		baseLogger.Fatal("Failed to recreate file/console writers for final logger", zap.Error(err))
	}
	consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(consoleEncoderCfg), consoleWriter, logLevel)
	fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(fileEncoderCfg), fileWriter, logLevel)
	sqliteEncoderCfg := fileEncoderCfg
	sqliteJSONEncoder := zapcore.NewJSONEncoder(sqliteEncoderCfg)
	sqliteCore := logging.NewSQLiteCore(logLevel, sqliteJSONEncoder, sqliteEncoderCfg, logRepo)

	finalTeeCore := zapcore.NewTee(consoleCore, fileCore, sqliteCore)
	finalLogger = zap.New(finalTeeCore, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	finalLogger.Info("Final logger initialized with Console, File, and SQLite destinations.")
	logging.SetGlobalLogger(finalLogger) // Set the logger globally

	// --- Initialize Services, Handlers, Processor ---
	authService := services.NewAuthService(userRepo, finalLogger, cfg.JWTSecret)
	profileService := services.NewProfileService(userRepo, finalLogger)
	authHandler := handlers.NewAuthHandler(authService, finalLogger, cfg.UploadDir)
	profileHandler := handlers.NewProfileHandler(profileService, finalLogger)
	logProcessor = logging.NewLogProcessor(cfg, logRepo, finalLogger)

	// --- Initialize Fiber App ---
	finalLogger.Info("Initializing Fiber application...")
	app = fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			lg := logging.GetLogger() // Use the globally set logger
			code := fiber.StatusInternalServerError
			var e *fiber.Error
			if errors.As(err, &e) {
				code = e.Code
			}
			fields := []zap.Field{zap.Int("status", code), zap.Error(err), zap.String("path", c.Path()), zap.String("method", c.Method()), zap.String("ip", c.IP())}
			if code == fiber.StatusNotFound {
				lg.Warn("Resource not found", fields...)
			} else {
				lg.Error("Unhandled error", fields...)
			}
			resp := fiber.Map{"error": fiber.ErrNotFound.Message}
			if code != fiber.StatusNotFound {
				resp["error"] = "An internal server error occurred"
				if cfg.AppEnv != "production" {
					resp["detail"] = err.Error()
				}
			}
			return c.Status(code).JSON(resp)
		},
	})

	// --- Middleware ---
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.AppEnv != "production", StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
		logging.GetLogger().Error("Panic recovered", zap.Any("panic_value", e), zap.Stack("stacktrace"))
	}}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // IMPORTANT: Restrict this in production
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, HEAD, PUT, DELETE, PATCH",
	}))
	app.Use(fiberzap.New(fiberzap.Config{Logger: finalLogger, Fields: []string{"status", "method", "url", "ip", "latency", "error"}, Next: func(c *fiber.Ctx) bool { return c.Path() == "/health" || strings.HasPrefix(c.Path(), "/uploads") }}))

	// --- Routes ---
	routes.SetupRoutes(app, cfg, finalLogger, authHandler, profileHandler, sqliteDB, oracleDB)

	// --- Start Log Processor ---
	logProcessor.Start()

	// --- Start Server & Graceful Shutdown ---
	serverCtx, cancelServerCtx := context.WithCancel(context.Background())
	defer cancelServerCtx()
	serverStopped := make(chan struct{})

	go func() {
		defer close(serverStopped)
		finalLogger.Info("Starting server", zap.String("address", ":"+cfg.Port), zap.Bool("prefork", os.Getenv("APP_ENV") == "production"))
		if err := app.Listen(":" + cfg.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			finalLogger.Error("Server listener failed", zap.Error(err))
			cancelServerCtx()
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	select {
	case s := <-sig:
		finalLogger.Info("Shutdown signal received.", zap.String("signal", s.String()))
	case <-serverCtx.Done():
		finalLogger.Info("Server context cancelled, initiating shutdown.")
	}

	finalLogger.Info("Initiating graceful shutdown...")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancelShutdown()

	// 1. Stop Log Processor
	if logProcessor != nil {
		finalLogger.Info("Stopping log processor...")
		logProcessor.Stop()
		finalLogger.Info("Log processor stopped.")
	}

	// 2. Shutdown Fiber server
	finalLogger.Info("Shutting down Fiber server...")
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		finalLogger.Error("Fiber server shutdown failed", zap.Error(err))
	} else {
		finalLogger.Info("Fiber server gracefully stopped.")
	}

	// 3. Wait for the listener goroutine to stop
	<-serverStopped
	finalLogger.Info("HTTP listener stopped.") // <-- Still safe, DBs are open

	// 4. Flush logger buffers (attempt to write pending logs before closing DB)
	finalLogger.Info("Syncing logger...") // <-- Still safe, DBs are open
	_ = finalLogger.Sync()                // Ignore sync error, might happen in abrupt shutdown
	// finalLogger.Info("Logger sync completed.") // <-- Let's move this after DB close using fmt

	// 5. Close Database Connections
	if sqliteDB != nil {
		finalLogger.Info("Closing SQLite database connection...") // <-- Log BEFORE closing
		if err := sqliteDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing SQLite database: %v\n", err)
			// baseLogger might be safer here if it ONLY writes to console/file
			// baseLogger.Error("Error closing SQLite database", zap.Error(err))
		} else {
			// Use fmt.Println instead of finalLogger AFTER closing
			fmt.Println("***[INFO] SQLite database connection closed.") // <-- MODIFIED
		}
	}

	if oracleDB != nil {
		finalLogger.Info("Closing Oracle database pool...") // <-- Log BEFORE closing (safe)
		if err := oracleDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing Oracle database pool: %v\n", err)
			// baseLogger.Error("Error closing Oracle database pool", zap.Error(err))
		} else {
			// Use fmt.Println instead of finalLogger AFTER closing
			fmt.Println("***[INFO] Oracle database pool closed.") // <-- MODIFIED
		}
	}

	// Log sync completion and final message using fmt.Println
	fmt.Println("***[INFO] Logger sync attempt completed.") // <-- MODIFIED (moved and changed)
	fmt.Println("***[INFO] Application shutdown complete.") // <-- MODIFIED
}

// NOTE: Ensure logging helpers are exported as mentioned before.
