package controllers

import (
	"HalalMate/models"
	"HalalMate/services"
	"HalalMate/utils"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthHandler struct
type ScrapController struct {
	ScrapService *services.ScrapService
}

// NewAuthHandler initializes AuthHandler
func NewScrapController() *ScrapController {
	return &ScrapController{
		ScrapService: services.NewScrapService(services.NewOpenAIService()),
	}
}

// ScrapeRequest represents the request payload
type ScrapeRequest struct {
	Latitude  float64 `json:"latitude" binding:"required"`
	Longitude float64 `json:"longitude" binding:"required"`
	Keyword   string  `json:"keyword"`
}

// ScrapeHandler processes the incoming request and calls the service
func (h *ScrapController) GetAllScrapePlaces(c *gin.Context) {
	var req ScrapeRequest

	// Bind JSON request
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format")
		return
	}

	formattedKeyword := strings.ReplaceAll(req.Keyword, " ", "+")
	latitude := fmt.Sprintf("%.7f", req.Latitude)
	longitude := fmt.Sprintf("%.7f", req.Longitude)

	url := fmt.Sprintf("https://www.google.com/maps/search/%s/@%s,%s,18.5z", formattedKeyword, latitude, longitude)

	log.Println("Scraping URL:", url)

	// Set response headers for SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()

	// Create channels to stream results
	placeChan := make(chan models.Place)
	doneChan := make(chan bool)

	// Start scraping in a separate goroutine
	go h.ScrapService.ScrapePlaces([]string{url}, placeChan, doneChan)

	// Stream results via SSE
	for {
		select {
		case place, ok := <-placeChan:
			if !ok {
				placeChan = nil // Channel closed, stop reading
			} else {
				c.SSEvent("place_scrap", place)
				c.Writer.Flush()
			}
		case <-doneChan:
			c.SSEvent("done_scrap", gin.H{"statusCode": 200, "message": "Scraping completed", "data": nil})
			c.Writer.Flush()
			return
		}
	}
}

func (c *ScrapController) ScrapeSinglePlace(ctx *gin.Context) {
	mapsLink := ctx.Query("mapsLink")
	if mapsLink == "" {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "mapsLink query parameter is required")
		return
	}

	place, err := c.ScrapService.ScrapeSinglePlace(mapsLink)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	utils.SuccessResponse(ctx, http.StatusOK, "Place scraped successfully", place)
}
