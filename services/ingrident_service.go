package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"context"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type IngridentService struct {
	FirestoreClient *firestore.Client
}

func NewIngridentService() *IngridentService {
	return &IngridentService{
		FirestoreClient: database.GetFirestoreClient(),
	}
}

//save room into firebase

func (s *IngridentService) SaveIngrident(ctx context.Context, name string) (*string, error) {

	// Buat dokumen baru di Firestore (Firestore akan otomatis generate ID)
	ingridentRef := s.FirestoreClient.Collection("ingridients").NewDoc()

	// Simpan data room ke dalam dokumen
	_, err := ingridentRef.Set(ctx, map[string]interface{}{
		"name": name,
	})
	if err != nil {
		return nil, err
	}
	return &ingridentRef.ID, nil

}

//get all room chat

// Correct GetAllIngridients method
func (s *IngridentService) GetAllIngridients(ctx context.Context) ([]*models.Ingrident, error) {
	iter := s.FirestoreClient.Collection("ingridients").Documents(ctx)
	defer iter.Stop()

	var ingridents []*models.Ingrident

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}

		var ingrident models.Ingrident
		err = doc.DataTo(&ingrident)
		if err != nil {
			return nil, err
		}
		ingridents = append(ingridents, &ingrident)
	}

	return ingridents, nil
}
