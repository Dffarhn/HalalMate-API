package controllers

import (
	"HalalMate/models"
	"HalalMate/services"
	"HalalMate/utils"
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type BookmarkController struct {
	BookmarkService *services.BookmarkService
}

func NewBookmarkController() *BookmarkController {
	return &BookmarkController{
		BookmarkService: services.NewBookmarkService(),
	}
}

func (b *BookmarkController) GetAllBookmark(c *gin.Context) {
	userID, exists := c.Get("userId")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "UserId is required")
		return
	}

	//the latitude and longtitude is from query
	latitudeStr := c.Query("latitude")
	longitudeStr := c.Query("longitude")

	latitude, err := strconv.ParseFloat(latitudeStr, 64)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid latitude")
		return
	}

	longitude, err := strconv.ParseFloat(longitudeStr, 64)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid longitude")
		return
	}

	bookmarks, err := b.BookmarkService.GetAllBookmarks(context.Background(), userID.(string),latitude,longitude)
	if err != nil {
		c.Error(err) // Middleware akan menangani error ini
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Bookmark fetched successfully", bookmarks)
}

func (b *BookmarkController) GetOneBookmark(c *gin.Context) {
	userId, exists := c.Get("userId")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "UserId is required")
		return
	}
	bookmarkID := c.Param("bookmarkID")

	if bookmarkID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format")
		return
	}

	bookmark, err := b.BookmarkService.GetBookmarkByID(context.Background(), userId.(string), bookmarkID)
	if err != nil {
		c.Error(err) // Middleware akan menangani error ini
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Bookmark fetched successfully", bookmark)
}

func (b *BookmarkController) CreateBookmark(c *gin.Context) {
	userID, exists := c.Get("userId")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized: User ID is required")
		return
	}

	// Extract restaurant ID from request body
	var requestBody struct {
		RestaurantID string `json:"restaurantId" binding:"required"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format or missing restaurantId")
		return
	}

	// Create a new bookmark struct
	bookmark := &models.Bookmark{
		UserID:       userID.(string),
		RestaurantID: requestBody.RestaurantID,
		CreatedAt:    time.Now(),
	}

	// Save the bookmark
	savedBookmark, err := b.BookmarkService.PostBookmark(context.Background(), bookmark.UserID, *bookmark)
	if err != nil {
		c.Error(err) // Middleware akan menangani error ini
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Bookmark added successfully", savedBookmark)
}

func (b *BookmarkController) DeleteBookmark(c *gin.Context) {
	userID, exists := c.Get("userId")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "UserId is required")
		return
	}

	bookmarkID := c.Param("bookmarkID")

	if userID == "" || bookmarkID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format")
		return
	}

	err := b.BookmarkService.DeleteBookmark(context.Background(), userID.(string), bookmarkID)
	if err != nil {
		c.Error(err) // Middleware akan menangani error ini
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Bookmark deleted successfully", nil)
}
