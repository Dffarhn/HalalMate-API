package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"HalalMate/utils"
	"context"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
)

type RoomService struct {
	FirestoreClient *firestore.Client
}

func NewRoomService() *RoomService {
	return &RoomService{
		FirestoreClient: database.GetFirestoreClient(),
	}
}

//save room into firebase

func (s *RoomService) SaveRoom(ctx context.Context, userId string, title string) (*models.Room, error) {

	var room models.Room
	// Buat dokumen baru di Firestore (Firestore akan otomatis generate ID)
	roomRef := s.FirestoreClient.Collection("users").Doc(userId).Collection("rooms").NewDoc()

	// Gunakan waktu sekarang jika CreatedAt tidak diset
	room.CreatedAt = time.Now().Format(time.RFC3339) // Format waktu standar

	room.UserID = userId

	room.RoomTitle = title

	// Simpan data ke Firestore
	_, err := roomRef.Set(ctx, room)
	if err != nil {
		return nil, err
	}

	// Kembalikan objek Room dengan RoomID yang di-generate Firestore
	return &models.Room{
		RoomID:    roomRef.ID, // Firestore-generated ID
		UserID:    room.UserID,
		RoomTitle: room.RoomTitle,
		CreatedAt: room.CreatedAt,
	}, nil
}

//get all room chat

func (s *RoomService) GetRooms(ctx context.Context, userId string) ([]*models.Room, error) {
	var rooms []*models.Room

	print(userId)

	// Query rooms collection
	roomDocs, err := s.FirestoreClient.Collection("users").Doc(userId).Collection("rooms").Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	print(roomDocs)

	// Iterate over room documents
	for _, doc := range roomDocs {
		var room models.Room
		if err := doc.DataTo(&room); err != nil {
			return nil, err
		}

		room.RoomID = doc.Ref.ID

		// Append room to list
		rooms = append(rooms, &room)
	}

	return rooms, nil
}

//get room by id

func (s *RoomService) GetRoomByID(ctx context.Context, userId, roomId string) (*models.RoomWithChat, error) {
	// Query room document
	roomDoc, err := s.FirestoreClient.Collection("users").Doc(userId).Collection("rooms").Doc(roomId).Get(ctx)
	if err != nil {
		return nil, utils.NewCustomError(http.StatusNotFound, "Room not found")
	}

	// Convert Firestore document to Room struct
	var room models.Room
	if err := roomDoc.DataTo(&room); err != nil {
		return nil, err
	}

	// Query chats collection in the room
	chatsSnapshot, err := s.FirestoreClient.Collection("users").Doc(userId).Collection("rooms").Doc(roomId).Collection("chats").OrderBy("CreatedAt", firestore.Asc).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	// Convert chat documents to Chat structs
	var chats []models.Chat
	for _, chatDoc := range chatsSnapshot {
		var chat models.Chat
		if err := chatDoc.DataTo(&chat); err != nil {
			return nil, err
		}
		chats = append(chats, chat)
	}

	// Return room with chats
	return &models.RoomWithChat{
		Room:  room,
		Chats: chats,
	}, nil
}
