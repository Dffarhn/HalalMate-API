package handlers

import (
	"HalalMate/controllers"

	"github.com/gin-gonic/gin"
)

func RegisterSnackRoutes(router *gin.RouterGroup, snackController *controllers.SnackController) {
	snackGroup := router.Group("/snack")
	{
		// snackGroup.POST("/:barcode", snackController.ScanSnackByBarcode)
		snackGroup.POST("/image", snackController.ScanSnackByImage)
		snackGroup.POST("/scan/front", snackController.ScanWithFrontOnly)
		snackGroup.POST("/scan/front-back", snackController.ScanWithFrontAndBack)
		snackGroup.POST("/scan/full", snackController.ScanWithImageAndBarcode)
		snackGroup.POST("/scan", snackController.SearchSnackByInput)

	}
}
