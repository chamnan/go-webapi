#!/bin/bash

# Define color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# --- Configuration ---
DEFAULT_PROJECT_NAME="myfiberapp"

# Prompt for PROJECT_NAME with a default value
read -p "Enter project name [${DEFAULT_PROJECT_NAME}]: " PROJECT_NAME
PROJECT_NAME=${PROJECT_NAME:-$DEFAULT_PROJECT_NAME}

# Check if the folder already exists
if [ -d "$PROJECT_NAME" ]; then
	echo -e "${RED}Error: The directory '$PROJECT_NAME' already exists.${NC}"
	exit 1
fi

GO_VERSION="1.24.2"
# Default environment variables (will be written to .env.local)
DEFAULT_APP_ENV="local"
DEFAULT_PORT="3000"
DEFAULT_JWT_SECRET="your-very-secret-key-change-me"
DEFAULT_ORACLE_CONN_STRING="oracle://user:password@host:port/service_name" # Replace with placeholder or actual local dev string
DEFAULT_SQLITE_DB_PATH="./logs.db"
DEFAULT_LOG_FILE_PATH="./app.log"
DEFAULT_LOG_MAX_SIZE="100" # MB
DEFAULT_LOG_MAX_BACKUPS="5"
DEFAULT_LOG_MAX_AGE="30" # Days
DEFAULT_LOG_COMPRESS="false"
DEFAULT_LOG_BATCH_INTERVAL_SECONDS="60"
DEFAULT_UPLOAD_DIR="./uploads"

# --- Helper Functions ---
create_file() {
  mkdir -p "$(dirname "$1")"
  touch "$1"
  echo -e "${YELLOW}Created:${NC} $1"
}

write_to_file() {
  mkdir -p "$(dirname "$1")"
  cat << EOF > "$1"
$2
EOF
  echo -e "${BLUE}Written:${NC} $1"
}


# --- Directory Structure ---
echo -e "${GREEN}Creating directory structure for $PROJECT_NAME...${NC}"
mkdir -p "$PROJECT_NAME"
mkdir -p "$PROJECT_NAME/cmd/api"
mkdir -p "$PROJECT_NAME/internal/app"
mkdir -p "$PROJECT_NAME/internal/config"
mkdir -p "$PROJECT_NAME/internal/database"
mkdir -p "$PROJECT_NAME/internal/handlers"
mkdir -p "$PROJECT_NAME/internal/middleware"
mkdir -p "$PROJECT_NAME/internal/models"
mkdir -p "$PROJECT_NAME/internal/repositories"
mkdir -p "$PROJECT_NAME/internal/services"
mkdir -p "$PROJECT_NAME/internal/logging"
mkdir -p "$PROJECT_NAME/internal/utils"
mkdir -p "$PROJECT_NAME/scripts"
mkdir -p "$PROJECT_NAME/uploads"

# --- Create Empty Files (or with minimal content) ---
echo "Creating initial files..."
create_file "$PROJECT_NAME/cmd/api/main.go"
create_file "$PROJECT_NAME/internal/app/app.go"
create_file "$PROJECT_NAME/internal/config/config.go"
create_file "$PROJECT_NAME/internal/database/oracle.go"
create_file "$PROJECT_NAME/internal/database/sqlite.go"
create_file "$PROJECT_NAME/internal/handlers/auth_handler.go"
create_file "$PROJECT_NAME/internal/handlers/profile_handler.go"
create_file "$PROJECT_NAME/internal/middleware/jwt_middleware.go"
create_file "$PROJECT_NAME/internal/models/user.go"
create_file "$PROJECT_NAME/internal/models/log.go"
create_file "$PROJECT_NAME/internal/repositories/user_repo.go"
create_file "$PROJECT_NAME/internal/repositories/log_repo.go"
create_file "$PROJECT_NAME/internal/services/auth_service.go"
create_file "$PROJECT_NAME/internal/services/profile_service.go"
create_file "$PROJECT_NAME/internal/logging/logger.go"
create_file "$PROJECT_NAME/internal/logging/processor.go"
create_file "$PROJECT_NAME/internal/utils/password.go"
create_file "$PROJECT_NAME/internal/utils/jwt.go"
create_file "$PROJECT_NAME/uploads/.gitkeep"
create_file "$PROJECT_NAME/.env.local"
create_file "$PROJECT_NAME/.env.prod"
create_file "$PROJECT_NAME/go.mod"
create_file "$PROJECT_NAME/.gitignore"
cp "$0" "$PROJECT_NAME/scripts/generate_project.sh" # Copy this script itself

# --- Populate Files ---

# .gitignore
GITIGNORE_CONTENT='
# Binaries for programs and plugins
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary, build with `go test -c`
*.test

# Output of the go coverage tool, specifically when used with LiteIDE
*.out

# Dependency directories (remove the comment below to include it)
# vendor/

# Go workspace file
go.work

# Env files
.env
.env.*
!/.env.example

# Log files
*.log
logs/
*.log.*

# SQLite database
*.db
*.db-journal

