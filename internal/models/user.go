
package models

import "time"

// User represents the structure of the tbl_user table
type User struct {
	ID          int64     `json:"id"`              // Assuming ID is a number
	Username    string    `json:"username"`
	PasswordHash string   `json:"-"` // Exclude password hash from JSON responses
	PhotoPath   string    `json:"photo_path,omitempty"`
	CreatedAt   time.Time `json:"created_at"`      // Optional: Audit column
	UpdatedAt   time.Time `json:"updated_at"`      // Optional: Audit column
}

