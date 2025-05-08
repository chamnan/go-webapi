package middleware

import (
	"fmt"
	"regexp"
	"strings"
	"time" // Import time

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/gofiber/fiber/v2"
)

const maxBodyLogSize = 1024 // Limit body size logged (e.g., 1KB)

// RequestDebugLogger logs detailed request information (headers, body) if the logger level is Debug,
// and also logs response status and latency after the request is handled.
func RequestDebugLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get the request-scoped file logger
		logger := GetRequestFileLogger(c)

		// Record start time *before* checking the log level,
		// as we might want to log latency even if request details are skipped.
		startTime := time.Now()

		// Log Request Details only if Debug is enabled
		if logger.Core().Enabled(zapcore.DebugLevel) {
			headersMap := make(map[string]string)
			c.Request().Header.VisitAll(func(key, value []byte) {
				headerKey := string(key)
				if headerKey == "Authorization" || headerKey == "Cookie" {
					headersMap[headerKey] = "*** HIDDEN ***"
				} else {
					headersMap[headerKey] = string(value)
				}
			})

			var bodyBytes []byte
			var bodyLog string
			contentType := string(c.Request().Header.ContentType())

			if len(c.BodyRaw()) > 0 && (strings.Contains(contentType, "json") || strings.Contains(contentType, "xml") || strings.Contains(contentType, "text") || strings.Contains(contentType, "form")) {
				bodyBytes = c.BodyRaw()
				if len(bodyBytes) > maxBodyLogSize {
					bodyLog = string(bodyBytes[:maxBodyLogSize]) + "... (truncated)"
				} else {
					bodyLog = string(bodyBytes)
				}
				// bodyLog = sanitizeSensitiveData(bodyLog) // Placeholder
			} else if len(c.BodyRaw()) > 0 {
				bodyLog = "(Binary or non-text body, size: " + fmt.Sprintf("%d", len(c.BodyRaw())) + " bytes)"
			} else {
				bodyLog = "(Empty Body)"
			}

			logger.Debug("Incoming Request Details",
				zap.String("method", c.Method()),
				zap.String("path", c.Path()),
				zap.String("ip", c.IP()),
				zap.Any("headers", headersMap),
				zap.String("body", bodyLog),
			)
		}

		// Continue to next middleware/handler
		err := c.Next()

		// Calculate latency AFTER the request has been handled by c.Next()
		latency := time.Since(startTime)

		// Log response status and latency (consider logging this at Info level, or keep at Debug)
		// Using Debug here to keep it consistent with the request details logging level.
		logger.Debug("Request Handled",
			zap.Int("status", c.Response().StatusCode()),
			zap.Duration("latency", latency),
			// Optionally add response headers or truncated response body here if needed
			// zap.String("response_body", getTruncatedResponseBody(c)),
		)

		return err
	}
}

// Helper function placeholder for sanitizing sensitive data (implement as needed)
func sanitizeSensitiveData(body string) string {
	// Example: Replace password fields
	re := regexp.MustCompile(`("password"\s*:\s*")[^"]*(")`)
	return re.ReplaceAllString(body, `$1***$2`)
}

// Optional helper to get response body safely (be careful with large responses)
func getTruncatedResponseBody(c *fiber.Ctx) string {
	respBody := c.Response().Body()
	if len(respBody) == 0 {
		return "(Empty Response Body)"
	}
	if len(respBody) > maxBodyLogSize {
		return string(respBody[:maxBodyLogSize]) + "... (truncated)"
	}
	// Potentially sanitize response body too
	return string(respBody)
}
