package handlers

import (
	"fmt"
	mw "go-webapi/internal/middleware" // Import middleware package for GetRequest*Logger funcs
	"go-webapi/internal/pkg/validation"
	"go-webapi/internal/services"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AuthHandler handles authentication related HTTP requests
type AuthHandler struct {
	authService services.AuthService
	uploadDir   string
	// No logger stored here, obtained per request from context
}

// NewAuthHandler creates a new AuthHandler
// Base logger parameter removed as we obtain logger from context in methods
func NewAuthHandler(authService services.AuthService, _ *zap.Logger, uploadDir string) *AuthHandler {
	return &AuthHandler{
		authService: authService,
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
	// Get request-scoped loggers
	fileLogger := mw.GetRequestFileLogger(c)
	sqliteLogger := mw.GetRequestSQLiteLogger(c)

	// Use fileLogger for operational/validation logs
	if !validation.ParseAndValidate(c, &req) {
		fileLogger.Warn("Login request validation failed or bad request body")
		return nil // Response already sent by ParseAndValidate
	}

	// Pass scoped loggers to service method
	token, err := h.authService.Login(c.Context(), fileLogger, sqliteLogger, req.Username, req.Password)
	if err != nil {
		// Log operational failures to fileLogger
		switch err {
		case services.ErrUserNotFound, services.ErrInvalidCredentials:
			fileLogger.Warn("Login failed", zap.String("username", req.Username), zap.Error(err))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": err.Error(),
			})
		default:
			fileLogger.Error("Internal server error during login", zap.String("username", req.Username), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Login failed due to an internal error",
			})
		}
	}

	// Log success info to fileLogger
	fileLogger.Info("Login successful", zap.String("username", req.Username))
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Login successful",
		"token":   token,
	})
}

// Register handles POST /auth/register requests (multipart/form-data)
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req RegisterRequest
	// Get request-scoped loggers
	fileLogger := mw.GetRequestFileLogger(c)
	sqliteLogger := mw.GetRequestSQLiteLogger(c)

	if err := c.BodyParser(&req); err != nil {
		fileLogger.Warn("Failed to parse register request form data", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid form data",
		})
	}

	// Perform validation on the parsed form data
	validationErrors := validation.ValidateStruct(&req)
	if validationErrors != nil {
		fileLogger.Warn("Register request form validation failed", zap.Any("details", validationErrors))
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

	// Handle file upload
	file, err := c.FormFile("photo")
	var photoPath string = ""

	if err == nil { // File was provided
		ext := filepath.Ext(file.Filename)
		uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
		uploadPath := filepath.Join(h.uploadDir, uniqueFilename)

		if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
			fileLogger.Error("Failed to ensure upload directory exists", zap.String("path", h.uploadDir), zap.Error(err))
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save photo"})
		}

		if err := c.SaveFile(file, uploadPath); err != nil {
			fileLogger.Error("Failed to save uploaded photo", zap.String("filename", file.Filename), zap.String("path", uploadPath), zap.Error(err))
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save photo"})
		}
		fileLogger.Info("Saved uploaded photo", zap.String("original_filename", file.Filename), zap.String("saved_path", uploadPath))
		photoPath = uniqueFilename
	} else if err != http.ErrMissingFile {
		fileLogger.Error("Error retrieving uploaded photo", zap.Error(err))
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Error processing photo upload"})
	}

	// Call the registration service, passing the appropriate loggers
	serviceErr := h.authService.Register(c.Context(), fileLogger, sqliteLogger, username, password, photoPath)
	if serviceErr != nil {
		switch serviceErr {
		case services.ErrUsernameExists:
			fileLogger.Warn("Registration failed: username exists", zap.String("username", username))
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": serviceErr.Error(),
			})
		case services.ErrRegistrationFailed:
			fileLogger.Error("Registration failed due to service error", zap.String("username", username), zap.Error(serviceErr))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Registration failed",
			})
		default:
			fileLogger.Error("Internal server error during registration", zap.String("username", username), zap.Error(serviceErr))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Registration failed due to an internal error",
			})
		}
	}

	fileLogger.Info("Registration successful", zap.String("username", username))
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
