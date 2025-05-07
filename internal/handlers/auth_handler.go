package handlers

import (
	"fmt"
	"go-webapi/internal/pkg/validation" // Import the validation package
	"go-webapi/internal/services"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid" // For generating unique filenames
	"go.uber.org/zap"
)

// AuthHandler handles authentication related HTTP requests
type AuthHandler struct {
	authService services.AuthService
	fileLogger  *zap.Logger // Renamed from logger
	uploadDir   string
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(authService services.AuthService, fileLogger *zap.Logger, uploadDir string) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		fileLogger:  fileLogger,
		uploadDir:   uploadDir,
	}
}

// LoginRequest defines the expected JSON body for login requests
type LoginRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=6"`
}

// RegisterRequest defines the expected form fields for registration requests (for validation purposes)
type RegisterRequest struct {
	Username string `form:"username" validate:"required,min=3,max=50,alphanum"`
	Password string `form:"password" validate:"required,min=6"`
}

// Login handles POST /auth/login requests
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if !validation.ParseAndValidate(c, &req) {
		h.fileLogger.Warn("Login request validation failed or bad request body")
		return nil
	}

	token, err := h.authService.Login(c.Context(), req.Username, req.Password)
	if err != nil {
		switch err {
		case services.ErrUserNotFound, services.ErrInvalidCredentials:
			h.fileLogger.Warn("Login failed", zap.String("username", req.Username), zap.Error(err))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": err.Error(),
			})
		default:
			h.fileLogger.Error("Internal server error during login", zap.String("username", req.Username), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Login failed due to an internal error",
			})
		}
	}

	h.fileLogger.Info("Login successful", zap.String("username", req.Username))
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Login successful",
		"token":   token,
	})
}

// Register handles POST /auth/register requests (multipart/form-data)
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		h.fileLogger.Warn("Failed to parse register request form data", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid form data",
		})
	}

	validationErrors := validation.ValidateStruct(&req)
	if validationErrors != nil {
		h.fileLogger.Warn("Register request form validation failed", zap.Any("details", validationErrors))
		errorMessages := make([]string, len(validationErrors))
		for i, ve := range validationErrors {
			errorMessages[i] = ve.Message
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":    "Validation failed",
			"details":  validationErrors,
			"messages": errorMessages,
		})
	}

	username := req.Username
	password := req.Password

	file, err := c.FormFile("photo")
	var photoPath string = ""

	if err == nil {
		ext := filepath.Ext(file.Filename)
		uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
		uploadPath := filepath.Join(h.uploadDir, uniqueFilename)

		if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
			h.fileLogger.Error("Failed to ensure upload directory exists", zap.String("path", h.uploadDir), zap.Error(err))
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save photo"})
		}

		if err := c.SaveFile(file, uploadPath); err != nil {
			h.fileLogger.Error("Failed to save uploaded photo", zap.String("filename", file.Filename), zap.String("path", uploadPath), zap.Error(err))
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save photo"})
		}
		h.fileLogger.Info("Saved uploaded photo", zap.String("original_filename", file.Filename), zap.String("saved_path", uploadPath))
		photoPath = uniqueFilename
	} else if err != http.ErrMissingFile {
		h.fileLogger.Error("Error retrieving uploaded photo", zap.Error(err))
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Error processing photo upload"})
	}

	serviceErr := h.authService.Register(c.Context(), username, password, photoPath)
	if serviceErr != nil {
		switch serviceErr {
		case services.ErrUsernameExists:
			h.fileLogger.Warn("Registration failed: username exists", zap.String("username", username))
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": serviceErr.Error(),
			})
		case services.ErrRegistrationFailed:
			h.fileLogger.Error("Registration failed due to service error", zap.String("username", username), zap.Error(serviceErr))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Registration failed",
			})
		default:
			h.fileLogger.Error("Internal server error during registration", zap.String("username", username), zap.Error(serviceErr))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Registration failed due to an internal error",
			})
		}
	}

	h.fileLogger.Info("Registration successful", zap.String("username", username))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User registered successfully",
	})
}

// SetupAuthRoutes registers authentication routes with the Fiber app
func (h *AuthHandler) SetupAuthRoutes(router fiber.Router) {
	authGroup := router.Group("/auth")
	authGroup.Post("/login", h.Login)
	authGroup.Post("/register", h.Register)
}
