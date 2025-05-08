package middleware

import (
	"go-webapi/internal/logging" // To get the base loggers

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RequestLoggers is a middleware that injects request-scoped loggers into c.Locals().
// These loggers include a unique "request_id" field.
// It creates scoped versions of both the file/console logger and the SQLite logger.
// It also stores the request_id string in Locals.
func RequestLoggers(baseFileLogger, baseSQLiteLogger *zap.Logger) fiber.Handler {
	if baseFileLogger == nil {
		baseFileLogger = zap.NewNop()
	}
	if baseSQLiteLogger == nil {
		baseSQLiteLogger = zap.NewNop()
	}

	return func(c *fiber.Ctx) error {
		// Generate a unique request ID
		requestID := uuid.NewString()

		// Add request_id to response headers for client-side correlation
		c.Set(RequestIDHeader, requestID)

		// Store the request ID string directly in Locals
		c.Locals(RequestIDKey, requestID) // <-- STORED HERE

		// Create request-scoped file/console logger
		reqFileLogger := baseFileLogger.With(
			zap.String("request_id", requestID),
		)
		c.Locals(RequestFileLoggerKey, reqFileLogger)

		// Create request-scoped SQLite logger
		reqSQLiteLogger := baseSQLiteLogger.With(
			zap.String("request_id", requestID),
		)
		c.Locals(RequestSQLiteLoggerKey, reqSQLiteLogger)

		return c.Next()
	}
}

// GetRequestFileLogger retrieves the request-scoped file/console logger from fiber.Ctx.Locals.
// Falls back to the global file logger if not found.
func GetRequestFileLogger(c *fiber.Ctx) *zap.Logger {
	if logger, ok := c.Locals(RequestFileLoggerKey).(*zap.Logger); ok && logger != nil {
		return logger
	}
	return logging.GetFileLogger()
}

// GetRequestSQLiteLogger retrieves the request-scoped SQLite logger from fiber.Ctx.Locals.
// Falls back to the global SQLite logger (which might be Nop).
func GetRequestSQLiteLogger(c *fiber.Ctx) *zap.Logger {
	if logger, ok := c.Locals(RequestSQLiteLoggerKey).(*zap.Logger); ok && logger != nil {
		return logger
	}
	return logging.GetSQLiteLogger()
}

// GetRequestID retrieves the request ID string from fiber.Ctx.Locals.
// Returns an empty string if not found.
func GetRequestID(c *fiber.Ctx) string {
	if reqID, ok := c.Locals(RequestIDKey).(string); ok {
		return reqID
	}
	// Fallback: try getting from header if needed, though locals should be primary
	// headerReqID := c.Get(RequestIDHeader)
	// return headerReqID
	return ""
}
