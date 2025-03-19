package models

import "time"

// Bookmark represents a bookmark entity
type Bookmark struct {
	ID           string                 `json:"id"`
	UserID       string                 `firestore:"userId"`
	RestaurantID string                 `firestore:"restaurantId"`
	Restaurant   map[string]interface{} `json:"Restaurant"`
	CreatedAt    time.Time              `firestore:"createdAt"`
}
