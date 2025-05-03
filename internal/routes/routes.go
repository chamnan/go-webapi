package routes

import (
	"context"
	"database/sql"
	"time"

	"go-webapi/internal/bootstrap"
	"go-webapi/internal/config"
	"go-webapi/internal/logging"
	mw "go-webapi/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// SetupRoutes configures the application routes.
// Accepts the main AppComponents struct containing handlers.
func SetupRoutes(
	app *fiber.App,
	cfg *config.Config,
	logger *zap.Logger,
	components *bootstrap.AppComponents, // <-- MODIFIED: Accept components struct
	sqliteDB *sql.DB,
	oracleDB *sql.DB,
) {
	logger.Info("Setting up application routes using components...")

	// --- Public Routes ---

	// Health Check
	app.Get("/health", func(c *fiber.Ctx) error {
		// ... (health check logic remains the same) ...
		lg := logging.GetLogger()
		healthStatus := fiber.Map{"status": "healthy", "timestamp": time.Now().UTC()}
		dbStatus := fiber.Map{}
		if sqliteDB != nil {
			// Use PingContext with a timeout for potentially slow pings
			pingCtx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
			defer cancel()
			if err := sqliteDB.PingContext(pingCtx); err == nil {
				dbStatus["sqlite"] = "connected"
			} else {
				dbStatus["sqlite"] = "disconnected"
				lg.Warn("Health check: SQLite ping failed", zap.Error(err))
			}
		} else {
			dbStatus["sqlite"] = "uninitialized"
		}
		if oracleDB != nil {
			pingCtx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
			defer cancel()
			if err := oracleDB.PingContext(pingCtx); err == nil {
				dbStatus["oracle"] = "connected"
			} else {
				dbStatus["oracle"] = "disconnected"
				lg.Warn("Health check: Oracle ping failed", zap.Error(err))
			}
			// cancel() // Not needed due to defer
		} else {
			dbStatus["oracle"] = "uninitialized"
		}
		healthStatus["dependencies"] = dbStatus
		return c.Status(fiber.StatusOK).JSON(healthStatus)

	})

	// Static File Server for Uploads
	if cfg.UploadDir != "" {
		app.Static("/uploads", cfg.UploadDir, fiber.Static{
			Compress:  true,
			ByteRange: true,
			Browse:    cfg.AppEnv != "production",
		})
		logger.Info("Serving static files", zap.String("path", "/uploads"), zap.String("directory", cfg.UploadDir))
	} else {
		logger.Warn("Upload directory not configured, skipping static file route setup.")
	}

	// --- API v1 Routes ---
	api := app.Group("/api/v1")

	// Authentication Routes (Public within API group)
	// Use the AuthHandler from the components struct
	components.AuthHandler.SetupAuthRoutes(api) // <-- MODIFIED: Use components.AuthHandler

	// Protected Routes (Requires JWT Authentication)
	protected := api.Group("/", mw.Protected(cfg.JWTSecret, logger))

	// Profile Routes (Protected)
	// Use the ProfileHandler from the components struct
	components.ProfileHandler.SetupProfileRoutes(protected) // <-- MODIFIED: Use components.ProfileHandler

	// Add other route groups/handlers using components here...
	// Example: components.SomeOtherHandler.SetupRoutes(api)
}
