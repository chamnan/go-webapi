package services

import (
	"context"
	"errors"
	"fmt"
	"go-webapi/internal/logging" // For fallback logger if needed
	"time"

	"go-webapi/internal/models"
	"go-webapi/internal/repositories"
	"go-webapi/internal/utils"
	"go.uber.org/zap"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUsernameExists     = errors.New("username already exists")
	ErrRegistrationFailed = errors.New("failed to register user")
)

// AuthService defines the interface for authentication related operations
type AuthService interface {
	// Methods now accept request-scoped loggers
	Register(ctx context.Context, reqFileLogger, reqSQLiteLogger *zap.Logger, username, password, photoPath string) error
	Login(ctx context.Context, reqFileLogger, reqSQLiteLogger *zap.Logger, username, password string) (string, error)
	UpdateRepository(repo repositories.UserRepository)
}

type authServiceImpl struct {
	userRepo     repositories.UserRepository
	sqliteLogger *zap.Logger // Base SQLite logger for potential non-request specific use or fallback
	jwtSecret    string
	jwtExpires   time.Duration
}

// NewAuthService creates a new AuthService
func NewAuthService(userRepo repositories.UserRepository, _ *zap.Logger /* baseFileLogger */, sqliteLogger *zap.Logger, jwtSecret string) AuthService {
	return &authServiceImpl{
		userRepo:     userRepo,
		sqliteLogger: sqliteLogger, // Store base SQLite logger
		jwtSecret:    jwtSecret,
		jwtExpires:   24 * time.Hour,
	}
}

// Register handles new user registration
// Accepts request-scoped fileLogger and sqliteLogger
func (s *authServiceImpl) Register(ctx context.Context, reqFileLogger, reqSQLiteLogger *zap.Logger, username, password, photoPath string) error {
	// Ensure fallbacks if necessary (though middleware should guarantee non-nil scoped loggers usually)
	if reqFileLogger == nil {
		reqFileLogger = logging.GetFileLogger()
	}
	if reqSQLiteLogger == nil {
		reqSQLiteLogger = logging.GetSQLiteLogger() // This might be Nop logger
	}

	reqFileLogger.Info("Attempting to register user", zap.String("username", username))

	existingUser, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		reqFileLogger.Error("Error checking for existing username", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}
	if existingUser != nil {
		reqFileLogger.Warn("Registration attempt failed: username already exists", zap.String("username", username))
		return ErrUsernameExists
	}

	hashedPassword, err := utils.HashPassword(password)
	if err != nil {
		reqFileLogger.Error("Failed to hash password during registration", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}

	newUser := &models.User{
		Username:     username,
		PasswordHash: hashedPassword,
		PhotoPath:    photoPath,
	}

	_, err = s.userRepo.CreateUser(ctx, newUser)
	if err != nil {
		reqFileLogger.Error("Failed to create user in database", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}

	reqFileLogger.Info("User registered successfully", zap.String("username", username), zap.Int64("userID", newUser.ID))

	// Use the request-scoped SQLite logger for the audit event
	reqSQLiteLogger.Info("New user registration for audit",
		zap.String("username", username),
		zap.Int64("new_user_id", newUser.ID),
	)

	return nil
}

// Login handles user login and JWT generation
// Accepts request-scoped fileLogger and sqliteLogger
func (s *authServiceImpl) Login(ctx context.Context, reqFileLogger, reqSQLiteLogger *zap.Logger, username, password string) (string, error) {
	if reqFileLogger == nil {
		reqFileLogger = logging.GetFileLogger()
	}
	if reqSQLiteLogger == nil {
		reqSQLiteLogger = logging.GetSQLiteLogger()
	}

	reqFileLogger.Info("Attempting to login user", zap.String("username", username))

	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		reqFileLogger.Error("Error finding user during login", zap.String("username", username), zap.Error(err))
		return "", ErrInvalidCredentials
	}
	if user == nil {
		reqFileLogger.Warn("Login attempt failed: user not found", zap.String("username", username))
		return "", ErrUserNotFound
	}

	if !utils.CheckPasswordHash(password, user.PasswordHash) {
		reqFileLogger.Warn("Login attempt failed: invalid password", zap.String("username", username))
		// Log failed login attempt to SQLite using the request-scoped SQLite logger
		reqSQLiteLogger.Warn("Failed login attempt (invalid password)",
			zap.String("username", username),
			// Add other relevant context if available (e.g., IP address)
			// zap.String("ip_address", "..."),
		)
		return "", ErrInvalidCredentials
	}

	token, err := utils.GenerateToken(user.ID, s.jwtSecret, s.jwtExpires)
	if err != nil {
		reqFileLogger.Error("Failed to generate JWT token during login", zap.String("username", username), zap.Int64("userID", user.ID), zap.Error(err))
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	reqFileLogger.Info("User logged in successfully", zap.String("username", username), zap.Int64("userID", user.ID))
	return token, nil
}

// UpdateRepository updates the underlying user repository
func (s *authServiceImpl) UpdateRepository(repo repositories.UserRepository) {
	// Use the global/base logger for non-request specific logs like this one
	logging.GetFileLogger().Info("AuthService: Repository updated dynamically.")
	s.userRepo = repo
}
