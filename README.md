# go-webapi

This project is a backend web API developed in Go using the Fiber framework. It provides a robust foundation with features including JWT authentication, user profile management, dual database support (Oracle and SQLite), structured and conditional logging with background processing, environment-based configuration, declarative request validation, and file uploads.

## Key Features

* **Framework:** [Fiber v2](https://gofiber.io/) (High-performance Go web framework)
* **Authentication:** JWT-based token authentication using `github.com/golang-jwt/jwt/v5`.
* **Database Support:**
    * **Oracle:** Used as the primary database for application data (e.g., users) via `github.com/godror/godror`.
    * **SQLite:** Used as a local buffer for specific application logs (e.g., audit trails, critical errors) via `github.com/mattn/go-sqlite3`. These logs are intended for persistence and potential transfer to Oracle.
* **Structured & Conditional Logging:** Comprehensive logging using `go.uber.org/zap`.
    * **Dual Logger System:**
        * **File/Console Logger (`fileLogger`):** Handles general application operational logs. Writes concurrently to the Console (colored, development-friendly) and a Rotating File (`github.com/DeRuina/timberjack` which wraps `gopkg.in/natefinch/lumberjack.v2`).
        * **Dedicated SQLite Logger (`sqliteLogger`):** Used for logging specific, important events (e.g., audit trails, security events) directly to the local SQLite database. This logger can be enabled/disabled and its log level configured independently.
    * **Request Logging:** Middleware (`github.com/gofiber/contrib/fiberzap/v2`) logs details for incoming HTTP requests using the `fileLogger`.
    * **Custom Startup Banner:** The application's startup information (Fiber version, listening address, PID, etc.) is logged to the `fileLogger` in a custom banner format, while Fiber's default console banner is disabled.
* **Background Log Processing:** A dedicated background process (`LogProcessor`) periodically transfers logs from the SQLite database (those written by `sqliteLogger`) to the Oracle database, including retry logic for Oracle connection issues.
* **Configuration:** Flexible configuration loading from `.env` files (e.g., `.env.local`, `.env.production`) using `github.com/joho/godotenv`, driven by the `APP_ENV` variable. Includes settings for the dedicated SQLite logger (`SQLITE_LOG_ENABLED`, `SQLITE_LOG_LEVEL`).
* **Request Validation:** Declarative validation of incoming request data (JSON bodies, form data) using `github.com/go-playground/validator/v10`, with helper utilities in `internal/pkg/validation/`.
* **Modular Architecture:** Code is organized into distinct packages (`app`, `bootstrap`, `config`, `database`, `handlers`, `logging`, `middleware`, `models`, `pkg/validation`, `repositories`, `routes`, `services`, `utils`) within the `internal` directory to promote separation of concerns and maintainability.
* **Dependency Wiring:** A `bootstrap` package centralizes the initialization and wiring of application components.
* **Middleware:** Includes standard middleware for Recovery, CORS, Request Logging, and JWT Authentication (`Protected` routes).
* **Graceful Shutdown:** Handles OS interrupt signals (SIGINT, SIGTERM) to shut down the server cleanly.
* **Password Security:** Uses `bcrypt` for hashing user passwords.
* **File Uploads:** Supports user photo uploads (e.g., during registration) saved to a configurable directory.
* **Project Scaffolding:** Includes a shell script (`scripts/generate_project.sh`) to generate the initial project skeleton.

## Project Structure
```
go-webapi/
├── cmd/
│   └── api/
│       └── main.go           # Application entry point.
├── internal/
│   ├── app/
│   │   └── app.go            # Core application orchestration: Loads config, initializes loggers & DBs, calls bootstrap, sets up Fiber app & middleware, manages server lifecycle, logs custom startup banner.
│   ├── bootstrap/
│   │   └── bootstrap.go      # Dependency Injection / Wiring: Initializes repositories, services, handlers, and the log processor, using the configured loggers.
│   ├── config/
│   │   └── config.go         # Handles loading and parsing of application configuration, including settings for different loggers.
│   ├── database/
│   │   ├── oracle.go         # Function for initializing the Oracle DB connection.
│   │   └── sqlite.go         # Function for initializing the SQLite DB connection and creating the log table.
│   ├── handlers/
│   │   ├── auth_handler.go   # HTTP handlers for authentication, using request validation.
│   │   └── profile_handler.go# HTTP handlers for user profiles.
│   ├── logging/
│   │   ├── logger.go         # Sets up Zap logger configurations: a file/console logger and a dedicated SQLite logger. Implements custom sqliteCore.
│   │   └── processor.go      # Implements the LogProcessor to transfer logs from SQLite (written by sqliteLogger) to Oracle.
│   ├── middleware/
│   │   └── jwt_middleware.go # JWT authentication middleware.
│   ├── models/
│   │   ├── user.go           # User data model.
│   │   └── log.go            # Log data model.
│   ├── pkg/                  # Internal shared utility packages (library-like for this project).
│   │   └── validation/
│   │       └── validator.go  # Request validation utilities using go-playground/validator.
│   ├── repositories/
│   │   ├── user_repo.go      # DB interactions for users.
│   │   └── log_repo.go       # DB interactions for logs (SQLite write, SQLite read, Oracle batch insert).
│   ├── routes/
│   │   └── routes.go         # Defines HTTP routes, applies middleware, maps to handlers.
│   ├── services/
│   │   ├── auth_service.go   # Business logic for auth, may use sqliteLogger for specific audit events.
│   │   └── profile_service.go# Business logic for user profiles.
│   └── utils/                # General utility functions (e.g., password, jwt).
│       ├── password.go       # Password hashing utilities.
│       └── jwt.go            # JWT generation and validation utilities.
├── logs/
│   └── app.log               # Example log file output from fileLogger.
├── scripts/
│   └── generate_project.sh   # Shell script for project skeleton generation.
├── uploads/
│   └── .gitkeep
├── .env.example              # Example environment variables template.
├── .env.local                # Local development environment variables (ignored by git).
├── .env.prod                 # Production environment variables example (ignored by git).
├── .gitignore
├── go.mod
├── go.sum
└── README.md                 # This file.
```

## Setup and Running

1.  **Prerequisites:**
    * Go (version specified in `go.mod`)
    * Access to an Oracle database instance.
    * Git

2.  **Clone the repository:**
    ```bash
    git clone <your-repository-url>
    cd go-webapi
    ```

3.  **Configuration:**
    * Create a configuration file for your environment by copying the example:
        ```bash
        cp .env.example .env.local
        ```
        (Or use `.env.development`, `.env.production` depending on your `APP_ENV`).
    * Open your newly created `.env.local` (or equivalent) file and update the environment variables. Refer to `.env.example` for a list of required variables and their purpose. **Crucially, change `JWT_SECRET` to a strong, unique secret key.**
    * **Important:** Ensure the directories specified for `SQLITE_DB_PATH` (parent directory), `LOG_FILE_PATH` (parent directory), and `UPLOAD_DIR` exist, or that the application has the necessary permissions to create them during startup.

4.  **Install Dependencies:**
    ```bash
    go mod tidy
    ```

5.  **Run the application:**
    * If you haven't set `APP_ENV` in your `.env` file, you can set it in your shell (optional, defaults to `local` if not set anywhere):
        ```bash
        # Example for Linux/macOS
        export APP_ENV=local
        # Example for Windows (Command Prompt)
        # set APP_ENV=local
        # Example for Windows (PowerShell)
        # $env:APP_ENV="local"
        ```
    * Run:
        ```bash
        go run ./cmd/api/main.go
        ```
    The server will start, and startup information will be logged by the `fileLogger`.

## Logging Strategy

The application employs a dual-logger system using `go.uber.org/zap`:

1.  **File/Console Logger (`fileLogger`):**
    * Initialized at application startup.
    * Handles general operational logs: startup messages (custom banner), HTTP request logs (via `fiberzap` middleware), informational messages from services/handlers, errors, etc.
    * Outputs to both the console (for development visibility) and a rotating log file (for persistent operational logging), using `timberjack` for rotation.
    * Its log level is configured by `LOG_LEVEL` in the `.env` file.

2.  **Dedicated SQLite Logger (`sqliteLogger`):**
    * Also initialized at startup but can be enabled or disabled via `SQLITE_LOG_ENABLED` in the `.env` file.
    * If enabled, this logger writes *only* to a local SQLite database (`tbl_log`).
    * It is intended for specific, important log events that require persistence and potential auditing or transfer to a central system (Oracle). Examples include critical errors, security-relevant events (like failed logins if configured), or explicit audit trail messages.
    * Application code must explicitly call `sqliteLogger` to write to this destination.
    * Its log level is independently configured by `SQLITE_LOG_LEVEL`, allowing you to, for example, log only `WARN` and above to SQLite while the `fileLogger` might be set to `INFO` or `DEBUG`.
    * If disabled, calls to `sqliteLogger` will use a no-op logger, incurring minimal performance overhead.

3.  **Log Processing (`LogProcessor`):**
    * A background goroutine that periodically reads log batches from the SQLite `tbl_log` table.
    * Attempts to insert these batches into the configured Oracle `tbl_log` table.
    * If the Oracle insert succeeds, the corresponding logs are deleted from SQLite.
    * Includes retry mechanisms for Oracle connection issues.

This strategy allows for verbose operational logging to transient/file storage while ensuring that critical or auditable logs are captured reliably in SQLite for more durable storage and processing.

## Oracle Connection Handling

* The initial Oracle connection pool (`*sql.DB`) is created during application startup. The standard `database/sql` package handles internal connection pooling and basic retries.
* The `LogProcessor` specifically monitors the connection when attempting batch inserts to Oracle. If a connection-related error occurs, it has logic to attempt re-initialization of an Oracle DB handle and update the handle within the shared `LogRepository` instance.

## API Endpoints (Examples)

* `GET /health`: Checks service health and database connectivity.
* `POST /api/v1/auth/register`: Registers a new user. Validates input. Specific registration events might be logged to SQLite via `sqliteLogger`.
* `POST /api/v1/auth/login`: Logs in a user. Validates input. Failed login attempts might be logged to SQLite. Returns JWT.
* `GET /api/v1/profile`: Retrieves authenticated user's profile (Requires `Authorization: Bearer <token>`).
* `GET /uploads/*`: Serves static files stored in the upload directory.