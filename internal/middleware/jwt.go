package middleware

import (
	"go-webapi/internal/utils"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	// No longer need to import logging package here if only using GetRequestFileLogger
)

// Protected returns a Fiber middleware function that checks for a valid JWT.
// It no longer accepts a logger; it retrieves it from the context.
func Protected(jwtSecret string) fiber.Handler { // Removed logger parameter
	return func(c *fiber.Ctx) error {
		// Get the request-scoped logger from context
		logger := GetRequestFileLogger(c)

		authHeader := c.Get(AuthorizationHeader)

		if authHeader == "" {
			logger.Warn("Missing Authorization header") // Now logs with request_id
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing authorization header",
			})
		}

		if !strings.HasPrefix(authHeader, BearerPrefix) {
			logger.Warn("Invalid Authorization header format", zap.String("header_prefix", authHeader[:len(BearerPrefix)])) // Log prefix only
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid authorization format (Bearer token required)",
			})
		}

		tokenString := strings.TrimPrefix(authHeader, BearerPrefix)
		if tokenString == "" {
			logger.Warn("Empty token string after Bearer prefix") // Now logs with request_id
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing token",
			})
		}

		claims, err := utils.ValidateToken(tokenString, jwtSecret)
		if err != nil {
			// Be careful logging tokens, log only the error maybe
			logger.Warn("Invalid JWT token", zap.Error(err)) // Now logs with request_id
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}

		// Token is valid, store user ID in context for downstream handlers
		c.Locals(UserIDKey, claims.UserID)
		logger.Debug("JWT validated successfully", zap.Int64("userID", claims.UserID)) // Now logs with request_id

		// Proceed to the next handler
		return c.Next()
	}
}
