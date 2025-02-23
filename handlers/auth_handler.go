package handlers

import (
	"HalalMate/controllers"

	"github.com/gin-gonic/gin"
)



func RegisterAuthRoutes(router *gin.RouterGroup, authController *controllers.AuthController) {

	scraperGroup := router.Group("/auth")
	{
		scraperGroup.POST("/register", authController.RegisterUser)
		scraperGroup.POST("/login", authController.LoginUser)
		scraperGroup.POST("/google", authController.GoogleLogin)
	}

}
