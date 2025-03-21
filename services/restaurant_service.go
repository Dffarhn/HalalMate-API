package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"HalalMate/utils"
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/mmcloughlin/geohash"
	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/type/latlng"
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

const earthRadiusKm = 6371.0 // Radius of Earth in km

// Haversine formula to calculate distance between two lat/lng points
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := (lat2 - lat1) * (math.Pi / 180.0)
	dLon := (lon2 - lon1) * (math.Pi / 180.0)

	lat1 = lat1 * (math.Pi / 180.0)
	lat2 = lat2 * (math.Pi / 180.0)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1)*math.Cos(lat2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

//get restaurant by doc id

func (s *RestaurantService) GetRestaurantByIdAndLocation(ctx context.Context, docID string, latitude, longitude float64, userId string) (map[string]interface{}, error) {
	doc, err := s.FirestoreClient.Collection("restaurants").Doc(docID).Get(ctx)
	if err != nil {
		return nil, utils.NewCustomError(http.StatusNotFound, "Restaurant not found")
	}

	restaurant := make(map[string]interface{})
	doc.DataTo(&restaurant)

	geoPoint, ok := doc.Data()["location"].(*latlng.LatLng)
	if !ok {
		return nil, fmt.Errorf("error getting location data")
	}

	// Calculate distance
	distance := haversine(latitude, longitude, geoPoint.Latitude, geoPoint.Longitude)
	restaurant["distance"] = distance

	// 🚀 Use batch fetch to check bookmark status
	bookmarkedMap, err := s.GetBookmarkedRestaurants(ctx, userId, []string{docID})
	if err != nil {
		return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to get bookmarks")
	}

	restaurant["isBookmarked"] = bookmarkedMap[docID]

	return restaurant, nil
}


func (s *RestaurantService) GetRestaurantByID(ctx context.Context, docID string) (map[string]interface{}, error) {
	doc, err := s.FirestoreClient.Collection("restaurants").Doc(docID).Get(ctx)
	if err != nil {
		return nil, utils.NewCustomError(http.StatusNotFound, "Restaurant not found")
	}

	restaurant := make(map[string]interface{})
	doc.DataTo(&restaurant)


	return restaurant, nil
}


func (c *RestaurantService) GetRestaurantsByIDs(ctx context.Context, restaurantIDs []string,latitude, longitude float64) ([]map[string]interface{}, error) {
	// Firestore `In` query to fetch all restaurants in one go
	iter := c.FirestoreClient.Collection("restaurants").Where("id", "in", restaurantIDs).Documents(ctx)
	var restaurants []map[string]interface{}

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		restaurant := make(map[string]interface{})
		if err := doc.DataTo(&restaurant); err != nil {
			return nil, err
		}

		geoPoint, ok := doc.Data()["location"].(*latlng.LatLng)
		if !ok {
			return nil, fmt.Errorf("error getting location data")
		}
	
		// Calculate distance
		distance := haversine(latitude, longitude, geoPoint.Latitude, geoPoint.Longitude)
		restaurant["distance"] = distance
	
		restaurant["bookmark"] = true
		restaurants = append(restaurants, restaurant)
	}

	return restaurants, nil
}



// function to save restaurant to firestore

func (s *RestaurantService) SaveRestaurant(ctx context.Context, restaurant *models.Place) error {
	geoHash := geohash.Encode(restaurant.Location.Latitude, restaurant.Location.Longitude)
	cleanedAddress := strings.TrimPrefix(restaurant.Address, "Alamat: ")

	// Create a new document reference with an auto-generated ID
	docRef := s.FirestoreClient.Collection("restaurants").NewDoc()

	// Convert GeoLocation to Firestore GeoPoint
	data := map[string]interface{}{
		"id":             docRef.ID, // Store the document ID
		"title":          restaurant.Title,
		"rating":         restaurant.Rating,
		"address":        cleanedAddress,
		"location":       &latlng.LatLng{Latitude: restaurant.Location.Latitude, Longitude: restaurant.Location.Longitude},
		"geohash":        geoHash,
		"price_range":    restaurant.PriceRange,
		"category":       restaurant.Category,
		"opening_status": restaurant.OpeningStatus,
		"image_url":      restaurant.ImageURL,
		"maps_link":      restaurant.MapsLink,
		"menu_link":      restaurant.MenuLink,
		"reviews":        restaurant.Reviews,
		"menu":           restaurant.Menu,
		"review_count":   restaurant.ReviewCount,
	}

	// Save the document with the generated ID
	_, err := docRef.Set(ctx, data)
	if err != nil {
		return err
	}
	return nil
}

