package handlers

import (
	"HalalMate/controllers"

	"github.com/gin-gonic/gin"
)

func RegisterRestaurantRoutes(router *gin.RouterGroup, restaurantController *controllers.RestaurantController) {
	restaurantGroup := router.Group("/restaurants")
	{
		restaurantGroup.GET("/", restaurantController.GetAllRestaurants)

		restaurantGroup.GET("/:id", restaurantController.GetRestaurantByID)

	}
}