package handlers

import (
	"HalalMate/controllers"
	"HalalMate/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterChatRoutes(router *gin.RouterGroup, chatController *controllers.ChatController) {
	chatGroup := router.Group("/hoca")
	{
		chatGroup.POST("/chat/:roomId", middleware.AuthMiddleware(),chatController.ChatRecomendation)

	}
}
