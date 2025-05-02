package handlers

import (
	"HalalMate/controllers"

	"github.com/gin-gonic/gin"
)

func RegisterIngridentsRoutes(router *gin.RouterGroup, ingridentController *controllers.IngridientController) {
	ingredientsRoutes := router.Group("/ingridients")
	{
		ingredientsRoutes.POST("/", ingridentController.CreateIngridient)
		ingredientsRoutes.GET("/", ingridentController.GetAllIngridient)
	}
}