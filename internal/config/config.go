package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap" // Use logger for loading errors
)

// Config holds all configuration for the application
type Config struct {
	AppEnv           string
	Port             string
	JWTSecret        string
	OracleConnString string
	SQLiteDBPath     string
	LogFilePath      string
	LogMaxSize       int // MB
	LogMaxBackups    int
	LogMaxAge        int // Days
	LogCompress      bool
	LogBatchInterval time.Duration
	UploadDir        string
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
		AppEnv:           getEnv("APP_ENV", "local"),
		Port:             getEnv("PORT", "3000"),
		JWTSecret:        getEnv("JWT_SECRET", "default-secret"),
		OracleConnString: getEnv("ORACLE_CONN_STRING", ""),
		SQLiteDBPath:     getEnv("SQLITE_DB_PATH", "./logs.db"),
		LogFilePath:      getEnv("LOG_FILE_PATH", "./app.log"),
		LogMaxSize:       getEnvAsInt("LOG_MAX_SIZE", 100),
		LogMaxBackups:    getEnvAsInt("LOG_MAX_BACKUPS", 5),
		LogMaxAge:        getEnvAsInt("LOG_MAX_AGE", 30),
		LogCompress:      getEnvAsBool("LOG_COMPRESS", false),
		UploadDir:        getEnv("UPLOAD_DIR", "./uploads"),
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
