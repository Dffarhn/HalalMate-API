package services

import (
	"fmt"
	"github.com/openfoodfacts/openfoodfacts-go"
)

// SnackService handles product analysis from OpenFoodFacts
type SnackService struct {
	Client openfoodfacts.Client
}

// NewSnackService initializes a new instance of SnackService
func NewSnackService() *SnackService {
	client := openfoodfacts.NewClient("world", "", "")
	return &SnackService{Client: client}
}

// NutrimentsHaram holds potentially haram-related values from the nutriments
type NutrimentsHaram struct {
	AlcoholValue   float64 `json:"alcohol_value"`
	AlcoholServing float64 `json:"alcohol_serving"`
	AlcoholUnit    string  `json:"alcohol_unit"`
	Alcohol100G    float64 `json:"alcohol_100g"`
	Alcohol        float64 `json:"alcohol"`
}

// ProductDetail is a structured response containing product information
type ProductDetail struct {
	Name            string           `json:"name"`
	IngredientsText string           `json:"ingredients_text"`
	IngredientsIDs  []string         `json:"ingredients_ids"`
	IngredientsTags []string         `json:"ingredients_tags"`
	PotentialHaram  NutrimentsHaram  `json:"potential_haram"`
}

// GetProductByBarcode fetches product details using a barcode
func (s *SnackService) GetProductByBarcode(barcode string) (*ProductDetail, error) {
	product, err := s.Client.Product(barcode)
	if err != nil {
		fmt.Printf("Error fetching product: %v\n", err)
		return nil, nil
	}

	// Produk tidak valid jika nama kosong dan ingredients kosong
	if product.ProductName == "" && product.IngredientsText == "" {
		return nil, nil
	}

	nutriment := product.Nutriments
	nutrimentHaram := NutrimentsHaram{
		AlcoholValue:   nutriment.AlcoholValue,
		AlcoholServing: nutriment.AlcoholServing,
		AlcoholUnit:    nutriment.AlcoholUnit,
		Alcohol100G:    nutriment.Alcohol100G,
		Alcohol:        nutriment.Alcohol,
	}

	detail := &ProductDetail{
		Name:            product.ProductName,
		IngredientsText: product.IngredientsText,
		IngredientsIDs:  product.IngredientsIdsDebug,
		IngredientsTags: product.IngredientsTags,
		PotentialHaram:  nutrimentHaram,
	}

	return detail, nil
}
