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
}

type profileServiceImpl struct {
	userRepo   repositories.UserRepository
	fileLogger *zap.Logger // Renamed from logger
}

// NewProfileService creates a new ProfileService
func NewProfileService(userRepo repositories.UserRepository, fileLogger *zap.Logger) ProfileService {
	return &profileServiceImpl{
		userRepo:   userRepo,
		fileLogger: fileLogger,
	}
}

// GetProfile retrieves the profile for the given user ID
func (s *profileServiceImpl) GetProfile(ctx context.Context, userID int64) (*models.User, error) {
	s.fileLogger.Debug("Fetching profile for user", zap.Int64("userID", userID))

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		s.fileLogger.Error("Error fetching profile from repository", zap.Int64("userID", userID), zap.Error(err))
		return nil, fmt.Errorf("could not retrieve profile: %w", err)
	}
	if user == nil {
		s.fileLogger.Warn("Profile requested for non-existent user ID", zap.Int64("userID", userID))
		return nil, ErrUserNotFound // Use the error from AuthService or define locally if needed
	}

	s.fileLogger.Debug("Profile fetched successfully", zap.Int64("userID", userID), zap.String("username", user.Username))
	return user, nil
}

// UpdateRepository dynamically replaces the repository (used during Oracle reconnection)
func (s *profileServiceImpl) UpdateRepository(repo repositories.UserRepository) {
	s.fileLogger.Info("ProfileService: Repository updated dynamically.")
	s.userRepo = repo
}
