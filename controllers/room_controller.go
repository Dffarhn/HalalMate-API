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

	room, err := c.RoomService.SaveRoom(ctx, userId.(string))

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


