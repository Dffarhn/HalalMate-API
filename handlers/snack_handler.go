package handlers

import (
	"HalalMate/controllers"

	"github.com/gin-gonic/gin"
)

func RegisterSnackRoutes(router *gin.RouterGroup, snackController *controllers.SnackController) {
	snackGroup := router.Group("/snack")
	{
		snackGroup.POST("/:barcode", snackController.ScanSnackByBarcode)
		snackGroup.POST("/image", snackController.ScanSnackByImage)
	}
}