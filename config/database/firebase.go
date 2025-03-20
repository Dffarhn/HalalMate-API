package database

import (
	"HalalMate/config/environment"
	"context"
	"encoding/json"
	"log"

	"cloud.google.com/go/firestore"
	"firebase.google.com/go"
	"firebase.google.com/go/auth"
	"google.golang.org/api/option"
)

var (
	FirebaseApp     *firebase.App
	FirestoreClient *firestore.Client
	AuthClient      *auth.Client
)

// InitFirebase initializes Firestore and Auth clients
func InitFirebase() {
	ctx := context.Background()

	// Get FIREBASE_CREDENTIALS from environment variables
	credentialsJSON := environment.GetFirebaseKey()

	var app *firebase.App
	var err error

	//Firebase configuration
	config := &firebase.Config{
		ProjectID: environment.GetFirebaseProjectID(),
	}

	var opt option.ClientOption

	if credentialsJSON == "" {
		log.Println("Warning: FIREBASE_CREDENTIALS is not set. Using default credentials.")
		opt = option.WithoutAuthentication() // Use default credentials if running in GCP environment
	} else {
		// Validate JSON format
		if !isValidJSON(credentialsJSON) {
			log.Fatalf("Error: Invalid FIREBASE_CREDENTIALS JSON")
		}
		opt = option.WithCredentialsJSON([]byte(credentialsJSON))
	}

	// Initialize Firebase App
	app, err = firebase.NewApp(ctx, config, opt)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase: %v", err)
	}

	FirebaseApp = app
	log.Println("Firebase initialized successfully")

	// Initialize Firestore client
	FirestoreClient, err = app.Firestore(ctx)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}

	// Initialize Firebase Auth client
	AuthClient, err = app.Auth(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase Auth client: %v", err)
	}

	log.Println("Firestore and Firebase Auth initialized successfully")
}

// GetFirestoreClient returns the Firestore client instance
func GetFirestoreClient() *firestore.Client {
	return FirestoreClient
}

// GetFirebaseAuthClient returns the Firebase Auth client instance
func GetFirebaseAuthClient() *auth.Client {
	return AuthClient
}

// isValidJSON checks if a string is a valid JSON format
func isValidJSON(str string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(str), &js) == nil
}
