package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap" // Use logger for loading errors
)

// Config holds all configuration for the application
type Config struct {
	AppEnv                              string
	Port                                string
	CORSAllowOrigins                    string
	CORSAllowMethods                    string
	CORSAllowHeaders                    string
	JWTSecret                           string
	OracleConnString                    string
	OracleMaxPoolOpenConns              int // Max open connections
	OracleMaxPoolIdleConns              int // Max idle connections
	OracleMaxPoolConnLifetimeMinutes    int // Max lifetime in minutes
	OracleMaxPoolConnIdleTimeMinutes    int // Max idle time in minutes
	SQLiteDBPath                        string
	LogFilePath                         string
	LogLevel                            string
	LogRotateInterval                   int // Hour
	LogMaxSize                          int // MB
	LogMaxBackups                       int
	LogMaxAge                           int // Days
	LogCompress                         bool
	LogBatchInterval                    time.Duration
	LogProcessorBatchSize               int // Number of logs per batch transfer
	LogProcessorOracleRetryAttempts     int // Max retries for Oracle insert on connection error
	LogProcessorOracleRetryDelaySeconds int // Delay between retries in seconds
	UploadDir                           string
}

// LoadConfig reads configuration from environment variables or .env file
func LoadConfig(logger *zap.Logger) (*Config, error) { // logger can be nil here
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "local" // Default to local if not set
	}

	envFileName := fmt.Sprintf(".env.%s", appEnv)
	if _, err := os.Stat(envFileName); err == nil {
		if err := godotenv.Load(envFileName); err != nil {
			// FIX: Add nil check
			if logger != nil {
				logger.Warn("Error loading .env file, continuing with environment variables", zap.String("file", envFileName), zap.Error(err))
			}
		} else {
			// FIX: Add nil check
			if logger != nil {
				logger.Info("Loaded configuration", zap.String("file", envFileName))
			}
		}
	} else if appEnv == "local" {
		// Try loading .env.local by default if .env.local specifically exists
		if _, err := os.Stat(".env.local"); err == nil {
			if err := godotenv.Load(".env.local"); err != nil {
				// FIX: Add nil check
				if logger != nil {
					logger.Warn("Error loading .env.local file", zap.Error(err))
				}
			} else {
				// FIX: Add nil check
				if logger != nil {
					logger.Info("Loaded configuration from .env.local")
				}
			}
		} else {
			// FIX: Add nil check
			if logger != nil {
				logger.Warn(".env.local not found, relying on environment variables or defaults")
			}
		}
	} else {
		// FIX: Add nil check
		if logger != nil {
			logger.Warn("No specific .env file found for environment, relying on environment variables or defaults", zap.String("environment", appEnv))
		}
	}

	cfg := &Config{
		AppEnv:    getEnv("APP_ENV", "local"),
		Port:      getEnv("PORT", "3000"),
		JWTSecret: getEnv("JWT_SECRET", "default-secret"),
		// --- Load Oracle Settings ---
		OracleConnString:                 getEnv("ORACLE_CONN_STRING", ""),
		OracleMaxPoolOpenConns:           getEnvAsInt("ORACLE_MAX_POOL_OPEN_CONNS", 20),             // Default 20
		OracleMaxPoolIdleConns:           getEnvAsInt("ORACLE_MAX_POOL_IDLE_CONNS", 5),              // Default 5
		OracleMaxPoolConnLifetimeMinutes: getEnvAsInt("ORACLE_MAX_POOL_CONN_LIFETIME_MINUTES", 60),  // Default 60 (1 hour)
		OracleMaxPoolConnIdleTimeMinutes: getEnvAsInt("ORACLE_MAX_POOL_CONN_IDLE_TIME_MINUTES", 10), // Default 10
		// --- End Load Oracle Settings ---
		SQLiteDBPath:      getEnv("SQLITE_DB_PATH", "./logs/logs.db"),
		LogFilePath:       getEnv("LOG_FILE_PATH", "./logs/app.log"),
		LogLevel:          strings.ToLower(getEnv("LOG_LEVEL", "info")),
		LogRotateInterval: getEnvAsInt("LOG_ROTATE_INTERVAL", 1),
		LogMaxSize:        getEnvAsInt("LOG_MAX_SIZE", 100),
		LogMaxBackups:     getEnvAsInt("LOG_MAX_BACKUPS", 5),
		LogMaxAge:         getEnvAsInt("LOG_MAX_AGE", 30),
		LogCompress:       getEnvAsBool("LOG_COMPRESS", false),
		UploadDir:         getEnv("UPLOAD_DIR", "./uploads"),

		// --- Load CORS Settings ---
		// Default AllowOrigins to "*" for local, empty for others (forcing explicit setting)
		CORSAllowOrigins: getEnv("CORS_ALLOW_ORIGINS", func() string {
			if getEnv("APP_ENV", "local") == "local" || getEnv("APP_ENV", "local") == "development" {
				return "*" // Be permissive in local/dev
			}
			return "" // Force setting in prod/other envs
		}()),
		CORSAllowMethods: getEnv("CORS_ALLOW_METHODS", "GET,POST,HEAD,PUT,DELETE,PATCH"),
		CORSAllowHeaders: getEnv("CORS_ALLOW_HEADERS", "Origin,Content-Type,Accept,Authorization"),
		// --- End Load CORS ---

		// --- Load Log Processor Settings ---
		LogProcessorBatchSize:               getEnvAsInt("LOG_PROCESSOR_BATCH_SIZE", 100),
		LogProcessorOracleRetryAttempts:     getEnvAsInt("LOG_PROCESSOR_ORACLE_RETRY_ATTEMPTS", 3),
		LogProcessorOracleRetryDelaySeconds: getEnvAsInt("LOG_PROCESSOR_ORACLE_RETRY_DELAY_SECONDS", 30),
		// --- End Load Log Processor ---
	}
	// Validation for LogLevel string here if desired
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true, "dpanic": true, "panic": true, "fatal": true}
	if !validLevels[cfg.LogLevel] {
		if logger != nil {
			logger.Warn("Invalid LOG_LEVEL specified, defaulting to 'info'", zap.String("invalidLevel", cfg.LogLevel))
		}
		cfg.LogLevel = "info" // Reset to default if invalid
	}

	batchIntervalSec := getEnvAsInt("LOG_BATCH_INTERVAL_SECONDS", 60)
	cfg.LogBatchInterval = time.Duration(batchIntervalSec) * time.Second

	// Check required fields and add warnings/errors
	if cfg.OracleConnString == "" {
		// FIX: Add nil check
		if logger != nil {
			logger.Error("ORACLE_CONN_STRING environment variable is not set")
		}
		// Still return error regardless of logger
		return nil, fmt.Errorf("ORACLE_CONN_STRING is required")
	}
	if cfg.JWTSecret == "default-secret" {
		// FIX: Add nil check
		if logger != nil {
			logger.Warn("JWT_SECRET is using the default value. Please set a strong secret in production.")
		}
	}
	// Add warning for default/empty CORS origins in production
	if cfg.AppEnv != "local" && cfg.AppEnv != "development" && (cfg.CORSAllowOrigins == "*" || cfg.CORSAllowOrigins == "") {
		if logger != nil {
			logger.Warn("CORS_ALLOW_ORIGINS is set to '*' or is empty in a non-local/dev environment. This is insecure. Set specific origins for production.")
		}
		// Consider returning an error if empty in production?
		return nil, fmt.Errorf("CORS_ALLOW_ORIGINS must be set explicitly in production environments")
	}

	// Create upload directory if it doesnt exist
	if _, err := os.Stat(cfg.UploadDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.UploadDir, 0755); err != nil {
			// FIX: Add nil check
			if logger != nil {
				logger.Error("Failed to create upload directory", zap.String("path", cfg.UploadDir), zap.Error(err))
			}
			// Still return error regardless of logger
			return nil, fmt.Errorf("failed to create upload directory %s: %w", cfg.UploadDir, err)
		}
		// FIX: Add nil check
		if logger != nil {
			logger.Info("Created upload directory", zap.String("path", cfg.UploadDir))
		}
	}

	return cfg, nil
}

// Helper function to get env var or default
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// Helper function to get env var as int or default
func getEnvAsInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}

// Helper function to get env var as bool or default
func getEnvAsBool(key string, fallback bool) bool {
	valueStr := getEnv(key, "")
	if value, err := strconv.ParseBool(valueStr); err == nil {
		return value
	}
	return fallback
}
