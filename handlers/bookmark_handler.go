package handlers

import (
	"HalalMate/controllers"
	"HalalMate/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterBookmarkRoute(router *gin.RouterGroup, bookmarkController *controllers.BookmarkController) {
	bookmarkGroup := router.Group("/bookmark")

	{
		bookmarkGroup.GET("", middleware.AuthMiddleware(), bookmarkController.GetAllBookmark)
		bookmarkGroup.GET("/:bookmarkID", middleware.AuthMiddleware(), bookmarkController.GetOneBookmark)
		bookmarkGroup.POST("", middleware.AuthMiddleware(), bookmarkController.CreateBookmark)
		bookmarkGroup.DELETE("/:bookmarkID", middleware.AuthMiddleware(), bookmarkController.DeleteBookmark)

	}

}
