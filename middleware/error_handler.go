package middleware

import (
	"HalalMate/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorHandlerMiddleware Middleware untuk menangani error secara global
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Cek apakah ada error di context
		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err

			// Cek apakah error adalah CustomError
			if customErr, ok := err.(*utils.CustomError); ok {
				utils.ErrorResponse(c, customErr.StatusCode, customErr.Message)
				return
			}

			// Jika bukan CustomError, anggap sebagai Internal Server Error
			utils.ErrorResponse(c, http.StatusInternalServerError, "Internal Server Error")
		}
	}
}
