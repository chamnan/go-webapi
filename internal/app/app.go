package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go-webapi/internal/repositories"
	// "net" // No longer needed for banner
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	// "runtime" // No longer needed for banner
	"strings"
	"syscall"
	"time"

	"go-webapi/internal/bootstrap"
	"go-webapi/internal/config"
	"go-webapi/internal/database"
	"go-webapi/internal/logging"
	"go-webapi/internal/middleware" // Ensure middleware package is imported
	routes "go-webapi/internal/routes"

	"github.com/DeRuina/timberjack"
	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Run initializes and starts the application
func Run() {
	var fileLogger *zap.Logger   // For console, file
	var sqliteLogger *zap.Logger // For dedicated SQLite logging
	var oracleDB *sql.DB
	var sqliteDB *sql.DB
	var cfg *config.Config
	var err error
	var app *fiber.App // Declare app variable earlier
	var components *bootstrap.AppComponents
	var fileSyncer zapcore.WriteSyncer
	var logRepo repositories.LogRepository

	// --- 1. Load Configuration ---
	tempConfigLogger, _ := zap.NewProduction(zap.ErrorOutput(zapcore.Lock(os.Stderr)))
	cfg, err = config.LoadConfig(tempConfigLogger)
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

	// --- 3. Initialize Databases ---
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

	// --- 4. Initialize LogRepository ---
	logRepo = repositories.NewLogRepository(sqliteDB, oracleDB, tempConfigLogger)
	tempConfigLogger.Info("LogRepository initialized (using temp logger).")

	// --- 5. Initialize Loggers ---
	appLoggers, err := logging.InitializeLoggers(cfg, logRepo, fileSyncer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize application loggers: %v\n", err)
		os.Exit(1)
	}
	fileLogger = appLoggers.File
	sqliteLogger = appLoggers.SQLite
	logging.SetGlobalLoggers(fileLogger, sqliteLogger)

	// --- 6. Log Loaded Config Details ---
	logLoadedConfigDetails(fileLogger, cfg)

	// --- 7. Initialize Fiber App ---
	fileLogger.Info("Initializing Fiber application...")
	app = fiber.New(fiber.Config{
		Prefork:               os.Getenv("APP_ENV") == "production", // Set Prefork based on Env
		DisableStartupMessage: false,                                // Disable default banner
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			// Use request-scoped logger - THIS IS THE FIX
			lg := middleware.GetRequestFileLogger(c)
			code := fiber.StatusInternalServerError
			var e *fiber.Error
			// Add nil check for 'e' after errors.As - THIS IS THE PANIC FIX
			if errors.As(err, &e) && e != nil {
				code = e.Code
			} else if err != nil {
				// Log non-fiber errors that might reach here
				lg.Error("Non-fiber error in ErrorHandler", zap.Error(err))
			} else {
				// Log if error handler was called with nil error (shouldn't happen)
				lg.Error("ErrorHandler called with nil error")
			}

			// Ensure 'c' is not nil before accessing methods (extra safety)
			if c == nil {
				fmt.Println("FATAL: fiber.Ctx is nil in ErrorHandler")
				// Cannot return JSON if context is nil
				return errors.New("internal context error") // Or handle differently
			}

			// Proceed with logging and response
			fields := []zap.Field{zap.Int("status", code), zap.Error(err)}
			// Safely add request details
			fields = append(fields, zap.String("path", c.Path()))
			fields = append(fields, zap.String("method", c.Method()))
			fields = append(fields, zap.String("ip", c.IP()))

			if code == fiber.StatusNotFound {
				lg.Warn("Resource not found", fields...)
			} else {
				lg.Error("Unhandled error", fields...)
			}
			resp := fiber.Map{"error": "An unexpected error occurred"}
			// Ensure cfg is not nil before accessing (extra safety)
			if cfg != nil && cfg.AppEnv != "production" {
				if err != nil { // Ensure err is not nil before calling Error()
					resp["detail"] = err.Error()
				} else {
					resp["detail"] = "Error object was nil"
				}
			}
			return c.Status(code).JSON(resp)
		},
	})

	// --- 8. Initialize Remaining Components ---
	components, err = bootstrap.InitializeAppComponents(cfg, fileLogger, sqliteLogger, sqliteDB, oracleDB, logRepo)
	if err != nil {
		fileLogger.Fatal("Failed to initialize application components", zap.Error(err))
	}
	logProcessor := components.LogProcessor

	// --- 9. Register Middleware ---
	// ORDER MATTERS!
	// 9.1. Recovery (to catch panics early)
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.AppEnv != "production", StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
		logger := middleware.GetRequestFileLogger(c)
		logger.Error("Panic recovered", zap.Any("panic_value", e), zap.Stack("stacktrace"))
	}}))

	// 9.2. CORS
	fileLogger.Info("Configuring CORS", zap.String("origins", cfg.CORSAllowOrigins), zap.String("methods", cfg.CORSAllowMethods), zap.String("headers", cfg.CORSAllowHeaders))
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSAllowOrigins,
		AllowMethods: cfg.CORSAllowMethods,
		AllowHeaders: cfg.CORSAllowHeaders,
	}))

	// 9.3. RequestLoggers (to inject scoped loggers for subsequent middleware/handlers)
	app.Use(middleware.RequestLoggers(fileLogger, sqliteLogger))

	// 9.4. RequestDebugLogger (uses the scoped logger injected above)
	app.Use(middleware.RequestDebugLogger())

	// 9.5. FiberZap (access logging - uses base fileLogger, but runs after request loggers are set)
	app.Use(fiberzap.New(fiberzap.Config{
		Logger: fileLogger,                                                    // Use the base logger for output destination
		Fields: []string{"status", "method", "url", "ip", "latency", "error"}, // Keep desired standard fields
		// Use FieldsFunc to add dynamic fields per request
		FieldsFunc: func(c *fiber.Ctx) []zap.Field {
			fields := []zap.Field{
				zap.String("log_type", "access"), // Add a static field here if desired
			}
			// --- MODIFIED: Prioritize getting request_id from Locals ---
			reqID := ""
			if idVal := c.Locals(middleware.RequestIDKey); idVal != nil {
				if idStr, ok := idVal.(string); ok {
					reqID = idStr
				}
			}
			// Fallback to header if not found in locals (optional, locals should work)
			if reqID == "" {
				reqID = c.Get(middleware.RequestIDHeader)
			}
			// --- End Modification ---

			if reqID != "" {
				fields = append(fields, zap.String("request_id", reqID))
			}
			// Add any other dynamic fields needed from fiber.Ctx 'c' here
			return fields
		},
		Next: func(c *fiber.Ctx) bool {
			// Skip logging for health checks or static assets if desired
			return c.Path() == "/health" || strings.HasPrefix(c.Path(), "/uploads")
		},
	}))

	// --- 10. Setup Routes ---
	// Routes are registered after all general middleware
	routes.SetupRoutes(app, cfg, fileLogger, components, sqliteDB, oracleDB)

	// --- 11. Log Custom Startup Banner --- **REMOVED**
	// logCustomStartupBanner(fileLogger, app, cfg.Port) // REMOVED CALL

	// --- 12. Start Log Processor ---
	if logProcessor != nil {
		logProcessor.Start()
	}

	// --- 13. Start Server & Graceful Shutdown ---
	serverCtx, cancelServerCtx := context.WithCancel(context.Background())
	defer cancelServerCtx()
	serverStopped := make(chan struct{})
	go func() {
		defer close(serverStopped)
		listenAddr := ":" + cfg.Port
		// Log server start message here instead of banner
		fileLogger.Info("Starting server",
			zap.String("address", listenAddr),
			zap.Bool("prefork", app.Config().Prefork), // Access prefork setting from app config
			zap.Int("pid", os.Getpid()),
		)
		if err := app.Listen(listenAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fileLogger.Error("Server listener failed", zap.String("address", listenAddr), zap.Error(err))
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
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 60*time.Second) // Try 60 seconds, adjust as needed
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
		if err := sqliteDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing SQLite database: %v\n", err)
		} else {
			fmt.Println("[INFO] SQLite database connection closed.")
		}
	}
	if oracleDB != nil {
		if err := oracleDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing Oracle database pool: %v\n", err)
		} else {
			fmt.Println("[INFO] Oracle database pool closed.")
		}
	}

	fmt.Println("[INFO] Application shutdown complete.")
}

// logLoadedConfigDetails function remains the same
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

// maskOracleConnString function remains the same
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
