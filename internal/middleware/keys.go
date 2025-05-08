package middleware

// ContextKey is a type for context keys to avoid collisions.
type ContextKey string

// Constants for middleware keys and values
const (
	// --- Logger Keys ---
	RequestFileLoggerKey   ContextKey = "requestFileLogger"
	RequestSQLiteLoggerKey ContextKey = "requestSQLiteLogger"
	RequestIDHeader                   = "X-Request-ID" // Header name

	// --- JWT Middleware Keys ---
	AuthorizationHeader            = "Authorization"
	BearerPrefix                   = "Bearer "
	UserIDKey           ContextKey = "userID"

	// --- Request ID Key ---
	RequestIDKey ContextKey = "requestID" // Key to store the request ID string in Locals
)
