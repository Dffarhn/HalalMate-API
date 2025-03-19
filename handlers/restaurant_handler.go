package handlers

import (
	"HalalMate/controllers"
	"HalalMate/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterRestaurantRoutes(router *gin.RouterGroup, restaurantController *controllers.RestaurantController) {
	restaurantGroup := router.Group("/restaurants")
	{
		restaurantGroup.GET("/", middleware.AuthMiddleware(), restaurantController.GetAllRestaurants)

		restaurantGroup.GET("/:id", middleware.AuthMiddleware(), restaurantController.GetRestaurantByID)

	}
}
