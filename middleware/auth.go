package middleware

import (
	"HalalMate/utils"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware validates JWT tokens
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenHeader := c.GetHeader("Authorization")
		if tokenHeader == "" {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Authorization header is required")
			c.Abort()
			return
		}

		tokenParts := strings.Split(tokenHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid token format")
			c.Abort()
			return
		}

		tokenString := tokenParts[1]
		token, err := utils.ValidateToken(tokenString)
		if err != nil || !token.Valid {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid or expired token")
			c.Abort()
			return
		}

		// Extract user ID from token claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid token claims")
			c.Abort()
			return
		}

		// Store the user ID in the context
		userID, ok := claims["uid"].(string)
		if !ok {
			utils.ErrorResponse(c, http.StatusUnauthorized, "User ID not found in token")
			c.Abort()
			return
		}

		// Pass userID to the context
		c.Set("userId", userID)

		c.Next()
	}
}
