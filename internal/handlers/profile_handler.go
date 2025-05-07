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
	fileLogger     *zap.Logger // Renamed from logger
}

// NewProfileHandler creates a new ProfileHandler
func NewProfileHandler(profileService services.ProfileService, fileLogger *zap.Logger) *ProfileHandler {
	return &ProfileHandler{
		profileService: profileService,
		fileLogger:     fileLogger,
	}
}

// GetProfile handles GET /api/profile requests
func (h *ProfileHandler) GetProfile(c *fiber.Ctx) error {
	userIDVal := c.Locals(middleware.UserIDKey)
	userID, ok := userIDVal.(int64)
	if !ok || userID == 0 {
		h.fileLogger.Error("User ID not found or invalid in context/locals after JWT validation", zap.Any("value", userIDVal))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized: User ID missing from token context",
		})
	}

	h.fileLogger.Debug("Handling GetProfile request", zap.Int64("userID", userID))

	profile, err := h.profileService.GetProfile(c.Context(), userID)
	if err != nil {
		switch err {
		case services.ErrUserNotFound:
			h.fileLogger.Warn("Attempted to get profile for non-existent user confirmed by service", zap.Int64("userID", userID))
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Profile not found"})
		default:
			h.fileLogger.Error("Failed to get profile", zap.Int64("userID", userID), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve profile"})
		}
	}
	return c.Status(fiber.StatusOK).JSON(profile)
}

// SetupProfileRoutes registers profile routes with the Fiber app (protected)
func (h *ProfileHandler) SetupProfileRoutes(router fiber.Router) {
	router.Get("/profile", h.GetProfile)
}