# Uploads directory
uploads/*
!uploads/.gitkeep

# IDE settings folders
.idea/
.vscode/

# OS generated files
.DS_Store
Thumbs.db
'
write_to_file "$PROJECT_NAME/.gitignore" "$GITIGNORE_CONTENT"

# go.mod
GOMOD_CONTENT="module $PROJECT_NAME

go $GO_VERSION

require (
	github.com/godror/godror v0.48.1 // Or latest stable Oracle driver
	github.com/gofiber/contrib/fiberzap/v2 v2.1.6
	github.com/gofiber/fiber/v2 v2.52.6 // Use specific version or latest stable
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/mattn/go-sqlite3 v1.14.28 // Or latest stable SQLite driver
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.37.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/godror/knownpb v0.2.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/planetscale/vtprotobuf v0.6.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.61.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)
"
write_to_file "$PROJECT_NAME/go.mod" "$GOMOD_CONTENT"


# .env.local
ENV_LOCAL_CONTENT="
APP_ENV=$DEFAULT_APP_ENV
PORT=$DEFAULT_PORT
JWT_SECRET=$DEFAULT_JWT_SECRET
ORACLE_CONN_STRING=\"$DEFAULT_ORACLE_CONN_STRING\" # Ensure quotes if string has special chars
SQLITE_DB_PATH=$DEFAULT_SQLITE_DB_PATH
LOG_FILE_PATH=$DEFAULT_LOG_FILE_PATH
LOG_MAX_SIZE=$DEFAULT_LOG_MAX_SIZE
LOG_MAX_BACKUPS=$DEFAULT_LOG_MAX_BACKUPS
LOG_MAX_AGE=$DEFAULT_LOG_MAX_AGE
LOG_COMPRESS=$DEFAULT_LOG_COMPRESS
LOG_BATCH_INTERVAL_SECONDS=$DEFAULT_LOG_BATCH_INTERVAL_SECONDS
UPLOAD_DIR=$DEFAULT_UPLOAD_DIR
"
write_to_file "$PROJECT_NAME/.env.local" "$ENV_LOCAL_CONTENT"

# .env.prod (Example - fill with actual production values)
ENV_PROD_CONTENT="
APP_ENV=production
PORT=8080 # Or standard prod port
JWT_SECRET=a-much-stronger-production-secret-key # Use env var or secrets manager in real prod
ORACLE_CONN_STRING=\"oracle://prod_user:prod_password@prod_host:1521/prod_service\" # Use env var or secrets manager
SQLITE_DB_PATH=/var/log/$PROJECT_NAME/logs.db # Or appropriate path
LOG_FILE_PATH=/var/log/$PROJECT_NAME/app.log # Or appropriate path
LOG_MAX_SIZE=200 # Example prod value
LOG_MAX_BACKUPS=10
LOG_MAX_AGE=60
LOG_COMPRESS=true
LOG_BATCH_INTERVAL_SECONDS=120 # Example prod value (longer interval)
UPLOAD_DIR=/var/www/uploads/$PROJECT_NAME # Or appropriate path
"
write_to_file "$PROJECT_NAME/.env.prod" "$ENV_PROD_CONTENT"


# internal/config/config.go
CONFIG_GO_CONTENT='
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
	AppEnv                 string
	Port                   string
	JWTSecret              string
	OracleConnString       string
	SQLiteDBPath           string
	LogFilePath            string
	LogMaxSize             int // MB
	LogMaxBackups          int
	LogMaxAge              int // Days
	LogCompress            bool
	LogBatchInterval       time.Duration
	UploadDir              string
}

// LoadConfig reads configuration from environment variables or .env file
func LoadConfig(logger *zap.Logger) (*Config, error) {
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "local" // Default to local if not set
	}

	envFileName := fmt.Sprintf(".env.%s", appEnv)
	if _, err := os.Stat(envFileName); err == nil {
		if err := godotenv.Load(envFileName); err != nil {
			logger.Warn("Error loading .env file, continuing with environment variables", zap.String("file", envFileName), zap.Error(err))
		} else {
			logger.Info("Loaded configuration", zap.String("file", envFileName))
		}
	} else if appEnv == "local" {
		// Try loading .env.local by default if .env.local specifically exists
		if _, err := os.Stat(".env.local"); err == nil {
			if err := godotenv.Load(".env.local"); err != nil {
                logger.Warn("Error loading .env.local file", zap.Error(err))
            } else {
                logger.Info("Loaded configuration from .env.local")
            }
		} else {
             logger.Warn(".env.local not found, relying on environment variables or defaults")
        }
	} else {
         logger.Warn("No specific .env file found for environment, relying on environment variables or defaults", zap.String("environment", appEnv))
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

	if cfg.OracleConnString == "" {
		logger.Error("ORACLE_CONN_STRING environment variable is not set")
		return nil, fmt.Errorf("ORACLE_CONN_STRING is required")
	}
    if cfg.JWTSecret == "default-secret" {
        logger.Warn("JWT_SECRET is using the default value. Please set a strong secret in production.")
    }

	// Create upload directory if it doesnt exist
	if _, err := os.Stat(cfg.UploadDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.UploadDir, 0755); err != nil {
			logger.Error("Failed to create upload directory", zap.String("path", cfg.UploadDir), zap.Error(err))
			return nil, fmt.Errorf("failed to create upload directory %s: %w", cfg.UploadDir, err)
		}
		logger.Info("Created upload directory", zap.String("path", cfg.UploadDir))
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

'
write_to_file "$PROJECT_NAME/internal/config/config.go" "$CONFIG_GO_CONTENT"

# internal/database/sqlite.go
SQLITE_GO_CONTENT='
package database

import (
	"database/sql"
	"fmt"
	"time"
	_ "github.com/mattn/go-sqlite3" // SQLite Driver
	"go.uber.org/zap"
	"'$PROJECT_NAME'/internal/config"
)

const createLogTableSQL = `
CREATE TABLE IF NOT EXISTS tbl_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    fields TEXT -- Store additional zap fields as JSON string
);
`

// InitSQLite initializes the SQLite database connection and ensures the log table exists.
func InitSQLite(cfg *config.Config, logger *zap.Logger) (*sql.DB, error) {
	logger.Info("Initializing SQLite database...", zap.String("path", cfg.SQLiteDBPath))

	db, err := sql.Open("sqlite3", cfg.SQLiteDBPath+"?_journal_mode=WAL&_busy_timeout=5000") // WAL mode is generally good for concurrent reads/writes
	if err != nil {
		logger.Error("Failed to open SQLite database", zap.Error(err))
		return nil, fmt.Errorf("failed to open sqlite database at %s: %w", cfg.SQLiteDBPath, err)
	}

	// Configure connection pool (optional but good practice)
	db.SetMaxOpenConns(1) // Usually 1 is sufficient for logging writes
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close() // Close if ping fails
		logger.Error("Failed to ping SQLite database", zap.Error(err))
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// Ensure log table exists
	if _, err := db.Exec(createLogTableSQL); err != nil {
		db.Close() // Close if table creation fails
		logger.Error("Failed to create tbl_log in SQLite", zap.Error(err))
		return nil, fmt.Errorf("failed to create sqlite table tbl_log: %w", err)
	}

	logger.Info("SQLite database initialized successfully", zap.String("path", cfg.SQLiteDBPath))
	return db, nil
}
'
write_to_file "$PROJECT_NAME/internal/database/sqlite.go" "$SQLITE_GO_CONTENT"

# internal/database/oracle.go
ORACLE_GO_CONTENT='
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/godror/godror" // Oracle Driver
	"go.uber.org/zap"
	"'$PROJECT_NAME'/internal/config"
)

const (
	maxReconnectAttempts = 5
	reconnectDelay       = 5 * time.Second
)

// InitOracle initializes the Oracle database connection with auto-reconnect logic.
func InitOracle(cfg *config.Config, logger *zap.Logger) (*sql.DB, error) {
	logger.Info("Initializing Oracle database connection...")

	var db *sql.DB
	var err error
	var attempt int

	for attempt = 1; attempt <= maxReconnectAttempts; attempt++ {
		db, err = sql.Open("godror", cfg.OracleConnString)
		if err != nil {
			logger.Error("Failed to open Oracle connection", zap.Int("attempt", attempt), zap.Error(err))
			if attempt < maxReconnectAttempts {
				logger.Info("Retrying Oracle connection...", zap.Duration("delay", reconnectDelay))
				time.Sleep(reconnectDelay)
				continue
			}
			return nil, fmt.Errorf("failed to open oracle connection after %d attempts: %w", maxReconnectAttempts, err)
		}

		// Configure connection pool (adjust as needed)
		db.SetMaxOpenConns(20)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(time.Hour)
		db.SetConnMaxIdleTime(10 * time.Minute)


		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Ping timeout
		err = db.PingContext(ctx)
		cancel() // Release context resources

		if err == nil {
			logger.Info("Oracle database connection established successfully")
			// Start background goroutine to monitor and reconnect
			go monitorOracleConnection(db, cfg.OracleConnString, logger)
			return db, nil
		}

        // If ping failed, close the potentially problematic connection object before retrying
        db.Close()
		logger.Error("Failed to ping Oracle database", zap.Int("attempt", attempt), zap.Error(err))
		if attempt < maxReconnectAttempts {
			logger.Info("Retrying Oracle connection...", zap.Duration("delay", reconnectDelay))
			time.Sleep(reconnectDelay)
		}
	}

	return nil, fmt.Errorf("failed to connect to oracle database after %d attempts: %w", maxReconnectAttempts, err)
}

// monitorOracleConnection periodically pings the database and attempts to reconnect if necessary.
// Note: database/sql often handles connection pooling and retries internally for transient errors.
// This explicit monitoring is more for handling sustained connection loss (network outage, DB restart).
func monitorOracleConnection(db *sql.DB, connString string, logger *zap.Logger) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := db.PingContext(ctx)
		cancel()

		if err != nil {
			logger.Warn("Oracle connection lost, attempting to reconnect...", zap.Error(err))
			// The standard library'\''s pool *should* handle reopening, but we log the event.
            // In extremely rare cases where the pool itself gets stuck, more drastic measures
            // might be needed, but usually letting the pool manage itself is best.
            // For this example, we will rely on `database/sql` internal pool management.
            // You could add explicit `db.Close()` and `sql.Open()` here if you encounter issues
            // where the pool doesn'\''t recover automatically, but start without it.
            // Example of explicit reconnect (use cautiously):
            err := db.Close()
			if err != nil {
				return
			} // Close the potentially broken pool
            var reconnErr error
            for i := 1; i <= maxReconnectAttempts; i++ {
                newDb, reconnErr := sql.Open("godror", connString)
                if reconnErr == nil {
                    ctxReconn, cancelReconn := context.WithTimeout(context.Background(), 10*time.Second)
                    pingErr := newDb.PingContext(ctxReconn)
                    cancelReconn()
                    if pingErr == nil {
                        logger.Info("Successfully reconnected to Oracle.")
                        // How to replace the original '\''db'\'' pointer globally is tricky.
                        // This usually requires a more complex setup (e.g., passing a struct
                        // with a mutex-protected *sql.DB field).
                        // For simplicity here, we assume the pool handles it.
                        *db = *newDb // DANGEROUS without proper synchronization if db is shared directly
                        break
                    } else {
                         newDb.Close()
                         logger.Error("Ping failed after reconnect attempt", zap.Int("attempt", i), zap.Error(pingErr))
                    }
                } else {
                    logger.Error("Failed to reopen Oracle connection", zap.Int("attempt", i), zap.Error(reconnErr))
                }
                time.Sleep(reconnectDelay)
            }
            if reconnErr != nil {
                 logger.Error("Failed to reconnect to Oracle after multiple attempts.")
            }
		}else { 
			logger.Debug("Oracle connection healthy") 
		}
	}
}

'
write_to_file "$PROJECT_NAME/internal/database/oracle.go" "$ORACLE_GO_CONTENT"


# internal/models/user.go
USER_MODEL_GO_CONTENT='
package models

import "time"

// User represents the structure of the tbl_user table
type User struct {
	ID          int64     `json:"id"`              // Assuming ID is a number
	Username    string    `json:"username"`
	PasswordHash string   `json:"-"` // Exclude password hash from JSON responses
	PhotoPath   string    `json:"photo_path,omitempty"`
	CreatedAt   time.Time `json:"created_at"`      // Optional: Audit column
	UpdatedAt   time.Time `json:"updated_at"`      // Optional: Audit column
}
'
write_to_file "$PROJECT_NAME/internal/models/user.go" "$USER_MODEL_GO_CONTENT"

# internal/models/log.go
LOG_MODEL_GO_CONTENT='
package models

import "time"

// LogEntry represents a log record to be stored temporarily in SQLite
// and then transferred to Oracle tbl_log
type LogEntry struct {
	ID        int64     `json:"-"`                // SQLite Row ID
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Fields    string    `json:"fields,omitempty"` // JSON representation of extra fields
	// Add other fields matching your Oracle tbl_log structure if needed
	// e.g., RequestID, UserID, IPAddress, etc.
}

'
write_to_file "$PROJECT_NAME/internal/models/log.go" "$LOG_MODEL_GO_CONTENT"

# internal/utils/password.go
PASSWORD_UTIL_GO_CONTENT='
package utils

import "golang.org/x/crypto/bcrypt"

// HashPassword generates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash compares a plain text password with a stored bcrypt hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

'
write_to_file "$PROJECT_NAME/internal/utils/password.go" "$PASSWORD_UTIL_GO_CONTENT"

# internal/utils/jwt.go
JWT_UTIL_GO_CONTENT='
package utils

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims defines the structure of the JWT claims
type Claims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken generates a new JWT for a given user ID
func GenerateToken(userID int64, secret string, expiresAfter time.Duration) (string, error) {
	expirationTime := time.Now().Add(expiresAfter)
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "your-app-name", // Optional: Identify the issuer
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates a JWT string and returns the claims if valid
func ValidateToken(tokenString string, secret string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is what you expect
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

'
write_to_file "$PROJECT_NAME/internal/utils/jwt.go" "$JWT_UTIL_GO_CONTENT"


# internal/repositories/user_repo.go
USER_REPO_GO_CONTENT='
package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"'$PROJECT_NAME'/internal/models"
	"go.uber.org/zap"
)

// UserRepository defines the interface for user data operations
type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (*models.User, error)
	FindByID(ctx context.Context, id int64) (*models.User, error)
	CreateUser(ctx context.Context, user *models.User) (int64, error) // Returns the new user ID
	// Add other methods like UpdateUser, DeleteUser etc. if needed
}

// oracleUserRepository implements UserRepository for Oracle
type oracleUserRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewOracleUserRepository creates a new UserRepository for Oracle
func NewOracleUserRepository(db *sql.DB, logger *zap.Logger) UserRepository {
	return &oracleUserRepository{db: db, logger: logger}
}

// FindByUsername retrieves a user by their username from Oracle tbl_user
func (r *oracleUserRepository) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `SELECT user_id, username, password_hash, photo_path, created_at, updated_at FROM tbl_user WHERE username = :1` // Adjust column names as needed
	user := &models.User{}
	var createdAt sql.NullTime // Handle potential NULLs if columns allow
	var updatedAt sql.NullTime // Handle potential NULLs if columns allow
    var photoPath sql.NullString

	r.logger.Debug("Executing FindByUsername query", zap.String("query", query), zap.String("username", username)) // Optional Debug logging

	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
        &photoPath, // Scan into NullString first
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.logger.Warn("User not found by username", zap.String("username", username))
			return nil, nil // Return nil, nil to indicate not found cleanly
		}
		r.logger.Error("Error querying user by username", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("error finding user by username %s: %w", username, err)
	}

	// Assign values from Null types if they are valid
	if createdAt.Valid {
		user.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		user.UpdatedAt = updatedAt.Time
	}
    if photoPath.Valid {
        user.PhotoPath = photoPath.String
    }


	return user, nil
}

// FindByID retrieves a user by their ID from Oracle tbl_user
func (r *oracleUserRepository) FindByID(ctx context.Context, id int64) (*models.User, error) {
    // Similar implementation to FindByUsername, but query by user_id
	query := `SELECT user_id, username, password_hash, photo_path, created_at, updated_at FROM tbl_user WHERE user_id = :1`
	user := &models.User{}
    var createdAt sql.NullTime
	var updatedAt sql.NullTime
    var photoPath sql.NullString

    r.logger.Debug("Executing FindByID query", zap.String("query", query), zap.Int64("id", id))

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
        &photoPath,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.logger.Warn("User not found by ID", zap.Int64("id", id))
			return nil, nil // Not found
		}
		r.logger.Error("Error querying user by ID", zap.Int64("id", id), zap.Error(err))
		return nil, fmt.Errorf("error finding user by ID %d: %w", id, err)
	}

    if createdAt.Valid { user.CreatedAt = createdAt.Time }
	if updatedAt.Valid { user.UpdatedAt = updatedAt.Time }
    if photoPath.Valid { user.PhotoPath = photoPath.String }

	return user, nil
}

// CreateUser inserts a new user into the Oracle tbl_user
func (r *oracleUserRepository) CreateUser(ctx context.Context, user *models.User) (int64, error) {
	// Note: Oracle often uses sequences for IDs. Adjust INSERT accordingly.
	// This example assumes user_id is auto-generated or managed by a trigger/sequence.
	// We use RETURNING INTO clause to get the ID back.
	query := `
        INSERT INTO tbl_user (username, password_hash, photo_path, created_at, updated_at)
        VALUES (:1, :2, :3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        RETURNING user_id INTO :4` // Adjust column names and returning clause for your schema

    var newID int64
    photoPath := sql.NullString{String: user.PhotoPath, Valid: user.PhotoPath != ""}


	r.logger.Debug("Executing CreateUser query", zap.String("query", "INSERT INTO tbl_user..."), zap.String("username", user.Username))

	_, err := r.db.ExecContext(ctx, query,
		user.Username,
		user.PasswordHash,
		photoPath, // Pass NullString
        sql.Out{Dest: &newID}, // Oracle specific way to get returned value
	)
	if err != nil {
		// TODO: Handle potential unique constraint violations (e.g., duplicate username)
		r.logger.Error("Error creating user", zap.String("username", user.Username), zap.Error(err))
		return 0, fmt.Errorf("error creating user %s: %w", user.Username, err)
	}

    user.ID = newID // Set the ID on the passed-in user object
	r.logger.Info("User created successfully", zap.String("username", user.Username), zap.Int64("newID", newID))
	return newID, nil
}

'
write_to_file "$PROJECT_NAME/internal/repositories/user_repo.go" "$USER_REPO_GO_CONTENT"

# internal/repositories/log_repo.go
LOG_REPO_GO_CONTENT='
package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"'$PROJECT_NAME'/internal/models"
	"go.uber.org/zap"
)

// LogRepository defines the interface for log data operations
type LogRepository interface {
	// SQLite Operations
	InsertSQLiteLog(ctx context.Context, entry models.LogEntry) error
	GetSQLiteLogs(ctx context.Context, limit int) ([]models.LogEntry, error)
	DeleteSQLiteLogsByID(ctx context.Context, ids []int64) error
	// Oracle Operations
	InsertBatchOracle(ctx context.Context, logs []models.LogEntry) error
}

// logRepositoryImpl implements LogRepository for both SQLite and Oracle
type logRepositoryImpl struct {
	sqliteDB *sql.DB
	oracleDB *sql.DB
	logger   *zap.Logger
}

// NewLogRepository creates a new LogRepository
func NewLogRepository(sqliteDB *sql.DB, oracleDB *sql.DB, logger *zap.Logger) LogRepository {
	return &logRepositoryImpl{
		sqliteDB: sqliteDB,
		oracleDB: oracleDB,
		logger:   logger,
	}
}

// --- SQLite Operations ---

// InsertSQLiteLog inserts a single log entry into the SQLite database
func (r *logRepositoryImpl) InsertSQLiteLog(ctx context.Context, entry models.LogEntry) error {
	query := `INSERT INTO tbl_log (timestamp, level, message, fields) VALUES (?, ?, ?, ?)`

	// Serialize extra fields if any (assuming entry.Fields contains the JSON string)
	fieldsJSON := entry.Fields
	if fieldsJSON == "" {
        fieldsJSON = "{}" // Store empty JSON object if fields are nil/empty
    }


	_, err := r.sqliteDB.ExecContext(ctx, query, entry.Timestamp, entry.Level, entry.Message, fieldsJSON)
	if err != nil {
		r.logger.Error("Failed to insert log into SQLite", zap.Error(err))
		return fmt.Errorf("sqlite insert failed: %w", err)
	}
	return nil
}

// GetSQLiteLogs retrieves a batch of log entries from SQLite, ordered by timestamp/ID
func (r *logRepositoryImpl) GetSQLiteLogs(ctx context.Context, limit int) ([]models.LogEntry, error) {
	query := `SELECT id, timestamp, level, message, fields FROM tbl_log ORDER BY id ASC LIMIT ?`
	rows, err := r.sqliteDB.QueryContext(ctx, query, limit)
	if err != nil {
		r.logger.Error("Failed to query logs from SQLite", zap.Error(err))
		return nil, fmt.Errorf("sqlite query failed: %w", err)
	}
	defer rows.Close()

	var logs []models.LogEntry
	for rows.Next() {
		var entry models.LogEntry
		var ts string // Read timestamp as string initially for flexibility
		var fields sql.NullString // Handle potential null fields

		if err := rows.Scan(&entry.ID, &ts, &entry.Level, &entry.Message, &fields); err != nil {
			r.logger.Error("Failed to scan log row from SQLite", zap.Error(err))
			// Continue processing other rows if possible, or return error immediately
			continue // Or return nil, fmt.Errorf("sqlite scan failed: %w", err)
		}

        // Try parsing the timestamp string (adjust format if needed)
        entry.Timestamp, err = time.Parse(time.RFC3339Nano, ts) // Use RFC3339Nano or format matching Zap output
        if err != nil {
            // Fallback or handle error if parsing fails
            r.logger.Warn("Failed to parse timestamp from SQLite, using current time", zap.String("raw_ts", ts), zap.Error(err))
            entry.Timestamp = time.Now() // Or skip the entry
        }

		if fields.Valid {
			entry.Fields = fields.String
		} else {
            entry.Fields = "{}" // Default to empty JSON object
        }

		logs = append(logs, entry)
	}

	if err = rows.Err(); err != nil {
		r.logger.Error("Error during iteration over SQLite log rows", zap.Error(err))
		return nil, fmt.Errorf("sqlite row iteration error: %w", err)
	}

	return logs, nil
}

// DeleteSQLiteLogsByID deletes log entries from SQLite based on their IDs
func (r *logRepositoryImpl) DeleteSQLiteLogsByID(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil // Nothing to delete
	}

	// Build the query with placeholders for variable number of IDs
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`DELETE FROM tbl_log WHERE id IN (%s)`, strings.Join(placeholders, ","))

	result, err := r.sqliteDB.ExecContext(ctx, query, args...)
	if err != nil {
		r.logger.Error("Failed to delete logs from SQLite", zap.Error(err))
		return fmt.Errorf("sqlite delete failed: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	r.logger.Debug("Deleted logs from SQLite", zap.Int64("rows_affected", rowsAffected), zap.Int("id_count", len(ids)))
	return nil
}

// --- Oracle Operations ---

// InsertBatchOracle inserts a slice of LogEntry into Oracle tbl_log using a batch operation
func (r *logRepositoryImpl) InsertBatchOracle(ctx context.Context, logs []models.LogEntry) error {
	if len(logs) == 0 {
		return nil
	}

	// Begin transaction
	tx, err := r.oracleDB.BeginTx(ctx, nil)
	if err != nil {
		r.logger.Error("Failed to begin Oracle transaction for log batch insert", zap.Error(err))
		return fmt.Errorf("oracle begin tx failed: %w", err)
	}
	defer tx.Rollback() // Rollback if commit is not called

	// Prepare statement - IMPORTANT: Adjust columns to match your Oracle tbl_log schema exactly
	// Example: Assuming tbl_log has columns: log_timestamp, log_level, log_message, log_details (CLOB/JSON)
	query := `INSERT INTO tbl_log (log_timestamp, log_level, log_message, log_details) VALUES (:1, :2, :3, :4)`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		r.logger.Error("Failed to prepare Oracle batch insert statement", zap.Error(err))
		return fmt.Errorf("oracle prepare statement failed: %w", err)
	}
	defer stmt.Close()

	var insertErrors []error
	// Execute statement for each log entry
	for _, entry := range logs {
		// Ensure fields is valid JSON or handle accordingly for Oracle target column type (e.g., CLOB)
        fieldsData := entry.Fields
        if fieldsData == "" {
            fieldsData = "{}"
        }

        // Ensure timestamp is in a format Oracle understands or use Oracle'\''s SYSTIMESTAMP if preferred in the INSERT
		_, err := stmt.ExecContext(ctx, entry.Timestamp, entry.Level, entry.Message, fieldsData) // Adjust parameters to match query placeholders
		if err != nil {
			// Log individual errors but continue trying to insert others in the batch
			r.logger.Error("Failed to execute Oracle batch insert for one entry", zap.Error(err), zap.Time("log_ts", entry.Timestamp))
			insertErrors = append(insertErrors, err)
            // Depending on requirements, you might want to break the loop on first error
		}
	}

    // Check if any errors occurred during the batch execution
	if len(insertErrors) > 0 {
		// Rollback is handled by defer, just return a combined error
        // Consider how to handle partial success if needed
		r.logger.Error("Errors occurred during Oracle log batch insert", zap.Int("error_count", len(insertErrors)))
		return fmt.Errorf("oracle batch insert failed for %d entries (first error: %w)", len(insertErrors), insertErrors[0])
	}


	// Commit transaction
	if err := tx.Commit(); err != nil {
		r.logger.Error("Failed to commit Oracle transaction for log batch insert", zap.Error(err))
		return fmt.Errorf("oracle commit failed: %w", err)
	}

	r.logger.Debug("Successfully inserted log batch into Oracle", zap.Int("batch_size", len(logs)))
	return nil
}


// --- SQLite Log Writer (Helper for Zap) ---

// SQLiteLogWriter implements io.Writer to write logs directly to SQLite
type SQLiteLogWriter struct {
	repo   LogRepository
	logger *zap.Logger
}

// NewSQLiteLogWriter creates a writer for Zap logs
func NewSQLiteLogWriter(repo LogRepository, logger *zap.Logger) *SQLiteLogWriter {
	return &SQLiteLogWriter{repo: repo, logger: logger}
}

// Write implements io.Writer. It expects structured log data (not raw bytes typically).
// This basic version might just store the raw message. A better approach integrates
// with Zap'\''s Core/Encoder to get structured fields.
// NOTE: This simple Write approach is NOT ideal for structured logging into DB.
// See logger.go for a better approach using zapcore.Core.
func (w *SQLiteLogWriter) Write(p []byte) (n int, err error) {
	// This is a fallback - ideally use a custom Zap Core to get structured data.
	entry := models.LogEntry{
		Timestamp: time.Now(),
		Level:     "info", // Cannot determine level from raw bytes easily
		Message:   string(p),
		Fields:    "{}", // No structured fields here
	}
	err = w.repo.InsertSQLiteLog(context.Background(), entry) // Use background context for logging
	if err != nil {
		w.logger.Error("SQLiteLogWriter failed to write", zap.Error(err))
		return 0, err
	}
	return len(p), nil
}

// Sync implements zapcore.WriteSyncer. Usually a no-op for DB writers unless buffering.
func (w *SQLiteLogWriter) Sync() error {
	return nil
}

'
write_to_file "$PROJECT_NAME/internal/repositories/log_repo.go" "$LOG_REPO_GO_CONTENT"


# internal/logging/logger.go
LOGGER_GO_CONTENT='
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"'$PROJECT_NAME'/internal/config"
	"'$PROJECT_NAME'/internal/models"
	"'$PROJECT_NAME'/internal/repositories"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var globalLogger *zap.Logger

// InitLogger initializes the Zap logger with multiple cores: console, rotating file, and SQLite.
func InitLogger(cfg *config.Config, logRepo repositories.LogRepository) (*zap.Logger, error) {
	// --- Configuration ---
	logLevel := zapcore.InfoLevel // Default level
	if cfg.AppEnv == "local" || cfg.AppEnv == "development" {
		logLevel = zapcore.DebugLevel
	}

	// --- Encoders ---
	// Console Encoder (human-readable, colored)
	consoleEncoderCfg := zap.NewDevelopmentEncoderConfig()
	consoleEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder // Colored level
	consoleEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder       // Human-readable time
    consoleEncoderCfg.EncodeCaller = zapcore.ShortCallerEncoder      // Show file:line

	// File & SQLite Encoder (JSON format for machine readability)
	jsonEncoderCfg := zap.NewProductionEncoderConfig()
	jsonEncoderCfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder // Precise timestamp RFC3339Nano matches SQLite better
    jsonEncoderCfg.EncodeCaller = zapcore.ShortCallerEncoder

	// --- Write Syncers ---
	// Console Writer
	consoleWriter := zapcore.Lock(os.Stdout)

	// Rotating File Writer
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.LogFilePath,
		MaxSize:    cfg.LogMaxSize,    // megabytes
		MaxBackups: cfg.LogMaxBackups, // number of old log files to retain
		MaxAge:     cfg.LogMaxAge,     // days
		Compress:   cfg.LogCompress,   // compress old log files
		LocalTime:  true,              // use local time for timestamps in filenames
	})

	// SQLite Writer (using a custom Core)
	sqliteCore := NewSQLiteCore(logLevel, zapcore.NewJSONEncoder(jsonEncoderCfg), logRepo)


	// --- Cores ---
	// Core for Console output
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(consoleEncoderCfg),
		consoleWriter,
		logLevel, // Log >= Info level to console
	)

	// Core for File output
	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(jsonEncoderCfg), // Use JSON format for files
		fileWriter,
		logLevel, // Log >= Debug level to file (adjust as needed)
	)

	// --- Combine Cores ---
	// Tee creates a core that duplicates logs to multiple underlying cores.
	// Order matters if levels differ; logs are filtered by each core'\''s level.
	combinedCore := zapcore.NewTee(
		consoleCore, // To console
		fileCore,    // To rotating file
		sqliteCore,  // To SQLite via custom core
	)

	// --- Build Logger ---
	// Add CallerSkip to ensure caller information points to the actual call site, not the logger wrapper.
	// Add Stacktrace adds stacktraces for Error level logs and above.
	logger := zap.New(combinedCore, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))

	globalLogger = logger // Store globally if needed, though dependency injection is preferred

	logger.Info("Logger initialized",
		zap.String("environment", cfg.AppEnv),
		zap.String("level", logLevel.String()),
		zap.String("logFile", cfg.LogFilePath),
		zap.Int("maxSizeMB", cfg.LogMaxSize),
		zap.Int("maxBackups", cfg.LogMaxBackups),
		zap.Int("maxAgeDays", cfg.LogMaxAge),
		zap.Bool("compress", cfg.LogCompress),
		zap.String("sqliteDB", cfg.SQLiteDBPath),
	)

	return logger, nil
}

// GetLogger returns the initialized global logger (use with caution, prefer injection)
func GetLogger() *zap.Logger {
	if globalLogger == nil {
        // Fallback basic logger if not initialized (should not happen in normal flow)
		fallbackLogger, _ := zap.NewDevelopment()
        fallbackLogger.Warn("Global logger accessed before initialization!")
        return fallbackLogger
	}
	return globalLogger
}


// --- Custom SQLite Zap Core ---

type sqliteCore struct {
	zapcore.LevelEnabler                   // Embed LevelEnabler for level filtering
	encoder zapcore.Encoder                // JSON encoder for log fields
	repo    repositories.LogRepository     // Repository to insert logs
	fields  []zapcore.Field                // Fields added with logger.With(...)
}

// NewSQLiteCore creates a Zap Core that writes logs to the SQLite database.
func NewSQLiteCore(enab zapcore.LevelEnabler, enc zapcore.Encoder, repo repositories.LogRepository) zapcore.Core {
	return &sqliteCore{
		LevelEnabler: enab,
		encoder:      enc.Clone(), // Clone encoder to avoid state issues
		repo:         repo,
        fields:       make([]zapcore.Field, 0),
	}
}

// Enabled implements zapcore.Core. Checks if the given level is enabled.
func (c *sqliteCore) Enabled(level zapcore.Level) bool {
	return c.LevelEnabler.Enabled(level)
}

// With implements zapcore.Core. Adds structured context fields to the logger.
func (c *sqliteCore) With(fields []zapcore.Field) zapcore.Core {
	clone := c.clone()
    // Append new fields, handling potential key conflicts if necessary
    clone.fields = append(clone.fields, fields...) // Simple append
	clone.encoder = c.encoder.Clone() // Ensure encoder state is separate
	// Add fields to the cloned encoder'\''s context if the encoder supports it
    // (Zap'\''s standard JSON encoder doesn'\''t directly store context this way,
    // it relies on the fields passed during Write)
	return clone
}

// Check implements zapcore.Core. Checks if the log entry should be logged.
func (c *sqliteCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

// Write implements zapcore.Core. This is where the log entry is written to SQLite.
func (c *sqliteCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
    // Combine fields from With(...) and the current log call
    allFields := append(c.fields, fields...)

	// Encode the fields into a buffer using the JSON encoder
	buf, err := c.encoder.EncodeEntry(ent, allFields)
	if err != nil {
        // Log encoding error using a fallback mechanism (e.g., stderr)
        fmt.Fprintf(os.Stderr, "Failed to encode log entry for SQLite: %v\n", err)
		return fmt.Errorf("failed to encode log entry: %w", err)
	}

    // The buffer `buf` now contains the full JSON log entry.
    // We need to extract the core parts and the extra fields for our DB schema.

    // Approach 1: Decode the JSON back into a map to extract fields
    var decoded map[string]interface{}
    if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
         fmt.Fprintf(os.Stderr, "Failed to decode JSON log entry for SQLite: %v\n", err)
         // Fallback: store the raw JSON? Or just the message?
         // For simplicity, we'\''ll proceed but log the error
    }

    // Prepare LogEntry for the repository
	logEntry := models.LogEntry{
		Timestamp: ent.Time,
		Level:     ent.Level.String(),
		Message:   ent.Message,
		Fields:    "{}", // Default empty JSON
	}

    // Extract core fields and put the rest into the Fields JSON
    if decoded != nil {
        // Remove standard fields handled by columns, put rest in '\''Fields'\''
        delete(decoded, "ts") // Or whatever the encoder uses for timestamp
        delete(decoded, "level")
        delete(decoded, "msg")
        delete(decoded, "caller") // If caller is encoded
        delete(decoded, "stacktrace") // If stacktrace is encoded

        // Marshal remaining fields back to JSON for the '\''Fields'\'' column
        if len(decoded) > 0 {
             fieldBytes, err := json.Marshal(decoded)
             if err == nil {
                 logEntry.Fields = string(fieldBytes)
             } else {
                  fmt.Fprintf(os.Stderr, "Failed to marshal extra fields for SQLite: %v\n", err)
                  logEntry.Fields = `{"error": "failed to marshal fields"}`
             }
        }
    } else {
        // If decoding failed, maybe store the raw buffer message?
        logEntry.Fields = fmt.Sprintf(`{"raw_log": "%s"}`, buf.String()) // Example fallback
    }


	// Insert into SQLite using the repository
    // Use background context as logging should generally not block the request flow
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // Add timeout
	defer cancel()
	err = c.repo.InsertSQLiteLog(ctx, logEntry)
	if err != nil {
        // Log insertion error using a fallback mechanism (e.g., stderr)
        fmt.Fprintf(os.Stderr, "Failed to insert log entry into SQLite: %v\n", err)
		return fmt.Errorf("failed to insert log into sqlite: %w", err)
	}

	// If buffer needs flushing (unlikely here), call buf.Free()
	buf.Free() // Release buffer resources

	return nil
}

// Sync implements zapcore.Core. Flush any buffered logs (no-op for this direct writer).
func (c *sqliteCore) Sync() error {
	return nil
}

// clone creates a copy of the core, necessary for With(...)
func (c *sqliteCore) clone() *sqliteCore {
	return &sqliteCore{
		LevelEnabler: c.LevelEnabler,
		encoder:      c.encoder.Clone(), // Clone encoder too
		repo:         c.repo,
        fields:       append([]zapcore.Field(nil), c.fields...), // Deep copy slice header + underlying array ref
	}
}
'
write_to_file "$PROJECT_NAME/internal/logging/logger.go" "$LOGGER_GO_CONTENT"


# internal/logging/processor.go
PROCESSOR_GO_CONTENT='
package logging

import (
	"context"
	"time"

	"'$PROJECT_NAME'/internal/config"
	"'$PROJECT_NAME'/internal/repositories"

	"go.uber.org/zap"
)

const (
	batchSize = 100 // Number of logs to fetch and insert in one batch (configurable maybe?)
    errorRetryDelay = 10 * time.Second // Delay after a failed batch insert to Oracle
)

// LogProcessor handles the transfer of logs from SQLite to Oracle
type LogProcessor struct {
	cfg       *config.Config
	logRepo   repositories.LogRepository
	logger    *zap.Logger
	ticker    *time.Ticker
	stopChan  chan struct{}
	isRunning bool
}

// NewLogProcessor creates a new LogProcessor instance
func NewLogProcessor(cfg *config.Config, logRepo repositories.LogRepository, logger *zap.Logger) *LogProcessor {
	return &LogProcessor{
		cfg:      cfg,
		logRepo:  logRepo,
		logger:   logger,
		stopChan: make(chan struct{}),
	}
}

// Start begins the log processing loop in a separate goroutine
func (p *LogProcessor) Start() {
	if p.isRunning {
		p.logger.Warn("Log processor already running")
		return
	}
	p.ticker = time.NewTicker(p.cfg.LogBatchInterval)
	p.isRunning = true
	go p.run()
	p.logger.Info("SQLite to Oracle log processor started", zap.Duration("interval", p.cfg.LogBatchInterval))
}

// Stop signals the log processing loop to terminate gracefully
func (p *LogProcessor) Stop() {
	if !p.isRunning {
		p.logger.Warn("Log processor not running")
		return
	}
	p.logger.Info("Stopping SQLite to Oracle log processor...")
	close(p.stopChan) // Signal the loop to stop
	p.ticker.Stop()   // Stop the ticker
	p.isRunning = false
	p.logger.Info("Log processor stopped")
    // Consider performing one last processing cycle here before fully stopping
    p.processBatch()
}

// run is the main loop that periodically processes log batches
func (p *LogProcessor) run() {
	for {
		select {
		case <-p.ticker.C:
			p.processBatch()
		case <-p.stopChan:
			p.logger.Info("Received stop signal, exiting log processing loop.")
			return
		}
	}
}

// processBatch fetches logs from SQLite, inserts them into Oracle, and deletes them from SQLite on success
func (p *LogProcessor) processBatch() {
	p.logger.Debug("Processing log batch...")
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.LogBatchInterval - 1*time.Second) // Timeout slightly less than interval
	defer cancel()

	// 1. Get logs from SQLite
	logs, err := p.logRepo.GetSQLiteLogs(ctx, batchSize)
	if err != nil {
		p.logger.Error("Failed to get logs from SQLite", zap.Error(err))
		return // Retry on the next tick
	}

	if len(logs) == 0 {
		p.logger.Debug("No logs in SQLite to process")
		return // Nothing to do
	}

	p.logger.Debug("Fetched logs from SQLite", zap.Int("count", len(logs)))

	// 2. Insert logs into Oracle
	err = p.logRepo.InsertBatchOracle(ctx, logs)
	if err != nil {
		p.logger.Error("Failed to insert log batch into Oracle, will retry later", zap.Error(err))
        // Consider adding a delay or backoff mechanism here if Oracle errors persist
        // time.Sleep(errorRetryDelay) // Simple delay
		return // Do not delete from SQLite if Oracle insert failed
	}

	p.logger.Debug("Successfully inserted log batch into Oracle", zap.Int("count", len(logs)))

	// 3. Delete logs from SQLite
	logIDs := make([]int64, len(logs))
	for i, log := range logs {
		logIDs[i] = log.ID
	}

	err = p.logRepo.DeleteSQLiteLogsByID(ctx, logIDs)
	if err != nil {
		// This is problematic: logs are in Oracle but not deleted from SQLite.
		// Requires manual intervention or more robust error handling (e.g., mark as processed).
		p.logger.Error("CRITICAL: Failed to delete logs from SQLite after successful Oracle insert", zap.Error(err), zap.Int64s("log_ids", logIDs))
		// You might want to implement a mechanism to retry deletion or flag these logs.
		return
	}

	p.logger.Info("Processed and transferred log batch", zap.Int("count", len(logs)))

    // If there might be more logs than the batch size, process immediately again
    if len(logs) == batchSize {
        p.logger.Debug("Batch size reached, checking for more logs immediately.")
        // Use a non-blocking send or a small delay to avoid tight loop on continuous high volume
        go func() {
             time.Sleep(100 * time.Millisecond) // Small delay
             p.processBatch()
        }()
    }
}
'
write_to_file "$PROJECT_NAME/internal/logging/processor.go" "$PROCESSOR_GO_CONTENT"

# internal/services/auth_service.go
AUTH_SERVICE_GO_CONTENT='
package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"'$PROJECT_NAME'/internal/models"
	"'$PROJECT_NAME'/internal/repositories"
	"'$PROJECT_NAME'/internal/utils"
	"go.uber.org/zap"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUsernameExists    = errors.New("username already exists")
    ErrRegistrationFailed = errors.New("failed to register user")
)

// AuthService defines the interface for authentication related operations
type AuthService interface {
	Register(ctx context.Context, username, password, photoPath string) error
	Login(ctx context.Context, username, password string) (string, error) // Returns JWT token
}

type authServiceImpl struct {
	userRepo   repositories.UserRepository
	logger     *zap.Logger
	jwtSecret  string
	jwtExpires time.Duration
}

// NewAuthService creates a new AuthService
func NewAuthService(userRepo repositories.UserRepository, logger *zap.Logger, jwtSecret string) AuthService {
	return &authServiceImpl{
		userRepo:   userRepo,
		logger:     logger,
		jwtSecret:  jwtSecret,
		jwtExpires: 24 * time.Hour, // Default JWT expiration (make configurable?)
	}
}

// Register handles new user registration
func (s *authServiceImpl) Register(ctx context.Context, username, password, photoPath string) error {
	s.logger.Info("Attempting to register user", zap.String("username", username))

	// Check if username already exists
	existingUser, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		// Log the underlying DB error but return a generic registration error
		s.logger.Error("Error checking for existing username", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}
	if existingUser != nil {
		s.logger.Warn("Registration attempt failed: username already exists", zap.String("username", username))
		return ErrUsernameExists
	}

	// Hash the password
	hashedPassword, err := utils.HashPassword(password)
	if err != nil {
		s.logger.Error("Failed to hash password during registration", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}

	// Create the user model
	newUser := &models.User{
		Username:    username,
		PasswordHash: hashedPassword,
		PhotoPath:   photoPath,
		// CreatedAt/UpdatedAt are usually handled by the database (or repo)
	}

	// Save the user to the database
	_, err = s.userRepo.CreateUser(ctx, newUser)
	if err != nil {
		s.logger.Error("Failed to create user in database", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}

	s.logger.Info("User registered successfully", zap.String("username", username), zap.Int64("userID", newUser.ID))
	return nil
}

// Login handles user login and JWT generation
func (s *authServiceImpl) Login(ctx context.Context, username, password string) (string, error) {
	s.logger.Info("Attempting to login user", zap.String("username", username))

	// Find the user by username
	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		s.logger.Error("Error finding user during login", zap.String("username", username), zap.Error(err))
		return "", ErrInvalidCredentials // Generic error even if DB error
	}
	if user == nil {
		s.logger.Warn("Login attempt failed: user not found", zap.String("username", username))
		return "", ErrUserNotFound // Or ErrInvalidCredentials for security
	}

	// Check the password
	if !utils.CheckPasswordHash(password, user.PasswordHash) {
		s.logger.Warn("Login attempt failed: invalid password", zap.String("username", username))
		return "", ErrInvalidCredentials
	}

	// Generate JWT token
	token, err := utils.GenerateToken(user.ID, s.jwtSecret, s.jwtExpires)
	if err != nil {
		s.logger.Error("Failed to generate JWT token during login", zap.String("username", username), zap.Int64("userID", user.ID), zap.Error(err))
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	s.logger.Info("User logged in successfully", zap.String("username", username), zap.Int64("userID", user.ID))
	return token, nil
}

'
write_to_file "$PROJECT_NAME/internal/services/auth_service.go" "$AUTH_SERVICE_GO_CONTENT"

# internal/services/profile_service.go
PROFILE_SERVICE_GO_CONTENT='
package services

import (
	"context"
	"'$PROJECT_NAME'/internal/models"
	"'$PROJECT_NAME'/internal/repositories"
	"go.uber.org/zap"
    "fmt"
)

// ProfileService defines the interface for user profile operations
type ProfileService interface {
	GetProfile(ctx context.Context, userID int64) (*models.User, error)
	// Add UpdateProfile, etc. later
}

type profileServiceImpl struct {
	userRepo repositories.UserRepository
	logger   *zap.Logger
}

// NewProfileService creates a new ProfileService
func NewProfileService(userRepo repositories.UserRepository, logger *zap.Logger) ProfileService {
	return &profileServiceImpl{
		userRepo: userRepo,
		logger:   logger,
	}
}

// GetProfile retrieves the profile for the given user ID
func (s *profileServiceImpl) GetProfile(ctx context.Context, userID int64) (*models.User, error) {
	s.logger.Debug("Fetching profile for user", zap.Int64("userID", userID))

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
        s.logger.Error("Error fetching profile from repository", zap.Int64("userID", userID), zap.Error(err))
		return nil, fmt.Errorf("could not retrieve profile: %w", err)
	}
	if user == nil {
        s.logger.Warn("Profile requested for non-existent user ID", zap.Int64("userID", userID))
		return nil, ErrUserNotFound // Use the error from AuthService or define locally
	}

	// The user object from the repo already excludes the password hash via JSON tag
	// If you need to further sanitize data before returning, do it here.
	s.logger.Debug("Profile fetched successfully", zap.Int64("userID", userID), zap.String("username", user.Username))
	return user, nil
}

'
write_to_file "$PROJECT_NAME/internal/services/profile_service.go" "$PROFILE_SERVICE_GO_CONTENT"


# internal/middleware/jwt_middleware.go
JWT_MIDDLEWARE_GO_CONTENT='
package middleware

import (
	"strings"
	"'$PROJECT_NAME'/internal/utils"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

const (
	AuthorizationHeader = "Authorization"
	BearerPrefix        = "Bearer "
    UserIDKey           = "userID" // Key to store user ID in Fiber Locals
)

// Protected returns a Fiber middleware function that checks for a valid JWT
func Protected(jwtSecret string, logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get(AuthorizationHeader)

		if authHeader == "" {
			logger.Warn("Missing Authorization header")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing authorization header",
			})
		}

		if !strings.HasPrefix(authHeader, BearerPrefix) {
			logger.Warn("Invalid Authorization header format", zap.String("header", authHeader))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid authorization format (Bearer token required)",
			})
		}

		tokenString := strings.TrimPrefix(authHeader, BearerPrefix)
		if tokenString == "" {
            logger.Warn("Empty token string after Bearer prefix")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing token",
			})
		}


		claims, err := utils.ValidateToken(tokenString, jwtSecret)
		if err != nil {
			logger.Warn("Invalid JWT token", zap.Error(err), zap.String("token", tokenString)) // Be careful logging tokens
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
				// "detail": err.Error(), // Avoid exposing detailed errors in prod
			})
		}

		// Token is valid, store user ID in context for downstream handlers
		c.Locals(UserIDKey, claims.UserID)
        logger.Debug("JWT validated successfully", zap.Int64("userID", claims.UserID))

		// Proceed to the next handler
		return c.Next()
	}
}
'
write_to_file "$PROJECT_NAME/internal/middleware/jwt_middleware.go" "$JWT_MIDDLEWARE_GO_CONTENT"


# internal/handlers/auth_handler.go
AUTH_HANDLER_GO_CONTENT='
package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"'$PROJECT_NAME'/internal/services"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
    "github.com/google/uuid" // For generating unique filenames
)

// AuthHandler handles authentication related HTTP requests
type AuthHandler struct {
	authService services.AuthService
	logger      *zap.Logger
    uploadDir   string
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(authService services.AuthService, logger *zap.Logger, uploadDir string) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		logger:      logger,
        uploadDir:   uploadDir,
	}
}

// LoginRequest defines the expected JSON body for login requests
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// Login handles POST /auth/login requests
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Warn("Failed to parse login request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

    // Optional: Add validation using a library like go-playground/validator
    // if err := validate.Struct(req); err != nil {
    //     return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Validation failed", "details": err.Error()})
    // }

	if req.Username == "" || req.Password == "" {
         return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Username and password are required",
		})
    }


	token, err := h.authService.Login(c.Context(), req.Username, req.Password)
	if err != nil {
		// Service layer returns specific errors we can map to HTTP status codes
		switch err {
		case services.ErrUserNotFound, services.ErrInvalidCredentials:
			h.logger.Warn("Login failed", zap.String("username", req.Username), zap.Error(err))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": err.Error(), // Use the error message from the service
			})
		default:
			h.logger.Error("Internal server error during login", zap.String("username", req.Username), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Login failed due to an internal error",
			})
		}
	}

	h.logger.Info("Login successful", zap.String("username", req.Username))
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Login successful",
		"token":   token,
	})
}

// Register handles POST /auth/register requests (multipart/form-data)
func (h *AuthHandler) Register(c *fiber.Ctx) error {
    // Expecting multipart/form-data
	username := c.FormValue("username")
	password := c.FormValue("password")

    if username == "" || password == "" {
         return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Username and password form fields are required",
		})
    }

    // Handle file upload
	file, err := c.FormFile("photo")
    var photoPath string = "" // Relative path to store in DB

	if err == nil { // File was provided
        // Generate unique filename to prevent collisions
        ext := filepath.Ext(file.Filename)
        uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
		uploadPath := filepath.Join(h.uploadDir, uniqueFilename)

        // Ensure upload directory exists (should be done at startup, but double-check)
        if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
             h.logger.Error("Failed to ensure upload directory exists", zap.String("path", h.uploadDir), zap.Error(err))
             return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save photo"})
        }


		// Save the file
		if err := c.SaveFile(file, uploadPath); err != nil {
			h.logger.Error("Failed to save uploaded photo", zap.String("filename", file.Filename), zap.String("path", uploadPath), zap.Error(err))
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save photo"})
		}
		h.logger.Info("Saved uploaded photo", zap.String("original_filename", file.Filename), zap.String("saved_path", uploadPath))
        photoPath = uniqueFilename // Store only the filename or relative path in DB
	} else if err != http.ErrMissingFile {
        // An error occurred other than the file simply not being provided
        h.logger.Error("Error retrieving uploaded photo", zap.Error(err))
        return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Error processing photo upload"})
    }
    // If err == http.ErrMissingFile, photoPath remains empty, which is acceptable


	// Call the registration service
	err = h.authService.Register(c.Context(), username, password, photoPath)
	if err != nil {
		switch err {
		case services.ErrUsernameExists:
			h.logger.Warn("Registration failed: username exists", zap.String("username", username))
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": err.Error(),
			})
        case services.ErrRegistrationFailed:
            h.logger.Error("Registration failed due to service error", zap.String("username", username), zap.Error(err))
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Registration failed", // Generic message
			})
		default:
			h.logger.Error("Internal server error during registration", zap.String("username", username), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Registration failed due to an internal error",
			})
		}
	}

	h.logger.Info("Registration successful", zap.String("username", username))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User registered successfully",
		// Optionally return user info (excluding password) or just success message
	})
}

// SetupAuthRoutes registers authentication routes with the Fiber app
func (h *AuthHandler) SetupAuthRoutes(router fiber.Router) {
    authGroup := router.Group("/auth")
	authGroup.Post("/login", h.Login)
	authGroup.Post("/register", h.Register)
}

'
write_to_file "$PROJECT_NAME/internal/handlers/auth_handler.go" "$AUTH_HANDLER_GO_CONTENT"


# internal/handlers/profile_handler.go
PROFILE_HANDLER_GO_CONTENT='
package handlers

import (
	"'$PROJECT_NAME'/internal/middleware"
	"'$PROJECT_NAME'/internal/services"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// ProfileHandler handles profile related HTTP requests
type ProfileHandler struct {
	profileService services.ProfileService
	logger         *zap.Logger
}

// NewProfileHandler creates a new ProfileHandler
func NewProfileHandler(profileService services.ProfileService, logger *zap.Logger) *ProfileHandler {
	return &ProfileHandler{
		profileService: profileService,
		logger:         logger,
	}
}

// GetProfile handles GET /api/profile requests
func (h *ProfileHandler) GetProfile(c *fiber.Ctx) error {
	// User ID should be set in Locals by the JWT middleware
	userIDVal := c.Locals(middleware.UserIDKey)
	userID, ok := userIDVal.(int64) // Type assertion
	if !ok || userID == 0 {
        h.logger.Error("User ID not found or invalid in context/locals after JWT validation", zap.Any("value", userIDVal))
		// This should ideally not happen if JWT middleware is working correctly
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized: User ID missing from token context",
		})
	}

    h.logger.Debug("Handling GetProfile request", zap.Int64("userID", userID))

	// Fetch profile using the user ID
	profile, err := h.profileService.GetProfile(c.Context(), userID)
	if err != nil {
        switch err {
        case services.ErrUserNotFound:
             h.logger.Warn("Attempted to get profile for non-existent user confirmed by service", zap.Int64("userID", userID))
             // This case might indicate a deleted user whose token is still valid briefly, or an inconsistency.
             return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Profile not found"})
        default:
            h.logger.Error("Failed to get profile", zap.Int64("userID", userID), zap.Error(err))
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve profile"})
        }
	}

	// Return the profile (PasswordHash is excluded by `json:"-"` tag in the model)
	return c.Status(fiber.StatusOK).JSON(profile)
}

// SetupProfileRoutes registers profile routes with the Fiber app (protected)
func (h *ProfileHandler) SetupProfileRoutes(router fiber.Router) {
    // Assuming the router passed here is already under the JWT middleware
	router.Get("/profile", h.GetProfile)
	// Add POST/PUT for updating profile later
    // router.Put("/profile", h.UpdateProfile)
}
'
write_to_file "$PROJECT_NAME/internal/handlers/profile_handler.go" "$PROFILE_HANDLER_GO_CONTENT"

# internal/app/app.go
APP_GO_CONTENT='
package app

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"'$PROJECT_NAME'/internal/config"
	"'$PROJECT_NAME'/internal/database"
	"'$PROJECT_NAME'/internal/handlers"
	"'$PROJECT_NAME'/internal/logging"
	mw "'$PROJECT_NAME'/internal/middleware" // Alias middleware package
	"'$PROJECT_NAME'/internal/repositories"
	"'$PROJECT_NAME'/internal/services"

	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

// Run initializes and starts the application
func Run() {
    // --- Early Logger Setup (for config loading etc.) ---
    // Use a temporary basic logger until the full logger is configured
    tempLogger, _ := zap.NewDevelopment() // Or zap.NewProduction()
    tempLogger.Info("Starting application initialization...")

    // --- Configuration ---
    cfg, err := config.LoadConfig(tempLogger) // Pass temp logger
	if err != nil {
		tempLogger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// --- Database Connections ---
	oracleDB, err := database.InitOracle(cfg, tempLogger)
	if err != nil {
		tempLogger.Fatal("Failed to initialize Oracle database", zap.Error(err))
	}
	defer func() {
		if err := oracleDB.Close(); err != nil {
			tempLogger.Error("Error closing Oracle database", zap.Error(err))
		} else {
            tempLogger.Info("Oracle database connection closed.")
        }
	}()

	sqliteDB, err := database.InitSQLite(cfg, tempLogger)
	if err != nil {
		tempLogger.Fatal("Failed to initialize SQLite database", zap.Error(err))
	}
    defer func() {
		if err := sqliteDB.Close(); err != nil {
			tempLogger.Error("Error closing SQLite database", zap.Error(err))
		} else {
            tempLogger.Info("SQLite database connection closed.")
        }
	}()

	// --- Repositories ---
	userRepo := repositories.NewOracleUserRepository(oracleDB, tempLogger) // Use tempLogger for now
	logRepo := repositories.NewLogRepository(sqliteDB, oracleDB, tempLogger)

    // --- Logging ---
	// Now initialize the full logger using the logRepo
    logger, err := logging.InitLogger(cfg, logRepo)
    if err != nil {
        tempLogger.Fatal("Failed to initialize application logger", zap.Error(err))
    }
    // Replace tempLogger references in previously initialized components
    // This is a bit awkward; dependency injection frameworks help here.
    // For now, we assume repos/DB init logging was minimal or re-log critical info.
    logger.Info("Application logger successfully initialized.")
    // Update repo loggers if they store the logger instance (our current impl doesn'\''t, it uses the passed one per call)
    // userRepo.(*repositories.oracleUserRepository).logger = logger // Requires type assertion and mutable field
    // logRepo.(*repositories.logRepositoryImpl).logger = logger   // Requires type assertion and mutable field
    // Better: Pass logger factory or use context with logger

    // Make logger available globally (use cautiously) or pass via context/DI
    // logging.SetGlobalLogger(logger) // Example if using a global logger


    // --- Log Processor ---
    logProcessor := logging.NewLogProcessor(cfg, logRepo, logger)
    logProcessor.Start()
    defer logProcessor.Stop() // Ensure it stops gracefully


	// --- Services ---
	authService := services.NewAuthService(userRepo, logger, cfg.JWTSecret)
	profileService := services.NewProfileService(userRepo, logger)

	// --- Handlers ---
	authHandler := handlers.NewAuthHandler(authService, logger, cfg.UploadDir)
	profileHandler := handlers.NewProfileHandler(profileService, logger)


	// --- Fiber App ---
	app := fiber.New(fiber.Config{
		// Prefork:      true, // Enable for production performance boost (test thoroughly)
		// CaseSensitive: true,
		StrictRouting: true,
		ServerHeader:  "MyFiberApp",
		AppName:       "My Go Fiber API v1.0",
        ErrorHandler: func(c *fiber.Ctx, err error) error {
            // Custom global error handler
            code := fiber.StatusInternalServerError
            var e *fiber.Error
            if errors.As(err, &e) {
                code = e.Code
            }
            logger.Error("Unhandled error", zap.Error(err), zap.String("path", c.Path()), zap.String("method", c.Method()))
            return c.Status(code).JSON(fiber.Map{"error": "An internal server error occurred"}) // Generic message
        },
	})

	// --- Middleware ---
    // Recovery middleware must be first
    app.Use(recover.New(recover.Config{
        EnableStackTrace: cfg.AppEnv != "production", // Show stack trace only in dev/local
        StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
             logger.Error("Panic recovered", zap.Any("panic_value", e), zap.Stack("stacktrace"))
        },
    }))

    // CORS
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // Adjust for production (e.g., "http://yourfrontend.com")
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
        AllowMethods: "GET, POST, HEAD, PUT, DELETE, PATCH",
	}))

    // Request/Response Logging (using fiberzap)
    app.Use(fiberzap.New(fiberzap.Config{
		Logger: logger, // Use our configured Zap logger
        // Custom message format (optional)
		// Format: "${status} | ${latency} | ${method} | ${path} | ${reqHeaders} | ${resBody}",
        Fields: []string{"latency", "status", "method", "url", "ip", "queryParams", "respHeader", "reqHeader"},
        //CustomAttributes: []zap.Field{
        //    zap.String("service", "myfiberapp"),
        //},
        Next: func(c *fiber.Ctx) bool {
             // Optionally skip logging for certain paths (e.g., health checks)
			 return c.Path() == "/health"
        },
        // Function to extract fields from response (be careful with large bodies)
        // GetResponseFields: func (c *fiber.Ctx) []zap.Field {
        //     if c.Response().StatusCode() >= 400 { // Log response body only for errors
        //         return []zap.Field{zap.String("response_body", string(c.Response().Body()))}
        //     }
        //     return nil
        // },
	}))


	// --- Routes ---
	// Health Check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

    // Static files for uploads (optional - consider serving via dedicated server like Nginx in prod)
    // Ensure UPLOAD_DIR matches the config used by the handler
    app.Static("/uploads", cfg.UploadDir, fiber.Static{
        // Compress: true, // Enable compression for static files
        // ByteRange: true, // Enable byte range requests
        Browse: cfg.AppEnv != "production", // Allow directory Browse only in non-prod
    })


    // API v1 Group
	api := app.Group("/api/v1")

	// Public routes (authentication)
	authHandler.SetupAuthRoutes(api) // Routes like /api/v1/auth/login

	// Protected routes (require JWT)
	protected := api.Group("/", mw.Protected(cfg.JWTSecret, logger)) // Apply JWT middleware
    profileHandler.SetupProfileRoutes(protected) // Routes like /api/v1/profile


	// --- Start Server ---
	go func() {
		logger.Info("Starting server", zap.String("address", ":"+cfg.Port))
		if err := app.Listen(":" + cfg.Port); err != nil {
			logger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // Block until signal is received

	logger.Info("Shutting down server...")

	// Give server time to finish processing requests
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	if err := app.ShutdownWithContext(ctxShutdown); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exiting")
}
'
write_to_file "$PROJECT_NAME/internal/app/app.go" "$APP_GO_CONTENT"

# cmd/api/main.go
MAIN_GO_CONTENT='
package main

import (
	"'$PROJECT_NAME'/internal/app"
)

func main() {
	app.Run()
}
'
write_to_file "$PROJECT_NAME/cmd/api/main.go" "$MAIN_GO_CONTENT"


# --- Final Steps ---
echo -e "${GREEN}Project structure and files generated in '$PROJECT_NAME/' directory.${NC}"
echo -e "${BOLD}Next steps:${NC}"
echo 	"1. cd $PROJECT_NAME"
echo -e "2. "${PURPLE}go mod tidy${NC}   # To download dependencies and clean up go.mod/go.sum"
echo 	"3. Adjust placeholders in .env.local (especially ORACLE_CONN_STRING)."
echo 	"4. Review the generated code, especially SQL queries in repositories to match your DB schema."
echo 	"5. Implement the actual logic within handlers, services, and repositories."
echo 	"6. Ensure your Oracle DB has the necessary tables (tbl_user, tbl_log) and permissions."
echo 	"   Example tbl_user:"
echo 	"     CREATE TABLE tbl_user ( user_id NUMBER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, username VARCHAR2(100) NOT NULL UNIQUE, password_hash VARCHAR2(100) NOT NULL, photo_path VARCHAR2(255), created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP );"
echo 	"   Example tbl_log (adjust column types/names as needed):"
echo 	"     CREATE TABLE tbl_log ( log_id NUMBER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, log_timestamp TIMESTAMP NOT NULL, log_level VARCHAR2(20), log_message CLOB, log_details CLOB );" # Using CLOB for potentially large message/details
echo -e	"7. Run the application: ${PURPLE}go run cmd/api/main.go${NC}"
