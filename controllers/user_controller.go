package controllers

import (
	"HalalMate/services"
	"HalalMate/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserController struct {
	UserService *services.UserService
}

func NewUserController() *UserController {
	return &UserController{
		UserService: services.NewUserService(),
	}
}

// controller profile

func (h *UserController) GetUserProfile(ctx *gin.Context) {
	// get user profile

	userId, exists := ctx.Get("userId")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "UserId is required")
		return

	}

	user, err := h.UserService.GetUserProfile(ctx, userId.(string))

	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get user profile")
		return
	}

	utils.SuccessResponse(ctx, http.StatusOK, "success fetch User profile", user)
}
