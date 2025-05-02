
package models

import "time"

// LogEntry represents a log record to be stored temporarily in SQLite
// and then transferred to Oracle tbl_log
type LogEntry struct {
	ID        int64     `json:"-"`                // SQLite Row ID
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Fields    string    `json:"fields,omitempty"` // JSON representation of extra fields
	// Add other fields matching your Oracle tbl_log structure if needed
	// e.g., RequestID, UserID, IPAddress, etc.
}


