package services

import (
	"HalalMate/config/database"
	"HalalMate/config/environment"
	"HalalMate/utils"
	"context"
	"crypto/rand"
	"errors"
	"log"
	"math/big"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/idtoken"
)

// AuthService provides authentication functions using Firestore
type AuthService struct {
	FirestoreClient *firestore.Client
}

// NewAuthService initializes AuthService with Firestore client
func NewAuthService() *AuthService {
	return &AuthService{
		FirestoreClient: database.GetFirestoreClient(),
	}
}

func generateUUID() (string, error) {
	newUUID, err := uuid.NewRandom()
	if err != nil {
		return "", errors.New("failed to generate UUID")
	}
	return newUUID.String(), nil
}

// Register creates a new user in Firestore
func (s *AuthService) Register(email, username, password string, fcmToken string) (*firestore.DocumentRef, string, error) {
	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}

	userID, err := generateUUID()
	if err != nil {
		return nil, "", err
	}

	// Create user in Firestore
	userRef := s.FirestoreClient.Collection("users").Doc(userID)
	_, err = userRef.Set(context.Background(), map[string]interface{}{
		"id":        userRef.ID,
		"email":     email,
		"username":  username,
		"password":  string(hashedPassword),
		"CreatedAt": firestore.ServerTimestamp,
		"UpdatedAt": firestore.ServerTimestamp,
		"fcmToken":  fcmToken,
	})
	if err != nil {
		return nil, "", err
	}

	// Generate JWT token
	token, err := utils.GenerateToken(userRef.ID) // Use email or UID as payload
	if err != nil {
		return nil, "", err
	}

	return userRef, token, nil
}

func (s *AuthService) Login(email string, password string) (string, error) {
	log.Printf("[DEBUG] Searching for user: %s", email)

	// Query Firestore for user by email
	query := s.FirestoreClient.Collection("users").Where("email", "==", email).Limit(1)
	docs, err := query.Documents(context.Background()).GetAll()
	if err != nil {
		log.Println("[ERROR] Firestore query failed:", err)
		return "", errors.New("internal server error")
	}
	if len(docs) == 0 {
		log.Println("[WARNING] User not found:", email)
		return "", errors.New("invalid email or password")
	}

	// Get user document
	docSnap := docs[0]
	userData := docSnap.Data()

	log.Printf("[DEBUG] User found: %+v", userData)

	// ✅ Check if user is a Google User
	if isGoogleUser, exists := userData["isGoogleUser"].(bool); exists && isGoogleUser {
		log.Println("[WARNING] User attempted to login with password but is a Google user:", email)
		return "", errors.New("you have previously signed in with Google, please log in using Google")
	}

	// ✅ Extract stored password
	storedPassword, ok := userData["password"].(string)
	if !ok || storedPassword == "" {
		log.Println("[ERROR] Password field missing in Firestore document")
		return "", errors.New("password not found")
	}

	// ✅ Compare hashed password
	err = bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(password))
	if err != nil {
		log.Println("[WARNING] Password mismatch for user:", email)
		return "", errors.New("invalid password")
	}

	// ✅ Generate JWT token
	token, err := utils.GenerateToken(userData["id"].(string))
	if err != nil {
		log.Println("[ERROR] JWT token generation failed:", err)
		return "", err
	}

	log.Println("[INFO] Login successful, token generated")
	return token, nil
}

func (s *AuthService) VerifyGoogleIDToken(idToken string) (string, error) {
	ctx := context.Background()
	audience := environment.GetWebClientId()

	// ✅ Validate Google ID Token
	payload, err := idtoken.Validate(ctx, idToken, audience)
	if err != nil {
		return "", errors.New("invalid ID token")
	}

	// ✅ Extract email
	email, ok := payload.Claims["email"].(string)
	if !ok {
		return "", errors.New("email not found in token")
	}

	// ✅ Extract display name
	name, ok := payload.Claims["name"].(string)
	if !ok {
		name = strings.Split(email, "@")[0]
	}

	// ✅ Check if user already exists in Firestore
	query := s.FirestoreClient.Collection("users").Where("email", "==", email).Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return "", errors.New("internal server error")
	}

	// ✅ If user exists, return JWT
	if len(docs) > 0 {
		userData := docs[0].Data()
		tokenJWT, err := utils.GenerateToken(userData["id"].(string))
		if err != nil {
			return "", err
		}
		return tokenJWT, nil
	}

	// ✅ New User: Generate UUID
	userID, err := generateUUID()
	if err != nil {
		return "", err
	}

	// ✅ Store new Google user in Firestore
	userRef := s.FirestoreClient.Collection("users").Doc(userID)
	_, err = userRef.Set(ctx, map[string]interface{}{
		"id":           userRef.ID,
		"email":        email,
		"username":     name,
		"isGoogleUser": true, // Mark as Google user
		"CreatedAt":    firestore.ServerTimestamp,
		"UpdatedAt":    firestore.ServerTimestamp,
	})
	if err != nil {
		return "", err
	}

	// ✅ Generate JWT token
	tokenJWT, err := utils.GenerateToken(userRef.ID)
	if err != nil {
		return "", err
	}

	log.Printf("[INFO] New Google user registered: %s", email)
	return tokenJWT, nil
}

// store fcm service
func (s *AuthService) StoreFCMToken(userID, fcmToken string) error {
	// Update user document with FCM token
	userRef := s.FirestoreClient.Collection("users").Doc(userID)
	_, err := userRef.Set(context.Background(), map[string]interface{}{
		"fcmToken": fcmToken,
	}, firestore.MergeAll)
	if err != nil {
		return err

	}
	return nil
}

// GenerateRandomPassword generates a random password for the user
func GenerateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var password strings.Builder
	for i := 0; i < length; i++ {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		password.WriteByte(charset[randomIndex.Int64()])
	}
	return password.String(), nil
}
