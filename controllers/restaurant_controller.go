package controllers

import (
	"HalalMate/services"
	"HalalMate/utils"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type RestaurantController struct {
	RestaurantService *services.RestaurantService
}

func NewRestaurantController() *RestaurantController {
	return &RestaurantController{
		RestaurantService: services.NewRestaurantService(),
	}
}

func (s *RestaurantController) GetAllRestaurants(c *gin.Context) {

	userId, exists := c.Get("userId")
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

	

	restaurants, err := s.RestaurantService.GetAllRestaurantByLocation(c, latitude, longitude, userId.(string))
	if err != nil {
		// Log the error for debugging purposes
		print("Error fetching restaurants: ", err)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Error fetching restaurants")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Restaurants fetched successfully", restaurants)
}

func (s *RestaurantController) GetRestaurantByID(c *gin.Context) {

	userId, exists := c.Get("userId")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "UserId is required")
		return
	}
	restaurantID := c.Param("id")

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

	restaurant, err := s.RestaurantService.GetRestaurantByIdAndLocation(c, restaurantID, latitude, longitude, userId.(string))
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Error fetching restaurant")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Restaurant fetched successfully", restaurant)
}
