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
	UpdateRepository(repo repositories.UserRepository)                    // ✅ Add this line
}

type authServiceImpl struct {
	userRepo   repositories.UserRepository
	logger     *zap.Logger
	jwtSecret  string
	jwtExpires time.Duration
}

// NewAuthService creates a new AuthService
func NewAuthService(userRepo repositories.UserRepository, logger *zap.Logger, jwtSecret string) AuthService {
	return &authServiceImpl{
		userRepo:   userRepo,
		logger:     logger,
		jwtSecret:  jwtSecret,
		jwtExpires: 24 * time.Hour, // Default JWT expiration (make configurable?)
	}
}

// Register handles new user registration
func (s *authServiceImpl) Register(ctx context.Context, username, password, photoPath string) error {
	s.logger.Info("Attempting to register user", zap.String("username", username))

	// Check if username already exists
	existingUser, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		// Log the underlying DB error but return a generic registration error
		s.logger.Error("Error checking for existing username", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}
	if existingUser != nil {
		s.logger.Warn("Registration attempt failed: username already exists", zap.String("username", username))
		return ErrUsernameExists
	}

	// Hash the password
	hashedPassword, err := utils.HashPassword(password)
	if err != nil {
		s.logger.Error("Failed to hash password during registration", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}

	// Create the user model
	newUser := &models.User{
		Username:     username,
		PasswordHash: hashedPassword,
		PhotoPath:    photoPath,
		// CreatedAt/UpdatedAt are usually handled by the database (or repo)
	}

	// Save the user to the database
	_, err = s.userRepo.CreateUser(ctx, newUser)
	if err != nil {
		s.logger.Error("Failed to create user in database", zap.String("username", username), zap.Error(err))
		return ErrRegistrationFailed
	}

	s.logger.Info("User registered successfully", zap.String("username", username), zap.Int64("userID", newUser.ID))
	return nil
}

// Login handles user login and JWT generation
func (s *authServiceImpl) Login(ctx context.Context, username, password string) (string, error) {
	s.logger.Info("Attempting to login user", zap.String("username", username))

	// Find the user by username
	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		s.logger.Error("Error finding user during login", zap.String("username", username), zap.Error(err))
		return "", ErrInvalidCredentials // Generic error even if DB error
	}
	if user == nil {
		s.logger.Warn("Login attempt failed: user not found", zap.String("username", username))
		return "", ErrUserNotFound // Or ErrInvalidCredentials for security
	}

	// Check the password
	if !utils.CheckPasswordHash(password, user.PasswordHash) {
		s.logger.Warn("Login attempt failed: invalid password", zap.String("username", username))
		return "", ErrInvalidCredentials
	}

	// Generate JWT token
	token, err := utils.GenerateToken(user.ID, s.jwtSecret, s.jwtExpires)
	if err != nil {
		s.logger.Error("Failed to generate JWT token during login", zap.String("username", username), zap.Int64("userID", user.ID), zap.Error(err))
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	s.logger.Info("User logged in successfully", zap.String("username", username), zap.Int64("userID", user.ID))
	return token, nil
}

// UpdateRepository updates the underlying user repository (for dynamic Oracle injection)
func (s *authServiceImpl) UpdateRepository(repo repositories.UserRepository) {
	s.logger.Info("AuthService: Repository updated dynamically.")
	s.userRepo = repo
}
