package routes

import (
	"context"
	"database/sql"
	"time"

	"go-webapi/internal/config"
	"go-webapi/internal/handlers"
	"go-webapi/internal/logging"
	mw "go-webapi/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// SetupRoutes configures the application routes.
func SetupRoutes(
	app *fiber.App,
	cfg *config.Config,
	logger *zap.Logger,
	authHandler *handlers.AuthHandler,
	profileHandler *handlers.ProfileHandler,
	sqliteDB *sql.DB, // Pass DB handles for health check
	oracleDB *sql.DB, // Pass DB handles for health check
) {
	logger.Info("Setting up application routes...")

	// --- Public Routes ---

	// Health Check
	app.Get("/health", func(c *fiber.Ctx) error {
		lg := logging.GetLogger() // Use GetLogger for consistency
		healthStatus := fiber.Map{"status": "healthy", "timestamp": time.Now().UTC()}
		dbStatus := fiber.Map{}

		if sqliteDB != nil {
			if err := sqliteDB.PingContext(c.Context()); err == nil {
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
			cancel() // Explicitly cancel context after use
		} else {
			dbStatus["oracle"] = "uninitialized"
		}
		healthStatus["dependencies"] = dbStatus
		return c.Status(fiber.StatusOK).JSON(healthStatus)
	})

	// Static File Server for Uploads
	// Ensure cfg.UploadDir is correctly passed and accessible
	if cfg.UploadDir != "" {
		app.Static("/uploads", cfg.UploadDir, fiber.Static{
			Compress:  true,
			ByteRange: true,
			Browse:    cfg.AppEnv != "production", // Enable directory Browse in non-prod
		})
		logger.Info("Serving static files", zap.String("path", "/uploads"), zap.String("directory", cfg.UploadDir))
	} else {
		logger.Warn("Upload directory not configured, skipping static file route setup.")
	}

	// --- API v1 Routes ---
	api := app.Group("/api/v1")

	// Authentication Routes (Public within API group)
	// The SetupAuthRoutes function groups routes under /auth
	authHandler.SetupAuthRoutes(api) // Example: POST /api/v1/auth/login, POST /api/v1/auth/register

	// Protected Routes (Requires JWT Authentication)
	// Pass logger to JWT middleware
	protected := api.Group("/", mw.Protected(cfg.JWTSecret, logger)) // Middleware applied to this group

	// Profile Routes (Protected)
	// The SetupProfileRoutes function groups routes under /profile (relative to 'protected')
	profileHandler.SetupProfileRoutes(protected) // Example: GET /api/v1/profile
}
