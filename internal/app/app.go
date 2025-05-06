package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath" // Import filepath
	"strings"
	"syscall"
	"time"

	"go-webapi/internal/bootstrap"
	"go-webapi/internal/config"
	"go-webapi/internal/database"
	"go-webapi/internal/logging"
	routes "go-webapi/internal/routes"

	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	// Use the correct import based on your logger.go
	"github.com/DeRuina/timberjack"
)

// Run initializes and starts the application
func Run() {
	// Declare variables early
	var baseLogger *zap.Logger
	var finalLogger *zap.Logger
	var oracleDB *sql.DB
	var sqliteDB *sql.DB
	var cfg *config.Config
	var err error
	var app *fiber.App
	var components *bootstrap.AppComponents
	var fileSyncer zapcore.WriteSyncer // Variable for the shared file syncer

	// --- Load Configuration ---
	// Passing nil here means config loading warnings/errors won't use Zap
	// but fatal errors will still cause exit via fmt/os.Exit.
	cfg, err = config.LoadConfig(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// --- Create SHARED File Writer/Syncer --- // <-- MOVED UP & CORRECTED
	// Ensure log directory exists first (config loader should also do this)
	logDir := filepath.Dir(cfg.LogFilePath)
	if logDir != "." {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: Failed to ensure log directory exists %s: %v\n", logDir, err)
			os.Exit(1)
		}
	}
	// Use timberjack based on logger.go
	timberJackLogger := &timberjack.Logger{
		Filename:         cfg.LogFilePath,
		MaxSize:          cfg.LogMaxSize,
		MaxBackups:       cfg.LogMaxBackups,
		MaxAge:           cfg.LogMaxAge,
		Compress:         cfg.LogCompress,
		LocalTime:        true,
		RotationInterval: time.Duration(cfg.LogRotateInterval) * time.Hour,
	}
	fileSyncer = zapcore.AddSync(timberJackLogger)
	fmt.Fprintf(os.Stderr, "[INFO] Shared file syncer created for path: %s\n", cfg.LogFilePath) // Use fmt before logger exists
	// --- End File Writer Setup ---

	// --- Initialize Base Logger (using shared fileSyncer) ---
	baseLogger, err = logging.InitFileConsoleLogger(cfg, fileSyncer) // Pass shared syncer
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize base logger: %v\n", err)
		os.Exit(1)
	}
	// baseLogger.Info("Base file/console logger initialized.") // Logged inside InitFileConsoleLogger

	// --- Initialize Databases (use baseLogger) ---
	sqliteDB, err = database.InitSQLite(cfg, baseLogger)
	if err != nil {
		baseLogger.Fatal("Failed to initialize SQLite database", zap.Error(err))
	}
	// baseLogger.Info("SQLite database initialized successfully.") // Logged inside InitSQLite if successful

	oracleDB, err = database.InitOracle(cfg, baseLogger)
	if err != nil {
		baseLogger.Error("Error during Oracle DB pool initialization (continuing)", zap.Error(err))
	}
	// baseLogger.Info("Oracle database pool initialized.") // Logged inside InitOracle if successful

	// --- Initialize Components (passing shared fileSyncer) ---
	components, finalLogger, err = bootstrap.InitializeAppComponents(cfg, baseLogger, sqliteDB, oracleDB, fileSyncer) // Pass fileSyncer
	if err != nil {
		baseLogger.Fatal("Failed to initialize application components", zap.Error(err))
	}
	logging.SetGlobalLogger(finalLogger) // Set global logger AFTER it's created
	logProcessor := components.LogProcessor
	// finalLogger.Info("Final logger initialized.") // Logged inside InitializeAppComponents

	// --- Log Final Configuration Values using finalLogger ---
	logLoadedConfigDetails(finalLogger, cfg)

	// --- Initialize Fiber App ---
	finalLogger.Info("Initializing Fiber application...")
	app = fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error { /* ... error handler ... */
			lg := logging.GetLogger()
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
			resp := fiber.Map{"error": "An unexpected error occurred"}
			if cfg.AppEnv != "production" {
				resp["detail"] = err.Error()
			}
			return c.Status(code).JSON(resp)
		},
	})

	// --- Middleware (use finalLogger) ---
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.AppEnv != "production", StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
		logging.GetLogger().Error("Panic recovered", zap.Any("panic_value", e), zap.Stack("stacktrace"))
	}}))
	finalLogger.Info("Configuring CORS", zap.String("origins", cfg.CORSAllowOrigins), zap.String("methods", cfg.CORSAllowMethods), zap.String("headers", cfg.CORSAllowHeaders))
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSAllowOrigins,
		AllowMethods: cfg.CORSAllowMethods,
		AllowHeaders: cfg.CORSAllowHeaders,
	}))
	app.Use(fiberzap.New(fiberzap.Config{Logger: finalLogger, Fields: []string{"status", "method", "url", "ip", "latency", "error"}, Next: func(c *fiber.Ctx) bool { return c.Path() == "/health" || strings.HasPrefix(c.Path(), "/uploads") }}))

	// --- Routes (use finalLogger) ---
	routes.SetupRoutes(app, cfg, finalLogger, components, sqliteDB, oracleDB)

	// --- Start Log Processor ---
	if logProcessor != nil {
		logProcessor.Start()
	}

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
	if logProcessor != nil {
		logProcessor.Stop()
	} // Stop processor first
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		finalLogger.Error("Fiber server shutdown failed", zap.Error(err))
	} else {
		finalLogger.Info("Fiber server gracefully stopped.")
	}
	<-serverStopped
	finalLogger.Info("HTTP listener stopped.")

	// Sync logger BEFORE closing DBs
	finalLogger.Info("Syncing logger before shutdown...")
	_ = finalLogger.Sync()
	// Closing the timberjack logger explicitly isn't standard, Sync should be sufficient
	fmt.Println("[INFO] Logger sync attempt completed.")

	// Close DBs
	if sqliteDB != nil {
		finalLogger.Info("Closing SQLite database connection...")
		if err := sqliteDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing SQLite database: %v\n", err)
		} else {
			fmt.Println("[INFO] SQLite database connection closed.")
		}
	}
	if oracleDB != nil {
		finalLogger.Info("Closing Oracle database pool...")
		if err := oracleDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing Oracle database pool: %v\n", err)
		} else {
			fmt.Println("[INFO] Oracle database pool closed.")
		}
	}

	fmt.Println("[INFO] Application shutdown complete.")
}

