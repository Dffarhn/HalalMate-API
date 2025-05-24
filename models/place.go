package models

type Place struct {
	Title         string      `json:"title"`
	Rating        string      `json:"rating"`
	Address       string      `json:"address"`
	ReviewCount   string      `json:"review_count"`
	Location      GeoLocation `json:"location"`
	PriceRange    string      `json:"price_range"`
	Category      string      `json:"category"`
	OpeningStatus string      `json:"opening_status"`
	ImageURL      string      `json:"image_url"`
	MapsLink      string      `json:"maps_link"`
	MenuLink      []string    `json:"menu_link"`
	Reviews       []string    `json:"reviews"`
	Menu          []MenuItem  `json:"menu"`
}

type GeoLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type MenuItem struct {
	SubMenu  string     `json:"sub_menu"`
	MenuList []MenuList `json:"menu_list"`
}

type MenuList struct {
	Name  string `json:"name"`
	Price int64 `json:"price"`
}

type AIResponsAnalyzeMenu struct {
	HalalStatus string       `json:"halal_status"` // "halal" or "haram"
	Menu        []MenuItem   `json:"menu"`
}
