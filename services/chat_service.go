package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

type ChatService struct {
	RestaurantService *RestaurantService
	RoomService       *RoomService
	OpenAIService     *OpenAIService
	FirestoreClient   *firestore.Client
}

// NewRecomendationService initializes RecomendationService with RestaurantService and OpenAIService
func NewChatService() *ChatService {
	return &ChatService{
		RestaurantService: NewRestaurantService(),
		RoomService:       NewRoomService(),
		OpenAIService:     NewOpenAIService(),
		FirestoreClient:   database.GetFirestoreClient(),
	}
}

//model user prompt system prompt

func (s *ChatService) StreamRecommendations(
	ctx context.Context,
	recommendationChan chan<- string,
	doneChan chan<- bool,
	location models.GeoLocation,
	prompt string,
	userId string,
	roomId string,
) {
	defer close(recommendationChan)
	defer close(doneChan)

	// Fetch nearby restaurants
	restaurants, err := s.RestaurantService.GetAllRestaurantByLocation(ctx, location.Latitude, location.Longitude)
	if err != nil {
		log.Println("Error fetching restaurants:", err)
		doneChan <- true
		return
	}

	//get history chat before

	rooms, err := s.RoomService.GetRoomByID(ctx, userId, roomId)

	if err != nil {
		log.Println("Error fetching chats:", err)
		doneChan <- true
		return
	}

	// Format chat history
	var chatHistory strings.Builder

	for _, chat := range rooms.Chats {
		chatHistory.WriteString(fmt.Sprintf(
			"**User %s** (%s):\n%s\n\n",
			chat.UserID, chat.CreatedAt, chat.Chat,
		))
	}

	// Convert restaurant data to JSON
	restaurantsJSON, err := json.Marshal(restaurants)
	if err != nil {
		log.Println("Error marshaling restaurants:", err)
		doneChan <- true
		return
	}

	// System prompt for AI
	systemPrompt := fmt.Sprintf(
		"Generate restaurant recommendations based on the user's request using the provided restaurant data.\n\n"+
			"### 🗣 Chat History:\n\n%s\n\n"+
			"### ⚡ Guidelines:\n"+
			"- You are free to generate descriptive and engaging recommendations, but **any data retrieved from the database must be formatted in a markdown-like block**.\n"+
			"- If a specific detail (e.g., menu, distance) **is missing in the database, do not guess or fabricate it**—simply omit it.\n"+
			"- Ensure that responses remain **concise, structured, and within the given data limits**.\n\n"+
			"###  Recommended Restaurants:\n\n%s\n\n"+
			"### Formatting Rules for Database Data:\n"+
			"- **Wrap all database-sourced restaurant details** inside a markdown-like fenced block:\n"+
			"  ```md\n"+
			"  **Name**: {{name}}\n"+
			"  **ID**: {{id}}\n"+
			"  **Distance**: {{distance}} km\n"+
			"  **Menu Highlights**: {{menu}}\n"+
			"  ```\n"+
			"- Feel free to add context or suggestions outside this block to make the response more engaging.\n"+
			"- Example output:\n"+
			"  *Looking for the best sushi spot? Try this one!*\n\n"+
			"  ```md\n"+
			"  **Name**: Sushi Go\n"+
			"  **ID**: 12345\n"+
			"  **Distance**: 2.4 km\n"+
			"  **Menu Highlights**: Salmon Sashimi, Tuna Roll\n"+
			"  ```\n\n"+
			" **Reminder**: Do **not** exceed the provided data limits, and avoid making assumptions about missing details.",
		chatHistory.String(),
		string(restaurantsJSON),
	)

	// Start OpenAI streaming
	stream, err := s.OpenAIService.ChatStream(ctx, systemPrompt, prompt)
	if err != nil {
		log.Println("Error starting OpenAI stream:", err)
		doneChan <- true
		return
	}
	defer stream.Close()

	// Read and process response line by line
	reader := bufio.NewReader(stream)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break // End of stream
		}

		// OpenAI responses are prefixed with "data: "
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break // End of stream
			}

			// Parse OpenAI JSON response
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(data), &parsed); err != nil {
				log.Println("Error parsing streaming data:", err)
				continue
			}

			// Extract content from OpenAI response
			if choices, ok := parsed["choices"].([]interface{}); ok && len(choices) > 0 {
				if delta, ok := choices[0].(map[string]interface{})["delta"].(map[string]interface{}); ok {
					if content, ok := delta["content"].(string); ok {
						// Send extracted recommendation content
						recommendationChan <- content
					}
				}
			}
		}
	}

	// Signal completion
	doneChan <- true
}

// save chat into firebase
func (s *ChatService) SaveChat(ctx context.Context, userId string, prompt string, roomId string, hocaAI bool) error {

	var chatData models.Chat

	chatData.RoomID = roomId
	chatData.Chat = prompt
	chatData.CreatedAt = time.Now().Format(time.RFC3339)

	if hocaAI {
		chatData.UserID = "HocaAI"
	} else {
		chatData.UserID = userId
	}

	chatRef := s.FirestoreClient.Collection("users").Doc(userId).Collection("rooms").Doc(roomId).Collection("chats").NewDoc()
	_, err := chatRef.Set(ctx, chatData)
	if err != nil {
		return err
	}
	return nil
}

//save room into firebase

func (s *ChatService) SaveRoom(ctx context.Context, userId string) (*models.Room, error) {

	var room models.Room
	// Buat dokumen baru di Firestore (Firestore akan otomatis generate ID)
	roomRef := s.FirestoreClient.Collection("users").Doc(userId).Collection("rooms").NewDoc()

	// Gunakan waktu sekarang jika CreatedAt tidak diset
	room.CreatedAt = time.Now().Format(time.RFC3339) // Format waktu standar

	room.UserID = userId

	// Simpan data ke Firestore
	_, err := roomRef.Set(ctx, room)
	if err != nil {
		return nil, err
	}

	// Kembalikan objek Room dengan RoomID yang di-generate Firestore
	return &models.Room{
		RoomID:    roomRef.ID, // Firestore-generated ID
		UserID:    room.UserID,
		CreatedAt: room.CreatedAt,
	}, nil
}

//get all room chat

func (s *ChatService) GetRooms(ctx context.Context, userId string) ([]*models.Room, error) {
	var rooms []*models.Room

	// Query rooms collection
	roomDocs, err := s.FirestoreClient.Collection("users").Doc(userId).Collection("rooms").Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	// Iterate over room documents
	for _, doc := range roomDocs {
		var room models.Room
		if err := doc.DataTo(&room); err != nil {
			return nil, err
		}

		// Append room to list
		rooms = append(rooms, &room)
	}

	return rooms, nil
}
