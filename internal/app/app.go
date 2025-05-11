package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go-webapi/internal/repositories"
	"go-webapi/internal/utils"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go-webapi/internal/bootstrap"
	"go-webapi/internal/config"
	"go-webapi/internal/database"
	"go-webapi/internal/logging"
	"go-webapi/internal/middleware"
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
	var fileLogger *zap.Logger
	var sqliteLogger *zap.Logger
	var oracleDB *sql.DB
	var sqliteDB *sql.DB
	var cfg *config.Config
	var err error
	var appFiber *fiber.App
	var components *bootstrap.AppComponents
	var fileSyncer zapcore.WriteSyncer
	var logRepo repositories.LogRepository

	// <<<< Record start time for App initialization
	initAppStartTime := time.Now()

	// --- 1. Load Configuration ---
	tempConfigLogger, _ := zap.NewProduction(zap.ErrorOutput(zapcore.Lock(os.Stderr)))
	defer tempConfigLogger.Sync()

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
	fmt.Fprintf(os.Stderr, "[INFO] Shared file syncer created for path: %s with MaxSize: %d MB, RotateInterval: %d hours\n", cfg.LogFilePath, cfg.LogMaxSize, cfg.LogRotateInterval)

	// --- 3. Initialize LogRepository (with nil DBs and temp logger initially) ---
	// LogRepository methods (InsertSQLiteLog) must be nil-safe for DB handles during this early phase.
	logRepo = repositories.NewLogRepository(nil, nil, tempConfigLogger)
	tempConfigLogger.Info("LogRepository initially created (DB handles are nil, using temp logger).")

	// --- 4. Initialize Main Application Loggers (File/Console and SQLite-dedicated) ---
	// InitializeLoggers uses logRepo. If sqliteLogger writes via logRepo here,
	// InsertSQLiteLog in logRepo will see sqliteDB as nil and should handle it gracefully (e.g., log error to stderr).
	appLoggers, err := logging.InitializeLoggers(cfg, logRepo, fileSyncer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize application loggers: %v\n", err)
		os.Exit(1)
	}
	fileLogger = appLoggers.File
	sqliteLogger = appLoggers.SQLite

	// --- 5. Set Global Loggers ---
	logging.SetGlobalLoggers(fileLogger, sqliteLogger)
	fileLogger.Info("Global application loggers (file/console and SQLite-dedicated) have been set.")
	// Update the logger instance within logRepo to use the final fileLogger
	if lrWithSetter, ok := logRepo.(interface{ SetLogger(*zap.Logger) }); ok {
		lrWithSetter.SetLogger(fileLogger) // logRepo will now use final fileLogger for its own messages
		fileLogger.Info("Final fileLogger set for LogRepository's internal logging.")
	}

	// --- 6. Trace Config Details (using the final fileLogger) ---
	utils.TraceConfigDetails(fileLogger, cfg)

	// --- 7. Initialize SQLite Database (using final fileLogger) ---
	// This is now after fileLogger is ready.
	sqliteDB, err = database.InitSQLite(cfg, fileLogger)
	if err != nil {
		fileLogger.Fatal("Failed to initialize SQLite database", zap.Error(err))
	}
	fileLogger.Info("SQLite database initialized successfully (using final fileLogger).")

	// Update LogRepository with the initialized SQLiteDB
	if lrWithSetter, ok := logRepo.(interface{ SetSqliteDB(*sql.DB) }); ok {
		lrWithSetter.SetSqliteDB(sqliteDB)
		fileLogger.Info("SQLiteDB handle has been set in LogRepository. sqliteLogger should now function fully.")
	} else {
		fileLogger.Error("LogRepository does not implement SetSqliteDB. sqliteLogger might not work.")
	}

	// --- 8. Initialize Oracle Database (using final fileLogger) ---
	// This was already after fileLogger in the previous step.
	oracleDB, err = database.InitOracle(cfg, fileLogger)
	if err != nil {
		fileLogger.Error("Error during Oracle DB pool initialization. Operations requiring Oracle may fail.", zap.Error(err))
	} else if oracleDB != nil {
		fileLogger.Info("Oracle database pool initialized successfully (using final fileLogger).")
		// Update LogRepository with the initialized OracleDB
		if lrWithSetter, ok := logRepo.(interface{ SetOracleDB(*sql.DB) }); ok {
			lrWithSetter.SetOracleDB(oracleDB)
			fileLogger.Info("OracleDB handle has been set in LogRepository.")
		} else {
			fileLogger.Error("LogRepository does not implement SetOracleDB. Oracle part of logRepo might not work.")
		}
	}

	// --- 9. Initialize Fiber App ---
	fileLogger.Info("Initializing Fiber application...")
	appFiber = fiber.New(fiber.Config{
		AppName: cfg.AppName,
		Prefork: cfg.Prefork,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			lg := middleware.GetRequestFileLogger(c)
			code := fiber.StatusInternalServerError
			var e *fiber.Error
			if errors.As(err, &e) && e != nil {
				code = e.Code
			}
			if c == nil {
				fmt.Println("FATAL: fiber.Ctx is nil in ErrorHandler")
				return errors.New("internal context error")
			}
			fields := []zap.Field{
				zap.Int("status", code),
				zap.String("path", c.Path()),
				zap.String("method", c.Method()),
				zap.String("ip", c.IP()),
				zap.Error(err),
			}
			if reqIDStr, ok := c.Locals(middleware.RequestIDKey).(string); ok && reqIDStr != "" {
				fields = append(fields, zap.String("request_id", reqIDStr))
			}
			if code == fiber.StatusNotFound {
				lg.Warn("Resource not found", fields...)
			} else {
				lg.Error("Generic ErrorHandler", fields...)
			}
			resp := fiber.Map{"error": "An unexpected error occurred"}
			if cfg != nil && cfg.AppEnv != "production" {
				if err != nil {
					resp["detail"] = err.Error()
				} else {
					resp["detail"] = "Error object was nil"
				}
			}
			return c.Status(code).JSON(resp)
		},
	})

	// --- 10. Initialize Remaining Application Components (Bootstrap) ---
	components, err = bootstrap.InitializeAppComponents(cfg, fileLogger, sqliteLogger, sqliteDB, oracleDB, logRepo)
	if err != nil {
		fileLogger.Fatal("Failed to initialize application components", zap.Error(err))
	}
	// logProcessor instance is obtained from components. Ensure bootstrap wires it up correctly.
	// var logProcessor *logging.LogProcessor // local var if needed, or use components.LogProcessor directly

	// --- 11. Register Middleware ---
	appFiber.Use(recover.New(recover.Config{
		EnableStackTrace: strings.ToLower(cfg.LogLevel) == "debug",
		StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
			logger := middleware.GetRequestFileLogger(c)
			if logger == nil {
				logger = logging.GetFileLogger()
			}
			logger.Error("Panic recovered", zap.Any("panic_value", e))
		},
	}))
	fileLogger.Info("Configuring CORS", zap.String("origins", cfg.CORSAllowOrigins), zap.String("methods", cfg.CORSAllowMethods), zap.String("headers", cfg.CORSAllowHeaders))
	appFiber.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSAllowOrigins,
		AllowMethods: cfg.CORSAllowMethods,
		AllowHeaders: cfg.CORSAllowHeaders,
	}))
	appFiber.Use(middleware.RequestLoggers(fileLogger, sqliteLogger))
	if strings.ToLower(cfg.LogLevel) == "debug" {
		appFiber.Use(middleware.RequestDebugLogger())
	}
	appFiber.Use(fiberzap.New(fiberzap.Config{
		Logger: fileLogger,
		Fields: []string{"status", "method", "url", "ip", "latency", "error"}, // Keep desired standard fields
		FieldsFunc: func(c *fiber.Ctx) []zap.Field {
			fields := []zap.Field{zap.String("log_type", "access")}
			reqID := ""
			if idVal := c.Locals(middleware.RequestIDKey); idVal != nil {
				if idStr, ok := idVal.(string); ok {
					reqID = idStr
				}
			}
			if reqID == "" {
				reqID = c.Get(middleware.RequestIDHeader)
			}
			if reqID != "" {
				fields = append(fields, zap.String("request_id", reqID))
			}
			return fields
		},
		Next: func(c *fiber.Ctx) bool {
			return c.Path() == "/health" || strings.HasPrefix(c.Path(), "/uploads")
		},
	}))

	// --- 12. Setup Application Routes ---
	routes.SetupRoutes(appFiber, cfg, fileLogger, components, sqliteDB, oracleDB)

	// --- 13. Start Log Processor (conditionally if not a forked child) ---
	if components != nil && components.LogProcessor != nil {
		// When Prefork is true, appFiber.IsChild() will be true in child processes.
		// We only want the LogProcessor to run in the master process.
		if !fiber.IsChild() {
			fileLogger.Info("Master process starting LogProcessor...", zap.Int("pid", os.Getpid()))
			components.LogProcessor.Start()
		} else {
			fileLogger.Info("Child process will not start its own LogProcessor instance.", zap.Int("pid", os.Getpid()))
		}
	} else {
		fileLogger.Warn("LogProcessor is nil or components are nil, cannot start it.")
	}

	// --- 14. Start Server & Graceful Shutdown ---
	serverCtx, cancelServerCtx := context.WithCancel(context.Background())
	defer cancelServerCtx()
	serverStopped := make(chan struct{})

	// <<<< Calculate initialization duration >>>>
	initAppDurationMs := time.Since(initAppStartTime).Milliseconds()

	go func() {
		defer close(serverStopped)
		listenAddr := ":" + cfg.Port
		fileLogger.Info(fmt.Sprintf("Completed initialization application in %d ms.", initAppDurationMs))
		fileLogger.Info("Starting Fiber server...",
			zap.String("address", listenAddr),
			zap.Bool("prefork_enabled", appFiber.Config().Prefork),
			zap.Int("pid", os.Getpid()),
			zap.String("app_env", cfg.AppEnv),
		)

		if err := appFiber.Listen(listenAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelShutdown()

	// Stop the LogProcessor (only if master process and processor exists)
	if components != nil && components.LogProcessor != nil && !fiber.IsChild() {
		fileLogger.Info("Master process stopping LogProcessor...", zap.Int("pid", os.Getpid()))
		components.LogProcessor.Stop()
	}

	if err := appFiber.ShutdownWithContext(shutdownCtx); err != nil {
		fileLogger.Error("Fiber server shutdown failed", zap.Error(err))
	} else {
		fileLogger.Info("Fiber server gracefully stopped.")
	}
	<-serverStopped
	fileLogger.Info("HTTP listener goroutine stopped.")

	fileLogger.Info("Syncing file/console logger before shutdown...")
	if errSync := fileLogger.Sync(); errSync != nil {
		errMsg := errSync.Error()
		if strings.Contains(errMsg, "handle is invalid") || strings.Contains(errMsg, "sync /dev/stdout") {
			// Log as debug or info, as this is often expected when stdout isn't available at exit
			fileLogger.Debug("Logger sync warning for stdout (handle likely invalid during shutdown).", zap.Error(errSync))
		} else {
			// Log other sync errors as warnings or errors
			fileLogger.Warn("Error syncing file/console logger.", zap.Error(errSync))
			// Use fmt.Fprintf as a fallback if the logger itself might be failing
			fmt.Fprintf(os.Stderr, "[WARN] Error syncing file/console logger: %v\n", errSync)
		}
	}
	fmt.Println("[INFO] Logger sync attempts completed.")

	if sqliteDB != nil {
		if errClose := sqliteDB.Close(); errClose != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing SQLite database: %v\n", errClose)
		} else {
			fmt.Println("[INFO] SQLite database connection closed.")
		}
	}
	if oracleDB != nil {
		if errClose := oracleDB.Close(); errClose != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Error closing Oracle database pool: %v\n", errClose)
		} else {
			fmt.Println("[INFO] Oracle database pool closed.")
		}
	}

	fmt.Println("[INFO] Application shutdown complete.")
}
