package validation

import (
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

// StructValidator is a singleton instance of the validator.
var StructValidator = validator.New(validator.WithRequiredStructEnabled())

// ErrorResponse represents a validation error message.
type ErrorResponse struct {
	FailedField string `json:"failed_field"`
	Tag         string `json:"tag"`
	Value       string `json:"value"`
	Message     string `json:"message"` // Custom message
}

// ValidateStruct performs validation on a struct.
// It returns a slice of ErrorResponse if validation fails, or nil otherwise.
func ValidateStruct(payload interface{}) []*ErrorResponse {
	var errors []*ErrorResponse
	err := StructValidator.Struct(payload)
	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			var element ErrorResponse
			element.FailedField = err.StructNamespace() // Fully qualified field name
			element.Tag = err.Tag()
			element.Value = fmt.Sprintf("%v", err.Value()) // Value that failed validation
			element.Message = generateValidationMessage(err)
			errors = append(errors, &element)
		}
	}
	return errors
}

// generateValidationMessage creates a user-friendly message for a validation error.
// This can be customized further based on specific tags or fields.
func generateValidationMessage(err validator.FieldError) string {
	field := err.Field()
	// Use StructField for potentially better naming (e.g., if struct has JSON tags, StructField reflects Go field name)
	// For simple cases, err.Field() might be preferred if it already gives the desired name (e.g., from json tags if validator is configured for it)
	// Sticking to err.Field() for now as it's generally simpler unless specific tag name strategies are in play.
	// If you want the Go struct field name: field = err.StructField()

	tag := err.Tag()
	param := err.Param()
	kind := err.Kind() // Get the reflect.Kind of the field

	switch tag {
	case "required":
		return fmt.Sprintf("The %s field is required.", field)
	case "email":
		return fmt.Sprintf("The %s field must be a valid email address.", field)
	case "min":
		switch kind {
		case reflect.String, reflect.Slice, reflect.Map, reflect.Array: // Corrected check
			return fmt.Sprintf("The %s field must have at least %s items/characters.", field, param)
		default: // For numbers
			return fmt.Sprintf("The %s field must be at least %s.", field, param)
		}
	case "max":
		switch kind {
		case reflect.String, reflect.Slice, reflect.Map, reflect.Array: // Corrected check
			return fmt.Sprintf("The %s field must have at most %s items/characters.", field, param)
		default: // For numbers
			return fmt.Sprintf("The %s field must be at most %s.", field, param)
		}
	case "len":
		switch kind {
		case reflect.String, reflect.Slice, reflect.Map, reflect.Array: // Corrected check
			return fmt.Sprintf("The %s field must have exactly %s items/characters.", field, param)
		default: // For numbers
			return fmt.Sprintf("The %s field must be exactly %s.", field, param)
		}
	case "alphanum":
		return fmt.Sprintf("The %s field may only contain alpha-numeric characters.", field)
	case "url":
		return fmt.Sprintf("The %s field must be a valid URL.", field)
	// Add more cases for other common tags as needed
	default:
		return fmt.Sprintf("The %s field is not valid (tag: %s).", field, tag)
	}
}

// ParseAndValidate is a utility function for Fiber handlers to parse the body and validate it.
// It returns true if parsing and validation are successful, false otherwise.
// If false, it sends the appropriate error response.
func ParseAndValidate(c *fiber.Ctx, payload interface{}) bool {
	if err := c.BodyParser(payload); err != nil {
		// It's good practice to log parsing errors on the server side for debugging
		// logger := logging.GetFileLogger() // Assuming you have a way to get your logger
		// logger.Warn("Failed to parse request body", zap.Error(err), zap.String("path", c.Path()))
		c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request body",
			"details": err.Error(), // Avoid sending raw error in production if it leaks info
		})
		return false
	}

	validationErrors := ValidateStruct(payload)
	if validationErrors != nil {
		errorMessages := make([]string, len(validationErrors))
		for i, ve := range validationErrors {
			errorMessages[i] = ve.Message
		}
		c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":    "Validation failed",
			"details":  validationErrors, // Provides detailed field errors
			"messages": errorMessages,    // Provides user-friendly messages
		})
		return false
	}
	return true
}
