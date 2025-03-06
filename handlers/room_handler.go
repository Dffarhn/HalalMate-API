package handlers

import (
	"HalalMate/controllers"
	"HalalMate/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterRoomRoutes(router *gin.RouterGroup, roomController *controllers.RoomController) {
	roomGroup := router.Group("/hoca")
	{
		roomGroup.POST("/room", middleware.AuthMiddleware(), roomController.CreateRoom)
		roomGroup.GET("/room", middleware.AuthMiddleware(), roomController.GetAllRoom)

	}
}
