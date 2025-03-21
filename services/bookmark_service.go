package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"HalalMate/utils"
	"context"
	"log"
	"net/http"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type BookmarkService struct {
	FirestoreClient   *firestore.Client
	RestaurantService *RestaurantService
}

// NewBookmarkService initializes a new BookmarkService
func NewBookmarkService() *BookmarkService {
	return &BookmarkService{
		FirestoreClient:   database.GetFirestoreClient(),
		RestaurantService: NewRestaurantService(),
	}
}

// GetAllBookmarks retrieves all bookmarks for a user
func (b *BookmarkService) GetAllBookmarks(ctx context.Context, userID string, latitude, longitude float64) ([]models.Bookmark, error) {
	log.Printf("Fetching bookmarks for user: %s", userID)

	iter := b.FirestoreClient.Collection("users").Doc(userID).Collection("bookmarks").Documents(ctx)
	var bookmarks []models.Bookmark
	var restaurantIDs []string

	// First loop: Fetch bookmarks and collect restaurant IDs
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Error fetching bookmarks: %v", err)
			return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to fetch bookmarks")
		}

		var bookmark models.Bookmark
		if err := doc.DataTo(&bookmark); err != nil {
			log.Printf("Error parsing bookmark data: %v", err)
			return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to parse bookmark data")
		}
		bookmark.ID = doc.Ref.ID
		bookmarks = append(bookmarks, bookmark)

		log.Printf("Fetched bookmark: %+v", bookmark)

		// Collect restaurant ID if not empty
		if bookmark.RestaurantID != "" {
			restaurantIDs = append(restaurantIDs, bookmark.RestaurantID)
		}
	}

	log.Printf("Collected Restaurant IDs: %v", restaurantIDs)

	// Fetch all restaurants in one query
	restaurantsMap := make(map[string]map[string]interface{})
	if len(restaurantIDs) > 0 {
		log.Println("Fetching restaurant details for collected IDs...")

		restaurants, err := b.RestaurantService.GetRestaurantsByIDs(ctx, restaurantIDs, latitude,longitude)
		if err != nil {
			log.Printf("Error fetching restaurant details: %v", err)
			return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to fetch restaurant details")
		}

		// Convert list to map for quick lookup
		for _, restaurant := range restaurants {
			if id, ok := restaurant["id"].(string); ok {
				restaurantsMap[id] = restaurant
				log.Printf("Mapped restaurant: %s -> %+v", id, restaurant)
			}
		}
	} else {
		log.Println("No restaurant IDs found, skipping restaurant lookup.")
	}

	// Second loop: Map restaurants to bookmarks
	for i := range bookmarks {
		if restaurant, exists := restaurantsMap[bookmarks[i].RestaurantID]; exists {
			bookmarks[i].Restaurant = restaurant
			log.Printf("Mapped restaurant to bookmark: %+v", bookmarks[i])
		} else {
			bookmarks[i].Restaurant = nil
			log.Printf("No matching restaurant found for bookmark ID: %s", bookmarks[i].ID)
		}
	}

	log.Println("Successfully fetched all bookmarks.")
	return bookmarks, nil
}


// PostBookmark adds a new bookmark for a user
func (b *BookmarkService) PostBookmark(ctx context.Context, userID string, bookmark models.Bookmark) (*models.Bookmark, error) {
	restaurantService := NewRestaurantService()
	_, err := restaurantService.GetRestaurantByID(ctx, bookmark.RestaurantID)
	if err != nil {
		return nil, utils.NewCustomError(http.StatusNotFound, "Restaurant not found")
	}

	iter := b.FirestoreClient.Collection("users").Doc(userID).Collection("bookmarks").
		Where("restaurantId", "==", bookmark.RestaurantID).Limit(1).Documents(ctx)

	_, err = iter.Next()
	if err == nil {
		return nil, utils.NewCustomError(http.StatusConflict, "Bookmark already exists")
	} else if err != iterator.Done {
		return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to check existing bookmarks")
	}

	docRef, _, err := b.FirestoreClient.Collection("users").Doc(userID).Collection("bookmarks").Add(ctx, bookmark)
	if err != nil {
		return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to add bookmark")
	}

	bookmark.ID = docRef.ID
	_, err = docRef.Set(ctx, map[string]interface{}{
		"id":           bookmark.ID,
		"userId":       bookmark.UserID,
		"restaurantId": bookmark.RestaurantID,
		"createdAt":    bookmark.CreatedAt,
	}, firestore.MergeAll)

	if err != nil {
		return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to update bookmark with ID")
	}

	return &bookmark, nil
}

// GetBookmarkByID retrieves a single bookmark by its ID
func (b *BookmarkService) GetBookmarkByID(ctx context.Context, userID string, bookmarkID string) (*models.Bookmark, error) {
	doc, err := b.FirestoreClient.Collection("users").Doc(userID).Collection("bookmarks").Doc(bookmarkID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, utils.NewCustomError(http.StatusNotFound, "Bookmark not found")
		}
		return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to fetch bookmark data")
	}

	var bookmark models.Bookmark
	if err := doc.DataTo(&bookmark); err != nil {
		return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to parse bookmark data")
	}

	bookmark.ID = doc.Ref.ID

	// Fetch associated restaurant details
	restaurant, err := b.RestaurantService.GetRestaurantByID(ctx, bookmark.RestaurantID)
	if err != nil {
		// Jika error adalah 404 (restoran tidak ditemukan), tetap lanjutkan tanpa restoran
		if customErr, ok := err.(*utils.CustomError); ok && customErr.StatusCode == http.StatusNotFound {
			bookmark.Restaurant = nil
		} else {
			return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to fetch restaurant details")
		}
	} else {
		bookmark.Restaurant = restaurant
	}

	return &bookmark, nil
}

// DeleteBookmark removes a bookmark by restaurantId
func (b *BookmarkService) DeleteBookmark(ctx context.Context, userID string, restaurantID string) error {
	collectionRef := b.FirestoreClient.Collection("users").Doc(userID).Collection("bookmarks")

	// Search for the bookmark with the given restaurantId
	query := collectionRef.Where("restaurantId", "==", restaurantID).Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return utils.NewCustomError(http.StatusInternalServerError, "Failed to query bookmarks")
	}

	// If no bookmark found, return error
	if len(docs) == 0 {
		return utils.NewCustomError(http.StatusNotFound, "Bookmark not found")
	}

	// Get the first document (since we used Limit(1))
	docRef := docs[0].Ref

	// Delete the bookmark
	_, err = docRef.Delete(ctx)
	if err != nil {
		return utils.NewCustomError(http.StatusInternalServerError, "Failed to delete bookmark")
	}

	return nil
}