func logLoadedConfigDetails(logger *zap.Logger, cfg *config.Config) {
	if logger == nil || cfg == nil {
		return // Should not happen in normal flow, but check anyway
	}

	// Mask sensitive data
	maskedOracleConn := maskOracleConnString(cfg.OracleConnString) // Use helper
	maskedJWTSecret := "*** MASKED ***"
	if cfg.JWTSecret == "default-secret" {
		maskedJWTSecret = "default-secret (!!!)"
	} else if len(cfg.JWTSecret) == 0 {
		maskedJWTSecret = "--- EMPTY ---"
	}

	// Create fields array
	fields := []zapcore.Field{
		zap.String("AppEnv", cfg.AppEnv),
		zap.String("Port", cfg.Port),
		zap.String("JWTSecret", maskedJWTSecret),         // Masked
		zap.String("OracleConnString", maskedOracleConn), // Masked
		zap.Int("OracleMaxPoolOpenConns", cfg.OracleMaxPoolOpenConns),
		zap.Int("OracleMaxPoolIdleConns", cfg.OracleMaxPoolIdleConns),
		zap.Int("OracleMaxPoolConnLifetimeMinutes", cfg.OracleMaxPoolConnLifetimeMinutes),
		zap.Int("OracleMaxPoolConnIdleTimeMinutes", cfg.OracleMaxPoolConnIdleTimeMinutes),
		zap.String("SQLiteDBPath", cfg.SQLiteDBPath),
		zap.String("LogFilePath", cfg.LogFilePath),
		zap.String("LogLevel", cfg.LogLevel),
		zap.Int("LogMaxSizeMB", cfg.LogMaxSize),
		zap.Int("LogMaxBackups", cfg.LogMaxBackups),
		zap.Int("LogMaxAgeDays", cfg.LogMaxAge),
		zap.Bool("LogCompress", cfg.LogCompress),
		zap.Duration("LogBatchInterval", cfg.LogBatchInterval),
		zap.Int("LogProcessorBatchSize", cfg.LogProcessorBatchSize),
		zap.Int("LogProcessorOracleRetryAttempts", cfg.LogProcessorOracleRetryAttempts),
		zap.Int("LogProcessorOracleRetryDelaySeconds", cfg.LogProcessorOracleRetryDelaySeconds),
		zap.String("UploadDir", cfg.UploadDir),
		zap.String("CORSAllowOrigins", cfg.CORSAllowOrigins),
		zap.String("CORSAllowMethods", cfg.CORSAllowMethods),
		zap.String("CORSAllowHeaders", cfg.CORSAllowHeaders),
	}

	// Log using the passed-in (final) logger
	logger.Debug("Loaded configuration details", fields...)
}

// maskOracleConnString helper needs to be defined or imported if moved outside config.go
// Added Helper function within app.go for simplicity here
func maskOracleConnString(connStr string) string {
	prefix := "oracle://"
	if !strings.HasPrefix(connStr, prefix) {
		return "*** UNKNOWN FORMAT ***"
	}
	parts := strings.SplitN(connStr[len(prefix):], "@", 2)
	if len(parts) < 2 {
		return connStr
	}
	authPart := parts[0]
	hostPart := parts[1]
	authParts := strings.SplitN(authPart, ":", 2)
	user := authParts[0]
	if len(authParts) < 2 || authParts[1] == "" {
		return fmt.Sprintf("%s%s@%s", prefix, user, hostPart)
	}
	return fmt.Sprintf("%s%s:***MASKED***@%s", prefix, user, hostPart)
}
