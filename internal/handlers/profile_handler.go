
package handlers

import (
	"go-webapi/internal/middleware"
	"go-webapi/internal/services"

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

