
package middleware

import (
	"strings"
	"go-webapi/internal/utils"

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

