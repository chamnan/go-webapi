package services

import (
	"context"
	"errors"
	"fmt"
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
	Register(ctx context.Context, username, password, photoPath string) error
	Login(ctx context.Context, username, password string) (string, error) // Returns JWT token
	UpdateRepository(repo repositories.UserRepository)
}

type authServiceImpl struct {
	userRepo     repositories.UserRepository
	fileLogger   *zap.Logger // Renamed from mainLogger
	sqliteLogger *zap.Logger // Dedicated SQLite logger
	jwtSecret    string
	jwtExpires   time.Duration
}

// NewAuthService creates a new AuthService
func NewAuthService(userRepo repositories.UserRepository, fileLogger *zap.Logger, sqliteLogger *zap.Logger, jwtSecret string) AuthService {
	return &authServiceImpl{
		userRepo:     userRepo,
		fileLogger:   fileLogger,
		sqliteLogger: sqliteLogger, // Can be a Nop logger if disabled
		jwtSecret:    jwtSecret,
		jwtExpires:   24 * time.Hour,
	}
}

// Register handles new user registration
func (s *authServiceImpl) Register(ctx context.Context, username, password, photoPath string) error {
	s.fileLogger.Info("Attempting to register user", zap.String("username", username))

	// Check if username already exists
	existingUser, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		s.fileLogger.Error("Error checking for existing username", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}
	if existingUser != nil {
		s.fileLogger.Warn("Registration attempt failed: username already exists", zap.String("username", username))
		return ErrUsernameExists
	}

	// Hash the password
	hashedPassword, err := utils.HashPassword(password)
	if err != nil {
		s.fileLogger.Error("Failed to hash password during registration", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}

	newUser := &models.User{
		Username:     username,
		PasswordHash: hashedPassword,
		PhotoPath:    photoPath,
	}

	_, err = s.userRepo.CreateUser(ctx, newUser)
	if err != nil {
		s.fileLogger.Error("Failed to create user in database", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}

	s.fileLogger.Info("User registered successfully", zap.String("username", username), zap.Int64("userID", newUser.ID))

	// Example of using the dedicated SQLite logger for an audit/important event
	s.sqliteLogger.Info("New user registration for audit",
		zap.String("username", username),
		zap.Int64("new_user_id", newUser.ID),
	)

	return nil
}

// Login handles user login and JWT generation
func (s *authServiceImpl) Login(ctx context.Context, username, password string) (string, error) {
	s.fileLogger.Info("Attempting to login user", zap.String("username", username))

	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		s.fileLogger.Error("Error finding user during login", zap.String("username", username), zap.Error(err))
		return "", ErrInvalidCredentials
	}
	if user == nil {
		s.fileLogger.Warn("Login attempt failed: user not found", zap.String("username", username))
		return "", ErrUserNotFound
	}

	if !utils.CheckPasswordHash(password, user.PasswordHash) {
		s.fileLogger.Warn("Login attempt failed: invalid password", zap.String("username", username))
		s.sqliteLogger.Warn("Failed login attempt (invalid password)",
			zap.String("username", username),
			// zap.String("ip_address", c.IP()), // Assuming you have access to IP, e.g. by passing fiber.Ctx
		)
		return "", ErrInvalidCredentials
	}

	token, err := utils.GenerateToken(user.ID, s.jwtSecret, s.jwtExpires)
	if err != nil {
		s.fileLogger.Error("Failed to generate JWT token during login", zap.String("username", username), zap.Int64("userID", user.ID), zap.Error(err))
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	s.fileLogger.Info("User logged in successfully", zap.String("username", username), zap.Int64("userID", user.ID))
	return token, nil
}

// UpdateRepository updates the underlying user repository
func (s *authServiceImpl) UpdateRepository(repo repositories.UserRepository) {
	s.fileLogger.Info("AuthService: Repository updated dynamically.")
	s.userRepo = repo
}
