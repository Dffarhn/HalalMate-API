package models

type Place struct {
	Title         string `json:"title"`
	Rating        string `json:"rating"`
	ReviewCount   string `json:"review_count"`
	PriceRange    string `json:"price_range"`
	Category      string `json:"category"`
	OpeningStatus string `json:"opening_status"`
	ImageURL      string `json:"image_url"`
	MapsLink      string `json:"maps_link"`
	MenuLink      string `json:"menu_link"`
	Reviews       []string `json:"reviews"`
}