func (s *RestaurantService) SaveRestaurants(ctx context.Context, restaurants []*models.Place) error {
	batch := s.FirestoreClient.Batch()

	for _, restaurant := range restaurants {
		docRef := s.FirestoreClient.Collection("restaurants").NewDoc()

		geoHash := geohash.Encode(restaurant.Location.Latitude, restaurant.Location.Longitude)
		cleanedAddress := strings.TrimPrefix(restaurant.Address, "Alamat: ")

		// Convert GeoLocation to Firestore GeoPoint
		data := map[string]interface{}{
			"id":             docRef.ID, // Store Firestore document ID
			"title":          restaurant.Title,
			"rating":         restaurant.Rating,
			"address":        cleanedAddress,
			"location":       &latlng.LatLng{Latitude: restaurant.Location.Latitude, Longitude: restaurant.Location.Longitude},
			"geohash":        geoHash,
			"price_range":    restaurant.PriceRange,
			"category":       restaurant.Category,
			"opening_status": restaurant.OpeningStatus,
			"image_url":      restaurant.ImageURL,
			"maps_link":      restaurant.MapsLink,
			"menu_link":      restaurant.MenuLink,
			"reviews":        restaurant.Reviews,
			"menu":           restaurant.Menu,
			"review_count":   restaurant.ReviewCount,
		}

		// Add the set operation to the batch
		batch.Set(docRef, data)
	}

	// Commit the batch operation
	_, err := batch.Commit(ctx)
	if err != nil {
		return err
	}
	return nil
}


//function to check if restaurant exists on database by on lat and long

func (s *RestaurantService) CheckRestaurantExists(ctx context.Context, latitude, longitude float64, title string) (bool, error) {
	// Generate geohash for the given location
	targetGeoHash := geohash.Encode(latitude, longitude)

	// Get a list of nearby geohashes (small variations)
	geohashPrefix := targetGeoHash[:5] // Use only the first 5 characters for a 3 km range

	// Query Firestore using geohash prefix and title
	iter := s.FirestoreClient.Collection("restaurants").
		Where("geohash", ">=", geohashPrefix).
		Where("geohash", "<=", geohashPrefix+"~").
		Where("title", "==", title).
		Documents(ctx)

	// Get first document
	_, err := iter.Next()
	if err == iterator.Done {
		return false, nil // No document found
	}
	if err != nil {
		return false, err // Return error if something goes wrong
	}

	return true, nil // Restaurant exists
}


func (s *RestaurantService) GetAllRestaurantByLocation(ctx context.Context, latitude, longitude float64, userId string) ([]map[string]interface{}, error) {
	// Generate geohash for the given location
	targetGeoHash := geohash.Encode(latitude, longitude)
	geohashPrefix := targetGeoHash[:5] // 3 km range

	// Query Firestore using geohash prefix
	iter := s.FirestoreClient.Collection("restaurants").
		Where("geohash", ">=", geohashPrefix).
		Where("geohash", "<=", geohashPrefix+"~").
		Documents(ctx)

	var restaurants []map[string]interface{}
	var restaurantIDs []string

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to get restaurants")
		}

		data := doc.Data()
		docID := doc.Ref.ID
		geoPoint, ok := data["location"].(*latlng.LatLng)
		if !ok {
			continue
		}

		// Haversine filter
		distance := haversine(latitude, longitude, geoPoint.Latitude, geoPoint.Longitude)
		if distance <= 3.0 {
			restaurant := make(map[string]interface{})
			doc.DataTo(&restaurant)
			restaurant["distance"] = distance

			restaurants = append(restaurants, restaurant)
			restaurantIDs = append(restaurantIDs, docID) // Collect restaurant IDs for batch bookmark check
		}
	}

	// 🚀 **Batch check bookmarks in ONE query**
	bookmarkedMap, err := s.GetBookmarkedRestaurants(ctx, userId, restaurantIDs)
	if err != nil {
		return nil, utils.NewCustomError(http.StatusInternalServerError, "Failed to get bookmarks")
	}

	// Assign bookmark status to restaurants
	for i := range restaurants {
		docID := restaurantIDs[i]
		restaurants[i]["isBookmarked"] = bookmarkedMap[docID] // Set bookmark status
	}

	// Sort by distance
	sort.Slice(restaurants, func(i, j int) bool {
		return restaurants[i]["distance"].(float64) < restaurants[j]["distance"].(float64)
	})

	return restaurants, nil
}

// 🚀 **Optimized function: Get all bookmarked restaurants in ONE query**
func (s *RestaurantService) GetBookmarkedRestaurants(ctx context.Context, userID string, restaurantIDs []string) (map[string]bool, error) {
	if len(restaurantIDs) == 0 {
		return map[string]bool{}, nil
	}

	bookmarkedMap := make(map[string]bool)
	batchSize := 10 // Firestore allows max 10 IDs per "IN" query

	for i := 0; i < len(restaurantIDs); i += batchSize {
		end := i + batchSize
		if end > len(restaurantIDs) {
			end = len(restaurantIDs)
		}

		iter := s.FirestoreClient.Collection("users").Doc(userID).Collection("bookmarks").
			Where("restaurantId", "in", restaurantIDs[i:end]).
			Documents(ctx)

		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}

			restaurantID, ok := doc.Data()["restaurantId"].(string)
			if ok {
				bookmarkedMap[restaurantID] = true
			}
		}
	}

	return bookmarkedMap, nil
}

