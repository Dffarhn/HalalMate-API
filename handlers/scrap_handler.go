package handlers

import (
	"HalalMate/controllers"

	"github.com/gin-gonic/gin"
)

// RegisterScraperRoutes sets up the scraper-related routes
func RegisterScraperRoutes(router *gin.RouterGroup , scraperController *controllers.ScrapController) {
	scraperGroup := router.Group("/scraper")
	{
		scraperGroup.POST("/", scraperController.GetAllScrapePlaces )
		scraperGroup.POST("/restaurant",scraperController.ScrapeSinglePlace)
	}
}
