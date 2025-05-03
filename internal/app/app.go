package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go-webapi/internal/bootstrap" // <-- Import bootstrap
	"go-webapi/internal/config"
	"go-webapi/internal/database"
	"go-webapi/internal/logging"
	routes "go-webapi/internal/routes"

	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

// Run initializes and starts the application
func Run() {
	// Declare variables early
	var baseLogger *zap.Logger
	var finalLogger *zap.Logger // Will be initialized by bootstrap
	var oracleDB *sql.DB
	var sqliteDB *sql.DB
	var cfg *config.Config
	var err error
	var app *fiber.App
	var components *bootstrap.AppComponents

	// --- Load Configuration ---
	cfg, err = config.LoadConfig(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// --- Initialize Base Logger (File/Console) ---
	// Used for DB init and passed to bootstrap
	baseLogger, err = logging.InitFileConsoleLogger(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize base logger: %v\n", err)
		os.Exit(1)
	}
	baseLogger.Info("Base file/console logger initialized.")

	// --- Initialize SQLite Database ---
	sqliteDB, err = database.InitSQLite(cfg, baseLogger)
	if err != nil {
		baseLogger.Fatal("Failed to initialize SQLite database", zap.Error(err))
	}
	baseLogger.Info("SQLite database initialized successfully.")

	// --- Initialize Oracle DB Handle ---
	oracleDB, err = database.InitOracle(cfg, baseLogger)
	if err != nil {
		// Log with baseLogger as finalLogger isn't ready
		baseLogger.Error("Error during Oracle DB pool initialization (continuing)", zap.Error(err))
	} else if oracleDB != nil {
		baseLogger.Info("Oracle database pool initialized.")
	}

	// Call bootstrap function, passing baseLogger and DB handles
	components, finalLogger, err = bootstrap.InitializeAppComponents(cfg, baseLogger, sqliteDB, oracleDB)
	if err != nil {
		// Use baseLogger for fatal error as finalLogger might not be valid
		baseLogger.Fatal("Failed to initialize application components", zap.Error(err))
	}

	// Update global logger with the fully configured finalLogger
	logging.SetGlobalLogger(finalLogger)
	// Get log processor for shutdown step
	logProcessor := components.LogProcessor

	// --- Initialize Fiber App ---
	// Use finalLogger for Fiber setup and error handling
	finalLogger.Info("Initializing Fiber application...")
	app = fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			lg := logging.GetLogger() // Get the globally set finalLogger
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
	// Use finalLogger where needed
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.AppEnv != "production", StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
		logging.GetLogger().Error("Panic recovered", zap.Any("panic_value", e), zap.Stack("stacktrace"))
	}}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // IMPORTANT: Restrict this in production
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, HEAD, PUT, DELETE, PATCH",
	}))
	// Pass the finalLogger returned by bootstrap to FiberZap
	app.Use(fiberzap.New(fiberzap.Config{Logger: finalLogger, Fields: []string{"status", "method", "url", "ip", "latency", "error"}, Next: func(c *fiber.Ctx) bool { return c.Path() == "/health" || strings.HasPrefix(c.Path(), "/uploads") }}))

	// --- Routes ---
	// Pass handlers from components struct and finalLogger
	routes.SetupRoutes(
		app,
		cfg,
		finalLogger, // Pass the initialized final logger
		components,
		sqliteDB,
		oracleDB,
	)

	// --- Start Log Processor ---
	if logProcessor != nil {
		logProcessor.Start()
	}

	// --- Start Server & Graceful Shutdown ---
	// Use finalLogger for lifecycle logs
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

	// Shutdown sequence (using finalLogger until DBs are closed)
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

	// 3. Wait for listener
	<-serverStopped
	finalLogger.Info("HTTP listener stopped.")

	// 4. Sync Logger
	finalLogger.Info("Syncing logger...")
	_ = finalLogger.Sync() // Use finalLogger here

	// 5. Close Databases
	if sqliteDB != nil {
		finalLogger.Info("Closing SQLite database connection...") // Use finalLogger
		if err := sqliteDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing SQLite database: %v\n", err)
		} else {
			fmt.Println("[INFO] SQLite database connection closed.")
		}
	}
	if oracleDB != nil {
		finalLogger.Info("Closing Oracle database pool...") // Use finalLogger
		if err := oracleDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing Oracle database pool: %v\n", err)
		} else {
			fmt.Println("[INFO] Oracle database pool closed.")
		}
	}

	fmt.Println("[INFO] Logger sync attempt completed.")
	fmt.Println("[INFO] Application shutdown complete.")
}

// NOTE: Ensure logging helpers (CreateFileConsoleEncoderConfigs, CreateFileConsoleWriteSyncers, NewSQLiteCore)
// are EXPORTED from the logging package for use in bootstrap.go.
