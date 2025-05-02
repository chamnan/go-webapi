
package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"go-webapi/internal/services"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
    "github.com/google/uuid" // For generating unique filenames
)

// AuthHandler handles authentication related HTTP requests
type AuthHandler struct {
	authService services.AuthService
	logger      *zap.Logger
    uploadDir   string
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(authService services.AuthService, logger *zap.Logger, uploadDir string) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		logger:      logger,
        uploadDir:   uploadDir,
	}
}

// LoginRequest defines the expected JSON body for login requests
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// Login handles POST /auth/login requests
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Warn("Failed to parse login request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

    // Optional: Add validation using a library like go-playground/validator
    // if err := validate.Struct(req); err != nil {
    //     return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Validation failed", "details": err.Error()})
    // }

	if req.Username == "" || req.Password == "" {
         return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Username and password are required",
		})
    }


	token, err := h.authService.Login(c.Context(), req.Username, req.Password)
	if err != nil {
		// Service layer returns specific errors we can map to HTTP status codes
		switch err {
		case services.ErrUserNotFound, services.ErrInvalidCredentials:
			h.logger.Warn("Login failed", zap.String("username", req.Username), zap.Error(err))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": err.Error(), // Use the error message from the service
			})
		default:
			h.logger.Error("Internal server error during login", zap.String("username", req.Username), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Login failed due to an internal error",
			})
		}
	}

	h.logger.Info("Login successful", zap.String("username", req.Username))
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Login successful",
		"token":   token,
	})
}

// Register handles POST /auth/register requests (multipart/form-data)
func (h *AuthHandler) Register(c *fiber.Ctx) error {
    // Expecting multipart/form-data
	username := c.FormValue("username")
	password := c.FormValue("password")

    if username == "" || password == "" {
         return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Username and password form fields are required",
		})
    }

    // Handle file upload
	file, err := c.FormFile("photo")
    var photoPath string = "" // Relative path to store in DB

	if err == nil { // File was provided
        // Generate unique filename to prevent collisions
        ext := filepath.Ext(file.Filename)
        uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
		uploadPath := filepath.Join(h.uploadDir, uniqueFilename)

        // Ensure upload directory exists (should be done at startup, but double-check)
        if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
             h.logger.Error("Failed to ensure upload directory exists", zap.String("path", h.uploadDir), zap.Error(err))
             return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save photo"})
        }


		// Save the file
		if err := c.SaveFile(file, uploadPath); err != nil {
			h.logger.Error("Failed to save uploaded photo", zap.String("filename", file.Filename), zap.String("path", uploadPath), zap.Error(err))
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save photo"})
		}
		h.logger.Info("Saved uploaded photo", zap.String("original_filename", file.Filename), zap.String("saved_path", uploadPath))
        photoPath = uniqueFilename // Store only the filename or relative path in DB
	} else if err != http.ErrMissingFile {
        // An error occurred other than the file simply not being provided
        h.logger.Error("Error retrieving uploaded photo", zap.Error(err))
        return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Error processing photo upload"})
    }
    // If err == http.ErrMissingFile, photoPath remains empty, which is acceptable


	// Call the registration service
	err = h.authService.Register(c.Context(), username, password, photoPath)
	if err != nil {
		switch err {
		case services.ErrUsernameExists:
			h.logger.Warn("Registration failed: username exists", zap.String("username", username))
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": err.Error(),
			})
        case services.ErrRegistrationFailed:
            h.logger.Error("Registration failed due to service error", zap.String("username", username), zap.Error(err))
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Registration failed", // Generic message
			})
		default:
			h.logger.Error("Internal server error during registration", zap.String("username", username), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Registration failed due to an internal error",
			})
		}
	}

	h.logger.Info("Registration successful", zap.String("username", username))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User registered successfully",
		// Optionally return user info (excluding password) or just success message
	})
}

// SetupAuthRoutes registers authentication routes with the Fiber app
func (h *AuthHandler) SetupAuthRoutes(router fiber.Router) {
    authGroup := router.Group("/auth")
	authGroup.Post("/login", h.Login)
	authGroup.Post("/register", h.Register)
}


