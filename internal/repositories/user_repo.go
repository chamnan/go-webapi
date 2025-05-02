
package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go-webapi/internal/models"
	"go.uber.org/zap"
)

// UserRepository defines the interface for user data operations
type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (*models.User, error)
	FindByID(ctx context.Context, id int64) (*models.User, error)
	CreateUser(ctx context.Context, user *models.User) (int64, error) // Returns the new user ID
	// Add other methods like UpdateUser, DeleteUser etc. if needed
}

// oracleUserRepository implements UserRepository for Oracle
type oracleUserRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewOracleUserRepository creates a new UserRepository for Oracle
func NewOracleUserRepository(db *sql.DB, logger *zap.Logger) UserRepository {
	return &oracleUserRepository{db: db, logger: logger}
}

// FindByUsername retrieves a user by their username from Oracle tbl_user
func (r *oracleUserRepository) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `SELECT user_id, username, password_hash, photo_path, created_at, updated_at FROM tbl_user WHERE username = :1` // Adjust column names as needed
	user := &models.User{}
	var createdAt sql.NullTime // Handle potential NULLs if columns allow
	var updatedAt sql.NullTime // Handle potential NULLs if columns allow
    var photoPath sql.NullString

	r.logger.Debug("Executing FindByUsername query", zap.String("query", query), zap.String("username", username)) // Optional Debug logging

	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
        &photoPath, // Scan into NullString first
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.logger.Warn("User not found by username", zap.String("username", username))
			return nil, nil // Return nil, nil to indicate not found cleanly
		}
		r.logger.Error("Error querying user by username", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("error finding user by username %s: %w", username, err)
	}

	// Assign values from Null types if they are valid
	if createdAt.Valid {
		user.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		user.UpdatedAt = updatedAt.Time
	}
    if photoPath.Valid {
        user.PhotoPath = photoPath.String
    }


	return user, nil
}

// FindByID retrieves a user by their ID from Oracle tbl_user
func (r *oracleUserRepository) FindByID(ctx context.Context, id int64) (*models.User, error) {
    // Similar implementation to FindByUsername, but query by user_id
	query := `SELECT user_id, username, password_hash, photo_path, created_at, updated_at FROM tbl_user WHERE user_id = :1`
	user := &models.User{}
    var createdAt sql.NullTime
	var updatedAt sql.NullTime
    var photoPath sql.NullString

    r.logger.Debug("Executing FindByID query", zap.String("query", query), zap.Int64("id", id))

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
        &photoPath,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.logger.Warn("User not found by ID", zap.Int64("id", id))
			return nil, nil // Not found
		}
		r.logger.Error("Error querying user by ID", zap.Int64("id", id), zap.Error(err))
		return nil, fmt.Errorf("error finding user by ID %d: %w", id, err)
	}

    if createdAt.Valid { user.CreatedAt = createdAt.Time }
	if updatedAt.Valid { user.UpdatedAt = updatedAt.Time }
    if photoPath.Valid { user.PhotoPath = photoPath.String }

	return user, nil
}

// CreateUser inserts a new user into the Oracle tbl_user
func (r *oracleUserRepository) CreateUser(ctx context.Context, user *models.User) (int64, error) {
	// Note: Oracle often uses sequences for IDs. Adjust INSERT accordingly.
	// This example assumes user_id is auto-generated or managed by a trigger/sequence.
	// We use RETURNING INTO clause to get the ID back.
	query := `
        INSERT INTO tbl_user (username, password_hash, photo_path, created_at, updated_at)
        VALUES (:1, :2, :3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        RETURNING user_id INTO :4` // Adjust column names and returning clause for your schema

    var newID int64
    photoPath := sql.NullString{String: user.PhotoPath, Valid: user.PhotoPath != ""}


	r.logger.Debug("Executing CreateUser query", zap.String("query", "INSERT INTO tbl_user..."), zap.String("username", user.Username))

	_, err := r.db.ExecContext(ctx, query,
		user.Username,
		user.PasswordHash,
		photoPath, // Pass NullString
        sql.Out{Dest: &newID}, // Oracle specific way to get returned value
	)
	if err != nil {
		// TODO: Handle potential unique constraint violations (e.g., duplicate username)
		r.logger.Error("Error creating user", zap.String("username", user.Username), zap.Error(err))
		return 0, fmt.Errorf("error creating user %s: %w", user.Username, err)
	}

    user.ID = newID // Set the ID on the passed-in user object
	r.logger.Info("User created successfully", zap.String("username", user.Username), zap.Int64("newID", newID))
	return newID, nil
}


