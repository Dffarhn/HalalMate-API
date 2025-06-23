package handlers

import (
	"HalalMate/controllers"

	"github.com/gin-gonic/gin"
)

// RegisterScraperRoutes sets up the scraper-related routes
func RegisterScraperRoutes(router *gin.RouterGroup, scraperController *controllers.ScrapController) {
	scraperGroup := router.Group("/scraper")
	{
		// Routes with trailing slash
		scraperGroup.GET("/", scraperController.GetAllScrapePlaces)
		scraperGroup.POST("/", scraperController.GetAllScrapePlaces)

		// Routes without trailing slash (to prevent redirects)
		scraperGroup.GET("", scraperController.GetAllScrapePlaces)
		scraperGroup.POST("", scraperController.GetAllScrapePlaces)

		scraperGroup.POST("/restaurant", scraperController.ScrapeSinglePlace)
		scraperGroup.GET("/test", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "CORS test successful"})
		})
	}
}
