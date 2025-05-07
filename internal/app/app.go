package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go-webapi/internal/repositories"
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
	var fileLogger *zap.Logger   // Renamed from mainLogger: For console, file
	var sqliteLogger *zap.Logger // For dedicated SQLite logging
	var oracleDB *sql.DB
	var sqliteDB *sql.DB
	var cfg *config.Config
	var err error
	var app *fiber.App
	var components *bootstrap.AppComponents
	var fileSyncer zapcore.WriteSyncer
	var logRepo repositories.LogRepository // Needed for logger init

	// --- 1. Load Configuration ---
	tempConfigLogger, _ := zap.NewProduction(zap.ErrorOutput(zapcore.Lock(os.Stderr)))
	cfg, err = config.LoadConfig(tempConfigLogger) // Pass temp logger for config loading phase
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// --- 2. Create SHARED File Writer/Syncer for timberjack ---
	logDir := filepath.Dir(cfg.LogFilePath)
	if logDir != "." && logDir != "/" {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: Failed to ensure log directory %s exists: %v\n", logDir, err)
			os.Exit(1)
		}
	}
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
	fmt.Fprintf(os.Stderr, "[INFO] Shared file syncer created for path: %s with MaxSize: %d MB\n", cfg.LogFilePath, cfg.LogMaxSize)

	// --- 3. Initialize Databases (pass tempConfigLogger for their own init logs) ---
	sqliteDB, err = database.InitSQLite(cfg, tempConfigLogger)
	if err != nil {
		tempConfigLogger.Fatal("Failed to initialize SQLite database", zap.Error(err))
	}
	tempConfigLogger.Info("SQLite database initialized successfully (using temp logger).")

	oracleDB, err = database.InitOracle(cfg, tempConfigLogger)
	if err != nil {
		tempConfigLogger.Error("Error during Oracle DB pool initialization (continuing)", zap.Error(err))
	} else if oracleDB != nil {
		tempConfigLogger.Info("Oracle database pool initialized (using temp logger).")
	}

	// --- 4. Initialize LogRepository (needed for the appLogger's SQLite core) ---
	// Pass tempConfigLogger to repositories for their own internal init logs.
	logRepo = repositories.NewLogRepository(sqliteDB, oracleDB, tempConfigLogger)
	tempConfigLogger.Info("LogRepository initialized (using temp logger).")

	// --- 5. Initialize Loggers ---
	appLoggers, err := logging.InitializeLoggers(cfg, logRepo, fileSyncer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize application loggers: %v\n", err)
		os.Exit(1)
	}
	fileLogger = appLoggers.File
	sqliteLogger = appLoggers.SQLite                   // Can be a Nop logger if disabled
	logging.SetGlobalLoggers(fileLogger, sqliteLogger) // Set global loggers

	// --- 6. Log Loaded Config Details (using the fileLogger) ---
	logLoadedConfigDetails(fileLogger, cfg)

	// --- 7. Initialize Remaining Components (Services, Handlers, Processor) ---
	// Pass fileLogger and potentially sqliteLogger to components that need them
	components, err = bootstrap.InitializeAppComponents(cfg, fileLogger, sqliteLogger, sqliteDB, oracleDB, logRepo)
	if err != nil {
		fileLogger.Fatal("Failed to initialize application components", zap.Error(err))
	}
	logProcessor := components.LogProcessor

	// --- Initialize Fiber App (use fileLogger) ---
	fileLogger.Info("Initializing Fiber application...")
	app = fiber.New(fiber.Config{
		AppName: "go-webapi",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			lg := logging.GetFileLogger() // Use File Logger for unhandled errors
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

	// --- Middleware (use fileLogger or logging.GetFileLogger()) ---
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.AppEnv != "production", StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
		logging.GetFileLogger().Error("Panic recovered", zap.Any("panic_value", e), zap.Stack("stacktrace"))
	}}))
	fileLogger.Info("Configuring CORS", zap.String("origins", cfg.CORSAllowOrigins), zap.String("methods", cfg.CORSAllowMethods), zap.String("headers", cfg.CORSAllowHeaders))
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSAllowOrigins,
		AllowMethods: cfg.CORSAllowMethods,
		AllowHeaders: cfg.CORSAllowHeaders,
	}))
	app.Use(fiberzap.New(fiberzap.Config{Logger: fileLogger, Fields: []string{"status", "method", "url", "ip", "latency", "error"}, Next: func(c *fiber.Ctx) bool { return c.Path() == "/health" || strings.HasPrefix(c.Path(), "/uploads") }}))

	// --- Routes (use fileLogger) ---
	routes.SetupRoutes(app, cfg, fileLogger, components, sqliteDB, oracleDB) // Pass fileLogger for route setup logs

	// --- Start Log Processor (it uses fileLogger internally from bootstrap init) ---
	if logProcessor != nil {
		logProcessor.Start()
	}

	// --- Start Server & Graceful Shutdown (use fileLogger) ---
	serverCtx, cancelServerCtx := context.WithCancel(context.Background())
	defer cancelServerCtx()
	serverStopped := make(chan struct{})
	go func() {
		defer close(serverStopped)
		fileLogger.Info("Starting server", zap.String("address", ":"+cfg.Port), zap.Bool("prefork", os.Getenv("APP_ENV") == "production"))
		if err := app.Listen(":" + cfg.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fileLogger.Error("Server listener failed", zap.Error(err))
			cancelServerCtx()
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	select {
	case s := <-sig:
		fileLogger.Info("Shutdown signal received.", zap.String("signal", s.String()))
	case <-serverCtx.Done():
		fileLogger.Info("Server context cancelled, initiating shutdown.")
	}

	fileLogger.Info("Initiating graceful shutdown...")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancelShutdown()

	if logProcessor != nil {
		logProcessor.Stop()
	}

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		fileLogger.Error("Fiber server shutdown failed", zap.Error(err))
	} else {
		fileLogger.Info("Fiber server gracefully stopped.")
	}
	<-serverStopped
	fileLogger.Info("HTTP listener stopped.")

	fileLogger.Info("Syncing file/console logger before shutdown...")
	_ = fileLogger.Sync()
	if sqliteLogger != nil && sqliteLogger != fileLogger {
		fileLogger.Info("Syncing SQLite logger before shutdown...")
		_ = sqliteLogger.Sync()
	}
	fmt.Println("[INFO] Logger sync attempt completed.")

	if sqliteDB != nil {
		// fileLogger.Info("Closing SQLite database connection...") // Logged via fmt now
		if err := sqliteDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing SQLite database: %v\n", err)
		} else {
			fmt.Println("[INFO] SQLite database connection closed.")
		}
	}
	if oracleDB != nil {
		// fileLogger.Info("Closing Oracle database pool...") // Logged via fmt now
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
		return
	}
	maskedOracleConn := maskOracleConnString(cfg.OracleConnString)
	maskedJWTSecret := "*** MASKED ***"
	if cfg.JWTSecret == "default-secret" {
		maskedJWTSecret = "default-secret (!!!)"
	} else if len(cfg.JWTSecret) == 0 {
		maskedJWTSecret = "--- EMPTY ---"
	}
	fields := []zapcore.Field{
		zap.String("AppEnv", cfg.AppEnv),
		zap.String("Port", cfg.Port),
		zap.String("JWTSecret", maskedJWTSecret),
		zap.String("OracleConnString", maskedOracleConn),
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
		zap.Bool("SQLLiteLogEnabled", cfg.SQLLiteLogEnabled),
		zap.String("SQLLiteLogLevel", cfg.SQLLiteLogLevel),
	}
	logger.Debug("Loaded configuration details", fields...)
}
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
