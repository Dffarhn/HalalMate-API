package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"HalalMate/utils"
	"context"
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
func (b *BookmarkService) GetAllBookmarks(ctx context.Context, userID string) ([]models.Bookmark, error) {
	iter := b.FirestoreClient.Collection("users").Doc(userID).Collection("bookmarks").Documents(ctx)
	var bookmarks []models.Bookmark

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to fetch bookmarks")
		}

		var bookmark models.Bookmark
		if err := doc.DataTo(&bookmark); err != nil {
			return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to parse bookmark data")
		}
		bookmark.ID = doc.Ref.ID

		// Ambil data restoran terkait
		bookmark.Restaurant, err = b.RestaurantService.GetRestaurantByID(ctx, bookmark.RestaurantID)
		if err != nil {
			// Jika restoran tidak ditemukan, tetap lanjutkan tanpa return error
			if customErr, ok := err.(*utils.CustomError); ok && customErr.StatusCode == http.StatusNotFound {
				bookmark.Restaurant = nil // Atur sebagai nil agar tidak mengganggu response utama
			} else {
				return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to fetch restaurant details")
			}
		}

		bookmarks = append(bookmarks, bookmark)
	}

	// Jika tidak ada bookmark yang ditemukan, kembalikan error 404
	// if len(bookmarks) == 0 {
	// 	return nil, utils.NewCustomError(http.StatusNotFound, "No bookmarks found")
	// }

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
