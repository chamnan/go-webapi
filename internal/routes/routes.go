package routes

import (
	"context"
	"database/sql"
	"time"

	"go-webapi/internal/bootstrap"
	"go-webapi/internal/config"
	"go-webapi/internal/logging" // For logging.GetFileLogger() if needed inside route handlers
	mw "go-webapi/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// SetupRoutes configures the application routes.
func SetupRoutes(
	app *fiber.App,
	cfg *config.Config,
	fileLogger *zap.Logger, // Renamed from logger
	components *bootstrap.AppComponents,
	sqliteDB *sql.DB,
	oracleDB *sql.DB,
) {
	fileLogger.Info("Setting up application routes using components...")

	// --- Public Routes ---
	app.Get("/health", func(c *fiber.Ctx) error {
		lg := logging.GetFileLogger() // Use the global file logger for health check logs
		healthStatus := fiber.Map{"status": "healthy", "timestamp": time.Now().UTC()}
		dbStatus := fiber.Map{}
		if sqliteDB != nil {
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
		fileLogger.Info("Serving static files", zap.String("path", "/uploads"), zap.String("directory", cfg.UploadDir))
	} else {
		fileLogger.Warn("Upload directory not configured, skipping static file route setup.")
	}

	// --- API v1 Routes ---
	api := app.Group("/api/v1")

	// Authentication Routes (Public within API group)
	components.AuthHandler.SetupAuthRoutes(api)

	// Protected Routes (Requires JWT Authentication)
	// Pass the fileLogger to the JWT middleware for its operational logs
	protected := api.Group("/", mw.Protected(cfg.JWTSecret, fileLogger))

	// Profile Routes (Protected)
	components.ProfileHandler.SetupProfileRoutes(protected)
}
