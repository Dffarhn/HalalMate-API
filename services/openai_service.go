package services

import (
	"HalalMate/config/environment"
	"HalalMate/models"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
)

// OpenAIService handles image processing with OpenAI API
type OpenAIService struct {
	APIKey string
}

// NewOpenAIService creates a new instance of OpenAIService
func NewOpenAIService() *OpenAIService {
	return &OpenAIService{
		APIKey: environment.GetOpenAIKey(),
	}
}

func (s *OpenAIService) DownloadAndEncodeImages(imageURLs []string) ([]string, error) {
	var encodedImages []string

	for _, imageURL := range imageURLs {
		// Download the image
		resp, err := http.Get(imageURL)
		if err != nil {
			return nil, fmt.Errorf("error downloading image %s: %w", imageURL, err)
		}
		defer resp.Body.Close()

		// Read the image data into memory
		imageData, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading image data %s: %w", imageURL, err)
		}

		// Detect the image format
		imageType := http.DetectContentType(imageData)

		// Encode image to Base64
		encodedImage := base64.StdEncoding.EncodeToString(imageData)
		encodedImages = append(encodedImages, "data:"+imageType+";base64,"+encodedImage)
	}

	return encodedImages, nil
}

func (s *OpenAIService) AnalyzeImages(ctx context.Context, imageUrls []string) ([]models.MenuItem, error) {
	encodedImages, err := s.DownloadAndEncodeImages(imageUrls)
	if err != nil {
		return nil, err
	}

	prompt := "Analyze these images and return a structured JSON output of the menu. Generate submenu categories dynamically. Each menu item should include a name and estimated price. If any item is unclear, return 'N/A'."

	var content []map[string]interface{}
	content = append(content, map[string]interface{}{"type": "text", "text": prompt})
	for _, img := range encodedImages {
		content = append(content, map[string]interface{}{"type": "image_url", "image_url": map[string]string{"url": img}})
	}

	url := "https://api.openai.com/v1/chat/completions"
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]interface{}{
			{
				"role": "system",
				"content": `You are an AI assistant that analyzes images of food menus and returns a structured JSON output. Your response must follow this format:\n\n[
									{\n
										\"sub_menu\": \"Generated category based on analysis\", \n
										\"menu_list\": [\n
											{\"name\": \"Dish name or 'N/A' if unclear\", \"price\": 0}\n
										]\n
									}\n
								]\n\n
								Rules:\n
								1 Extract menu items and group them into relevant submenu categories (e.g., 'Makanan Berat', 'Minuman Dingin', 'Snack').\n
								2 Convert all price formats into integer values in Indonesian Rupiah (IDR) without currency symbols. Example:\n
									- '5K' ➝ 5000
									- 'IDR 2K' ➝ 2000
									- 'Rp 10.500' ➝ 10500
								3 If the price is unclear or missing, return 0 instead of 'N/A'.\n
								4 Do not include text explanations outside the JSON response.\n`,
			},
			{"role": "user", "content": content},
		},
	}

	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// ✅ FIX: Check if API response is valid
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Raw API Response:", string(body)) // ✅ Debugging

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, errors.New("no valid response received")
	}

	fmt.Println("Response Content:", result.Choices[0].Message.Content) // ✅ Debugging

	cleanedJSON := cleanJSONResponse(result.Choices[0].Message.Content)

	fmt.Println("Cleaned JSON:", cleanedJSON) // Debugging

	var menuItems []models.MenuItem
	if err := json.Unmarshal([]byte(cleanedJSON), &menuItems); err != nil {
		fmt.Println("Error:", err)
		return nil, err
	}

	// Print the parsed data
	for _, item := range menuItems {
		fmt.Println("Sub Menu:", item.SubMenu)
		for _, menuItem := range item.MenuList {
			fmt.Printf("- %s: %d\n", menuItem.Name, menuItem.Price)
		}
	}
	return menuItems, nil
}

func cleanJSONResponse(response string) string {
	// Remove markdown code block markers like ```json and ```
	re := regexp.MustCompile("(?s)```(?:json)?(.*?)```")
	cleaned := re.ReplaceAllString(response, "$1")

	// Trim unnecessary whitespace
	return strings.TrimSpace(cleaned)
}

// ChatStream sends a request to OpenAI's API and returns a streaming response
func (s *OpenAIService) ChatStream(ctx context.Context, systemPrompt string, userPrompt string) (io.ReadCloser, error) {
	url := "https://api.openai.com/v1/chat/completions"
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"stream": true, // Enable streaming mode
	}

	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Println("OpenAI API error response:", string(body))
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Return the response stream (caller must close it)
	return resp.Body, nil
}
