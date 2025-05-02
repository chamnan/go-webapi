package services

import (
	"context"
	"fmt"
	"go-webapi/internal/models"
	"go-webapi/internal/repositories"
	"go.uber.org/zap"
)

// ProfileService defines the interface for user profile operations
type ProfileService interface {
	GetProfile(ctx context.Context, userID int64) (*models.User, error)
	UpdateRepository(repo repositories.UserRepository)
	// Add UpdateProfile, etc. later
}

type profileServiceImpl struct {
	userRepo repositories.UserRepository
	logger   *zap.Logger
}

// NewProfileService creates a new ProfileService
func NewProfileService(userRepo repositories.UserRepository, logger *zap.Logger) ProfileService {
	return &profileServiceImpl{
		userRepo: userRepo,
		logger:   logger,
	}
}

// GetProfile retrieves the profile for the given user ID
func (s *profileServiceImpl) GetProfile(ctx context.Context, userID int64) (*models.User, error) {
	s.logger.Debug("Fetching profile for user", zap.Int64("userID", userID))

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		s.logger.Error("Error fetching profile from repository", zap.Int64("userID", userID), zap.Error(err))
		return nil, fmt.Errorf("could not retrieve profile: %w", err)
	}
	if user == nil {
		s.logger.Warn("Profile requested for non-existent user ID", zap.Int64("userID", userID))
		return nil, ErrUserNotFound // Use the error from AuthService or define locally
	}

	// The user object from the repo already excludes the password hash via JSON tag
	// If you need to further sanitize data before returning, do it here.
	s.logger.Debug("Profile fetched successfully", zap.Int64("userID", userID), zap.String("username", user.Username))
	return user, nil
}

// UpdateRepository dynamically replaces the repository (used during Oracle reconnection)
func (s *profileServiceImpl) UpdateRepository(repo repositories.UserRepository) {
	s.logger.Info("ProfileService: Repository updated dynamically.")
	s.userRepo = repo
}
