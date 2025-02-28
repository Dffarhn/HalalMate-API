package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"context"

	"cloud.google.com/go/firestore"
)

type RestaurantService struct {
	FirestoreClient *firestore.Client
}

// NewRestaurantService initializes RestaurantService with Firestore and OpenAI service
func NewRestaurantService() *RestaurantService {
	return &RestaurantService{
		FirestoreClient: database.GetFirestoreClient(),
	}
}


// function to save restaurant to firestore

func (s *RestaurantService) SaveRestaurant(ctx context.Context, restaurant *models.Place) error {
	_, _, err := s.FirestoreClient.Collection("restaurants").Add(ctx, restaurant)
	if err != nil {
		return err
	}
	return nil
}
