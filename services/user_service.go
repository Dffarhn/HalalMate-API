package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"context"

	"cloud.google.com/go/firestore"
)

type UserService struct {
	FirestoreClient *firestore.Client
}

// NewUserService initializes UserService with FirestoreClient
func NewUserService() *UserService {
	return &UserService{
		FirestoreClient: database.GetFirestoreClient(),
	}
}

//profile service

func (s *UserService) GetUserProfile(ctx context.Context, userId string) (interface{}, error) {
	doc, err := s.FirestoreClient.Collection("users").Doc(userId).Get(ctx)
	if err != nil {
		return nil, err
	}

	//before return parsing to model profile
	var profile models.Profile
	doc.DataTo(&profile)
	return profile, nil

	// return doc.Data(), nil
}
