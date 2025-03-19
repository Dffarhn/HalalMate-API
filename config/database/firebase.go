package database

import (
	"HalalMate/config/environment"
	"context"
	"encoding/base64"
	"log"

	"cloud.google.com/go/firestore"
	"firebase.google.com/go"
	"firebase.google.com/go/auth"
	// "github.com/joho/godotenv"
	"google.golang.org/api/option"
)

var FirebaseApp *firebase.App
var FirestoreClient *firestore.Client
var AuthClient       *auth.Client

// InitFirebase initializes both Firestore and Storage clients
func InitFirebase() {
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal("Error loading .env file")
	// }

	// Get Base64 encoded credentials from env
	encodedCredentials := environment.GetFirebaseKey();
	if encodedCredentials == "" {
		log.Fatal("FIREBASE_CREDENTIALS_BASE64 environment variable is missing")
	}

	// Decode Base64 string
	decodedCredentials, err := base64.StdEncoding.DecodeString(encodedCredentials)
	if err != nil {
		log.Fatalf("Failed to decode Firebase credentials: %v", err)
	}

	// Get Project ID from environment variables
	projectID := environment.GetFirebaseProjectID()
	if projectID == "" {
		log.Fatal("FIREBASE_PROJECT_ID environment variable is missing")
	}
	
	ctx := context.Background()
	// Initialize Firestore client
	firestoreOpt := option.WithCredentialsJSON(decodedCredentials)

	config := &firebase.Config{
		ProjectID: projectID,
	}
	app, err := firebase.NewApp(ctx, config, firestoreOpt)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase Firestore: %v", err)
	}
	FirebaseApp = app
	log.Println("Firebase Firestore initialized successfully")


	// Initialize Firestore client
	FirestoreClient, err = 	app.Firestore(ctx)
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
