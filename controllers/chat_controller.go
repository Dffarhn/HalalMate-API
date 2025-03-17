package controllers

import (
	"HalalMate/models"
	"HalalMate/services"
	"HalalMate/utils"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// RecommendationController struct
type ChatController struct {
	ChatService *services.ChatService
}

// NewChatController initializes ChatController with the service layer
func NewChatController() *ChatController {
	return &ChatController{
		ChatService: services.NewChatService(),
	}
}

// RecommendationRequest represents the request payload
type ChatRequest struct {
	Latitude  string `json:"latitude" binding:"required"`
	Longitude string `json:"longitude" binding:"required"`
	Prompt    string `json:"prompt" binding:"required"`
}

// StreamRecommendations streams recommended places
func (c *ChatController) ChatRecomendation(ctx *gin.Context) {
	var req ChatRequest

	// Bind JSON request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid request format")
		return
	}

	userId, exists := ctx.Get("userId")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "UserId is required")
		return
	}

	//binding on parameter for roomId

	roomId := ctx.Param("roomId")

	// Check if roomId is empty
	if roomId == "" {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Room ID is required")
		return
	}

	// Convert request data to GeoLocation

	latitude, err := strconv.ParseFloat(req.Latitude, 64)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid latitude format")
		return
	}
	longitude, err := strconv.ParseFloat(req.Longitude, 64)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid longitude format")
		return
	}
	location := models.GeoLocation{Latitude: latitude, Longitude: longitude}
	formattedPrompt := strings.TrimSpace(req.Prompt)

	log.Println("Streaming recommendations for:", formattedPrompt)

	// Set SSE headers
	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Flush()

	// Create channels for streaming responses
	recommendationChan := make(chan string)
	doneChan := make(chan bool)

	if err := c.ChatService.SaveChat(ctx, userId.(string), req.Prompt, roomId, false); err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to save chat user")
		return
	}

	// Start streaming recommendations in a separate goroutine
	go c.ChatService.StreamRecommendations(ctx, recommendationChan, doneChan, location, formattedPrompt, userId.(string), roomId)

	// Stream results via SSE
	// Stream results via SSE
	var recommendations []string // Use []string for better handling

	for {
		select {
		case recommendation, ok := <-recommendationChan:
			if !ok {
				recommendationChan = nil // Channel closed, stop reading
			} else {
				// Convert recommendation to string
				recStr := fmt.Sprint(recommendation)
				recommendations = append(recommendations, recStr)

				// Send recommendation event to client
				ctx.SSEvent("recommendation", recStr)
				ctx.Writer.Flush() // Ensure event is sent immediately
			}

		case <-doneChan:
			// Save all collected recommendations to Firestore
			err := c.ChatService.SaveChat(ctx, userId.(string), strings.Join(recommendations, ""), roomId, true)

			if err != nil {
				ctx.SSEvent("error", gin.H{
					"statusCode": 500,
					"message":    "Failed to save recommendations",
				})
				ctx.Writer.Flush()
				return
			}

			// Send final event with all recommendations
			ctx.SSEvent("done_recommendations", gin.H{
				"statusCode": 200,
				"message":    "Recommendation process completed",
				"data":       recommendations,
			})
			ctx.Writer.Flush() // Ensure final event is sent
			return
		}
	}

}
