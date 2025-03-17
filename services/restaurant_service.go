package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"context"
	"fmt"
	"math"
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

func (s *RestaurantService) GetAllRestaurantByLocation(ctx context.Context, latitude, longitude float64) ([]map[string]interface{}, error) {
	// Generate geohash for the given location
	targetGeoHash := geohash.Encode(latitude, longitude)

	// Get a list of nearby geohashes (small variations)
	geohashPrefix := targetGeoHash[:5] // Use only the first 5 characters for a 3 km range

	// Query Firestore using geohash prefix
	iter := s.FirestoreClient.Collection("restaurants").
		Where("geohash", ">=", geohashPrefix).
		Where("geohash", "<=", geohashPrefix+"~"). // "~" ensures we get similar prefixes
		Documents(ctx)

	var restaurants []map[string]interface{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		docId := doc.Ref.ID

		// Extract location data
		data := doc.Data()

		// fmt.Println(data)
		geoPoint, ok := data["location"].(*latlng.LatLng)

		// fmt.Println(geoPoint)
		if !ok {
			continue
		}

		// Optional: Apply Haversine filter to ensure accuracy
		distance := haversine(latitude, longitude, geoPoint.Latitude, geoPoint.Longitude)
		if distance <= 3.0 {
			restaurant := make(map[string]interface{})
			doc.DataTo(&restaurant)
			restaurant["id"] = docId
			fmt.Println(docId)
			restaurant["distance"] = distance
			restaurants = append(restaurants, restaurant)
		}
	}

	//sort by distance
	sort.Slice(restaurants, func(i, j int) bool {
		return restaurants[i]["distance"].(float64) < restaurants[j]["distance"].(float64)
	})

	return restaurants, nil
}

//get restaurant by doc id

func (s *RestaurantService) GetRestaurantByIdAndLocation(ctx context.Context, docID string, latitude, longitude float64) (map[string]interface{}, error) {
	doc, err := s.FirestoreClient.Collection("restaurants").Doc(docID).Get(ctx)
	if err != nil {
		return nil, err
	}

	restaurant := make(map[string]interface{})
	doc.DataTo(&restaurant)
	restaurant["id"] = docID

	// fmt.Println(data)
	geoPoint, ok := doc.Data()["location"].(*latlng.LatLng)

	// fmt.Println(geoPoint)
	if !ok {
		return nil, fmt.Errorf("error getting location data")
	}

	// Optional: Apply Haversine filter to ensure accuracy
	distance := haversine(latitude, longitude, geoPoint.Latitude, geoPoint.Longitude)
	doc.DataTo(&restaurant)
	restaurant["distance"] = distance

	return restaurant, nil
}

func (s *RestaurantService) GetRestaurantByID(ctx context.Context, docID string) (map[string]interface{}, error) {
	doc, err := s.FirestoreClient.Collection("restaurants").Doc(docID).Get(ctx)
	if err != nil {
		return nil, err
	}

	restaurant := make(map[string]interface{})
	doc.DataTo(&restaurant)
	restaurant["id"] = docID

	return restaurant, nil
}

// function to save restaurant to firestore

func (s *RestaurantService) SaveRestaurant(ctx context.Context, restaurant *models.Place) error {

	geoHash := geohash.Encode(restaurant.Location.Latitude, restaurant.Location.Longitude)

	cleanedAddress := strings.TrimPrefix(restaurant.Address, "Alamat: ")

	// Convert GeoLocation to Firestore GeoPoint
	data := map[string]interface{}{
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

	_, _, err := s.FirestoreClient.Collection("restaurants").Add(ctx, data)
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
