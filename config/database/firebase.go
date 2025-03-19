package database

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	"firebase.google.com/go"
	"firebase.google.com/go/auth"
	"google.golang.org/api/option"
)

var FirebaseApp *firebase.App
var FirestoreClient *firestore.Client
var AuthClient       *auth.Client

// InitFirebase initializes both Firestore and Storage clients
func InitFirebase() {
	ctx := context.Background()
	// Initialize Firestore client
	firestoreOpt := option.WithCredentialsFile("halalmate-db-firebase-adminsdk-fbsvc-e090138059.json")
	app, err := firebase.NewApp(ctx, nil, firestoreOpt)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase Firestore: %v", err)
	}
	FirebaseApp = app
	log.Println("Firebase Firestore initialized successfully")

	// Initialize Firestore client
	FirestoreClient, err = app.Firestore(ctx)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}

	AuthClient, err = app.Auth(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase Auth client: %v", err)
	}
	log.Println("Firebase Auth initialized successfully")

}

// GetFirestoreClient returns the Firestore client instance
func GetFirestoreClient() *firestore.Client {
	return FirestoreClient
}


func GetFirebaseAuthClient() *auth.Client{
	return AuthClient
}
