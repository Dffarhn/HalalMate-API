package models

type OpenAIRequest struct {
	Type        string `json:"type" binding:"required"`        // "text" atau "image"
	UserPrompt  string `json:"user_prompt" binding:"required"`
	SystemPrompt string `json:"system_prompt"`
	Model       string `json:"model"`
	ImageURL    string `json:"image_url"` // URL gambar untuk OCR
}