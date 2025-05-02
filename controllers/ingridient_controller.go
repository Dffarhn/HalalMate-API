package controllers

import (
	"HalalMate/services"
	"HalalMate/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

type IngridientController struct {
	IngridentService *services.IngridentService
}

func NewIngridientController() *IngridientController {
	return &IngridientController{
		IngridentService: services.NewIngridentService(),
	}
}

func (c *IngridientController) CreateIngridient(ctx *gin.Context) {

	// Extract title from request body
	var requestBody struct {
		Name string `json:"name" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&requestBody); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Name is required")
		return
	}

	title := requestBody.Name

	ingridient, err := c.IngridentService.SaveIngrident(ctx, title)

	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create ingridients chat")
		return
	}

	// Additional logic to handle room chat creation can be added here

	utils.SuccessResponse(ctx, http.StatusCreated, "ingridients chat created", ingridient)
}

//get all room chat

func (c *IngridientController) GetAllIngridient(ctx *gin.Context) {

	Ingridents, err := c.IngridentService.GetAllIngridients(ctx)

	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get Ingridents")
		return
	}

	utils.SuccessResponse(ctx, http.StatusOK, "Ingridents fetched successfully", Ingridents)
}
