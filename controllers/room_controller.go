package controllers

import (
	"HalalMate/services"
	"HalalMate/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

type RoomController struct {
	RoomService *services.RoomService
}

func NewRoomController() *RoomController {
	return &RoomController{
		RoomService: services.NewRoomService(),
	}
}

// made room chat

func (c *RoomController) CreateRoom(ctx *gin.Context) {
	// Extract userId from JWT
	userId, exists := ctx.Get("userId")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "UserId is required")
		return
	}

	// Extract title from request body
	var requestBody struct {
		Title string `json:"title" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&requestBody); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Title is required")
		return
	}

	title := requestBody.Title

	room, err := c.RoomService.SaveRoom(ctx, userId.(string), title)

	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create room chat")
		return
	}

	// Additional logic to handle room chat creation can be added here

	utils.SuccessResponse(ctx, http.StatusCreated, "Room chat created", room)
}

//get all room chat

func (c *RoomController) GetAllRoom(ctx *gin.Context) {
	// Extract userId from JWT
	userId, exists := ctx.Get("userId")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "UserId is required")
		return
	}

	rooms, err := c.RoomService.GetRooms(ctx, userId.(string))

	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get rooms")
		return
	}

	utils.SuccessResponse(ctx, http.StatusOK, "Rooms fetched successfully", rooms)
}

func (c *RoomController) GetSpesificRoom(ctx *gin.Context) {
	// Ambil userId dari middleware (disimpan di context)
	userId, exists := ctx.Get("userId")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "UserId is required")
		return
	}

	// Ambil roomId dari URL params
	roomId := ctx.Param("roomId")
	if roomId == "" {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "roomId parameter is required")
		return
	}

	// Panggil service dengan Firestore-compatible context
	roomWithChat, err := c.RoomService.GetRoomByID(ctx.Request.Context(), userId.(string), roomId)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get room: "+err.Error())
		return
	}

	// Response sukses
	utils.SuccessResponse(ctx, http.StatusOK, "Room fetched successfully", roomWithChat)
}
