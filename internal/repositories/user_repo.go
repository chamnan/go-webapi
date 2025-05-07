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
	CreateUser(ctx context.Context, user *models.User) (int64, error)
}

// oracleUserRepository implements UserRepository for Oracle
type oracleUserRepository struct {
	db         *sql.DB
	fileLogger *zap.Logger // Renamed from logger
}

// NewOracleUserRepository creates a new UserRepository for Oracle
func NewOracleUserRepository(db *sql.DB, fileLogger *zap.Logger) UserRepository {
	return &oracleUserRepository{db: db, fileLogger: fileLogger}
}

// FindByUsername retrieves a user by their username from Oracle tbl_user
func (r *oracleUserRepository) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `SELECT user_id, username, password_hash, photo_path, created_at, updated_at FROM tbl_user WHERE username = :1`
	user := &models.User{}
	var createdAt sql.NullTime
	var updatedAt sql.NullTime
	var photoPath sql.NullString

	r.fileLogger.Debug("Executing FindByUsername query", zap.String("query", query), zap.String("username", username))

	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&photoPath,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.fileLogger.Warn("User not found by username", zap.String("username", username))
			return nil, nil
		}
		r.fileLogger.Error("Error querying user by username", zap.String("username", username), zap.Error(err))
		return nil, fmt.Errorf("error finding user by username %s: %w", username, err)
	}

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
	query := `SELECT user_id, username, password_hash, photo_path, created_at, updated_at FROM tbl_user WHERE user_id = :1`
	user := &models.User{}
	var createdAt sql.NullTime
	var updatedAt sql.NullTime
	var photoPath sql.NullString

	r.fileLogger.Debug("Executing FindByID query", zap.String("query", query), zap.Int64("id", id))

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
			r.fileLogger.Warn("User not found by ID", zap.Int64("id", id))
			return nil, nil
		}
		r.fileLogger.Error("Error querying user by ID", zap.Int64("id", id), zap.Error(err))
		return nil, fmt.Errorf("error finding user by ID %d: %w", id, err)
	}

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

// CreateUser inserts a new user into the Oracle tbl_user
func (r *oracleUserRepository) CreateUser(ctx context.Context, user *models.User) (int64, error) {
	query := `
        INSERT INTO tbl_user (username, password_hash, photo_path, created_at, updated_at)
        VALUES (:1, :2, :3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        RETURNING user_id INTO :4`

	var newID int64
	photoPathNull := sql.NullString{String: user.PhotoPath, Valid: user.PhotoPath != ""} // Renamed variable

	r.fileLogger.Debug("Executing CreateUser query", zap.String("query", "INSERT INTO tbl_user..."), zap.String("username", user.Username))

	_, err := r.db.ExecContext(ctx, query,
		user.Username,
		user.PasswordHash,
		photoPathNull, // Use renamed variable
		sql.Out{Dest: &newID},
	)
	if err != nil {
		r.fileLogger.Error("Error creating user", zap.String("username", user.Username), zap.Error(err))
		return 0, fmt.Errorf("error creating user %s: %w", user.Username, err)
	}

	user.ID = newID
	r.fileLogger.Info("User created successfully", zap.String("username", user.Username), zap.Int64("newID", newID))
	return newID, nil
}
