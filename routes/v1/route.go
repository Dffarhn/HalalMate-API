package route

import (
	"HalalMate/controllers"
	"HalalMate/handlers"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes initializes all routes
func RegisterRoutes(router *gin.Engine) {
	authHandler := controllers.NewAuthController()
	scrapHandler := controllers.NewScrapController()
	restaurantHandler := controllers.NewRestaurantController()
	chatHandler := controllers.NewChatController()
	roomHandler := controllers.NewRoomController()
	userHandler := controllers.NewUserController()
	bookmarkHandler := controllers.NewBookmarkController()

	// Register the routes
	v1Routes := router.Group("/v1")
	{
		handlers.RegisterAuthRoutes(v1Routes, authHandler)     // ✅ Fixed the undefined v1 issue
		handlers.RegisterScraperRoutes(v1Routes, scrapHandler) // ✅ Fixed the undefined v1 issue
		handlers.RegisterRestaurantRoutes(v1Routes, restaurantHandler)
		handlers.RegisterChatRoutes(v1Routes, chatHandler)
		handlers.RegisterRoomRoutes(v1Routes, roomHandler)
		handlers.RegisterUserRoutes(v1Routes, userHandler)
		handlers.RegisterBookmarkRoute(v1Routes, bookmarkHandler)
	}
}
