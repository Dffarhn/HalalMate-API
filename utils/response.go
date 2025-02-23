package utils

import (
	"github.com/gin-gonic/gin"
)

// Response struct for API responses
type Response struct {
	StatusCode int         `json:"statusCode"` // Capitalized to be exported
	Message    string      `json:"message"`
	Data       interface{} `json:"data,omitempty"` // Capitalized to be exported
}

// SuccessResponse sends a success response
func SuccessResponse(c *gin.Context, statusCode int, message string, data interface{}) {
	c.JSON(statusCode, Response{
		StatusCode: statusCode, // Use the capitalized field
		Message:    message,     // Use the capitalized field
		Data:       data,        // Use the capitalized field
	})
}

// ErrorResponse sends an error response
func ErrorResponse(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, Response{
		StatusCode: statusCode, // Use the capitalized field
		Message:    message,     // Use the capitalized field
	})
}
