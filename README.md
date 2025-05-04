# go-webapi

This project is a backend web API developed in Go using the Fiber framework. It provides a robust foundation with features including JWT authentication, user profile management, dual database support (Oracle and SQLite), structured logging with background processing, environment-based configuration, and file uploads.

## Key Features

* **Framework:** [Fiber v2](https://gofiber.io/) (High-performance Go web framework)
* **Authentication:** JWT-based token authentication using `github.com/golang-jwt/jwt/v5`.
* **Database Support:**
    * **Oracle:** Used as the primary database for application data (e.g., users) via `github.com/godror/godror`.
    * **SQLite:** Used as a local buffer for application logs via `github.com/mattn/go-sqlite3`, ensuring logs are captured even if the primary database is temporarily unavailable.
* **Structured Logging:** Comprehensive logging using `go.uber.org/zap`.
    * **Multi-Destination:** Logs concurrently to Console (colored, development-friendly), a Rotating File (`gopkg.in/natefinch/lumberjack.v2`), and the local SQLite database.
    * **Request Logging:** Middleware (`github.com/gofiber/contrib/fiberzap/v2`) logs details for incoming HTTP requests.
* **Background Log Processing:** A dedicated background process (`LogProcessor`) periodically transfers buffered logs from SQLite to the Oracle database, including retry logic for Oracle connection issues.
* **Configuration:** Flexible configuration loading from `.env` files (e.g., `.env.local`, `.env.production`) using `github.com/joho/godotenv`, driven by the `APP_ENV` variable.
* **Modular Architecture:** Code is organized into distinct packages (`app`, `bootstrap`, `config`, `database`, `handlers`, `logging`, `middleware`, `models`, `repositories`, `routes`, `services`, `utils`) within the `internal` directory to promote separation of concerns and maintainability.
* **Dependency Wiring:** A `bootstrap` package centralizes the initialization and wiring of application components (repositories, logger, services, handlers, processor).
* **Middleware:** Includes standard middleware for Recovery, CORS, Request Logging, and JWT Authentication (`Protected` routes).
* **Graceful Shutdown:** Handles OS interrupt signals (SIGINT, SIGTERM) to shut down the server cleanly.
* **Password Security:** Uses `bcrypt` for hashing user passwords.
* **File Uploads:** Supports user photo uploads (e.g., during registration) saved to a configurable directory.
* **Project Scaffolding:** Includes a shell script (`scripts/generate_project.sh`) to generate the initial project structure and files.

## Project Structure
```
go-webapi/
├── cmd/
│   └── api/
│       └── main.go           # Application entry point. Imports and runs the core app from internal/app.
├── internal/
│   ├── app/
│   │   └── app.go            # Core application orchestration: Loads config, initializes base logger & DBs, calls bootstrap, sets up Fiber app & middleware, defines routes via routes package, manages server lifecycle.
│   ├── bootstrap/
│   │   └── bootstrap.go      # Dependency Injection / Wiring: Initializes repositories, the final multi-core logger, services, handlers, and the log processor. Returns wired components struct.
│   ├── config/
│   │   └── config.go         # Handles loading and parsing of application configuration from environment variables and .env files. Defines the Config struct.
│   ├── database/
│   │   ├── oracle.go         # Function for initializing the Oracle DB connection pool (sql.DB).
│   │   └── sqlite.go         # Function for initializing the SQLite DB connection (sql.DB) and creating the log table. Ensures directory exists.
│   ├── handlers/
│   │   ├── auth_handler.go   # HTTP handlers for authentication endpoints (/login, /register). Parses requests, calls AuthService, handles responses and file uploads.
│   │   └── profile_handler.go# HTTP handlers for user profile endpoints (e.g., /profile). Uses ProfileService.
│   ├── logging/
│   │   ├── logger.go         # Sets up Zap logger configurations (base console/file, final multi-core including SQLite). Implements the custom sqliteCore for Zap. Exports helpers and global logger accessors.
│   │   └── processor.go      # Implements the LogProcessor background service to transfer logs from SQLite to Oracle with retry logic.
│   ├── middleware/
│   │   └── jwt_middleware.go # Implements the JWT authentication middleware (Protected) using utils.ValidateToken. Sets user ID in Fiber context.
│   ├── models/
│   │   ├── user.go           # Defines the User struct representing the user data model (matches tbl_user).
│   │   └── log.go            # Defines the LogEntry struct representing the log data model (matches tbl_log).
│   ├── repositories/
│   │   ├── user_repo.go      # Defines UserRepository interface and provides Oracle implementation (oracleUserRepository). Handles DB interactions for users.
│   │   └── log_repo.go       # Defines LogRepository interface and provides implementation (logRepositoryImpl) handling both SQLite log inserts/reads/deletes and Oracle batch inserts. Includes mechanism to update Oracle DB handle dynamically.
│   ├── routes/
│   │   └── routes.go         # Defines all HTTP routes using Fiber (e.g., /health, /api/v1/), groups them, applies middleware, and maps them to handlers obtained from the AppComponents struct. Includes static file serving setup.
│   ├── services/
│   │   ├── auth_service.go   # Implements AuthService interface containing business logic for registration (password hashing, existence check) and login (credential validation, JWT generation).
│   │   └── profile_service.go# Implements ProfileService interface containing business logic for retrieving user profiles.
│   └── utils/
│       ├── password.go       # Utility functions for password hashing and verification using bcrypt.
│       └── jwt.go            # Utility functions for generating and validating JWT tokens.
├── logs/
│   └── logfile.log           # Example log file output (location configured via .env). Should be gitignored.
├── scripts/
│   └── generate_project.sh   # Shell script used to generate this project skeleton.
├── uploads/                  # Directory for storing user uploaded files (e.g., photos). Should be gitignored except for .gitkeep.
│   └── .gitkeep              # Placeholder to keep the directory in Git even if empty.
├── .env.local                # Example/Default environment variables for local development. Should be gitignored.
├── .env.prod                 # Example environment variables for production. Should be gitignored.
├── .gitignore                # Specifies intentionally untracked files (like .env, logs, uploads, binaries).
├── go.mod                    # Go module definition file (defines module path, Go version, dependencies).
├── go.sum                    # Go module checksums.
└── README.md                 # This file: Project overview, setup, and structure.
```
## Setup and Running

1.  **Prerequisites:**
    * Go (version specified in `go.mod`, e.g., 1.24 or later)
    * Access to an Oracle database instance.
    * Git

2.  **Clone the repository:**
    ```bash
    git clone <your-repository-url>
    cd go-webapi
    ```

3.  **Configuration:**
    * Copy `.env.local` or create a new `.env.<APP_ENV>` file (e.g., `.env.local`).
    * Update the environment variables within the file, especially:
        * `ORACLE_CONN_STRING`: Correct DSN for your Oracle database.
        * `JWT_SECRET`: **Change this to a strong, unique secret.**
        * `SQLITE_DB_PATH`: Path where the SQLite log database will be stored.
        * `LOG_FILE_PATH`: Path for the rotating log file.
        * `UPLOAD_DIR`: Path where uploaded files will be stored.
    * **Important:** Ensure the directories specified (`SQLITE_DB_PATH` parent, `LOG_FILE_PATH` parent, `UPLOAD_DIR`) exist or the application has permissions to create them.

4.  **Install Dependencies:**
    ```bash
    go mod tidy
    ```

5.  **Run the application:**
    * Set the active environment (optional, defaults to `local` if not set):
        ```bash
        # Example for Linux/macOS
        export APP_ENV=local
        # Example for Windows (Command Prompt)
        # set APP_ENV=local
        # Example for Windows (PowerShell)
        # $env:APP_ENV="local"
        ```
    * Run the main application:
        ```bash
        go run ./cmd/api/main.go
        ```
    The server will start, listening on the port defined in your `.env` file (default `3000`).

## Logging Strategy

1.  Application code uses the configured Zap logger (`finalLogger`).
2.  Zap writes logs simultaneously to:
    * Console (formatted for readability).
    * Rotating file (`app.log` or configured path).
    * SQLite database (`logs.db` or configured path) via a custom Zap core (`sqliteCore`).
3.  A background `LogProcessor` goroutine runs periodically.
4.  The `LogProcessor` reads log batches from the SQLite database.
5.  It attempts to insert these batches into the configured Oracle `tbl_log` table.
6.  If the Oracle insert succeeds, the corresponding logs are deleted from SQLite.
7.  If the Oracle insert fails due to connection issues, the processor attempts to re-initialize the connection and retries the batch insertion. Logs remain in SQLite until successfully transferred.

## Oracle Connection Handling

* The initial Oracle connection pool (`*sql.DB`) is created during application startup (`internal/database/oracle.go`). The standard `database/sql` package handles internal connection pooling and basic retries.
* The `LogProcessor` specifically monitors the connection when attempting batch inserts. If a connection-related error occurs, it tries to re-establish a connection handle (`database.InitOracle`) and updates the handle within the shared `LogRepository` instance to facilitate recovery for subsequent log transfer attempts. *(Note: Ensure components relying on Oracle, like UserRepository, correctly handle potential connection errors or are also updated if the DB handle needs global replacement)*.

## API Endpoints (Examples)

* `GET /health`: Checks service health and database connectivity.
* `POST /api/v1/auth/register`: Registers a new user (multipart/form-data: `username`, `password`, `photo` [optional]).
* `POST /api/v1/auth/login`: Logs in a user (JSON: `username`, `password`) -> Returns JWT.
* `GET /api/v1/profile`: Retrieves authenticated user's profile (Requires `Authorization: Bearer <token>`).
* `GET /uploads/*`: Serves static files stored in the upload directory.
