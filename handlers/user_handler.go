package handlers

import (
	"HalalMate/controllers"
	"HalalMate/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes sets up the User routes
func RegisterUserRoutes(router *gin.RouterGroup, userController *controllers.UserController) {
	userGroup := router.Group("/users")
	{
		userGroup.GET("/profile", middleware.AuthMiddleware(), userController.GetUserProfile)
	}

}
