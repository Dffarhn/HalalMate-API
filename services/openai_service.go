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
	"os"
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
		resp, err := http.Get(replaceImageQuality(imageURL))
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

// Function to replace any resolution with "s1600-k-no"
func replaceImageQuality(imageURL string) string {
	// Regex to match "s<number>-k-no"
	re := regexp.MustCompile(`s\d+-k-no`)

	// Replace it with "s1600-k-no"
	return re.ReplaceAllString(imageURL, "s1600-k-no")
}

func (s *OpenAIService) AnalyzeImages(ctx context.Context, imageUrls []string) (*models.AIResponsAnalyzeMenu, error) {

	// Create/Open log file
	logFile, err := os.OpenFile("analyze_images.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, fmt.Errorf("error opening log file: %w", err)
	}
	defer logFile.Close()

	// Create a new logger that writes to the file
	logger := log.New(logFile, "", log.LstdFlags)

	// Log function entry
	logger.Println("Starting image analysis process...")
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
				"content": `You are an AI assistant that analyzes images of food menus and returns a structured JSON output. Your response must follow this format:

{
  "halal_status": "halal", // or "haram"
  "menu": [
    {
      "sub_menu": "Generated category based on analysis",
      "menu_list": [
        { "name": "Dish name or 'N/A' if unclear", "price": 0 }
      ]
    }
  ]
}

Rules:
1. Extract menu items and group them into relevant submenu categories like 'Makanan Berat', 'Minuman Dingin', etc.
2. Convert all price formats into integer values in Indonesian Rupiah (IDR). Examples:
   - '5K' ➝ 5000
   - 'IDR 2K' ➝ 2000
   - 'Rp 10.500' ➝ 10500
3. If price is unclear or missing, return 0.
4. Determine halal_status based on whether any item likely contains haram ingredients (e.g., pork, bacon, lard, alcohol).
   - If any haram food is found, set "halal_status": "haram".
   - Otherwise, set "halal_status": "halal".
5. Do not include any explanation outside the JSON response.`,
			},
			{
				"role":    "user",
				"content": content,
			},
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
	// fmt.Println("Raw API Response:", string(body)) // ✅ Debugging
	logger.Printf("Raw API Response: %s", string(body))
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

	var aiResponse models.AIResponsAnalyzeMenu
	if err := json.Unmarshal([]byte(cleanedJSON), &aiResponse); err != nil {
		fmt.Println("Error:", err)
		return nil, err
	}

	// Print the parsed data
	fmt.Println("Halal Status:", aiResponse.HalalStatus)
	for _, menuItem := range aiResponse.Menu {
		fmt.Println("Sub Menu:", menuItem.SubMenu)
		for _, dish := range menuItem.MenuList {
			fmt.Printf("- %s: %d\n", dish.Name, dish.Price)
		}
	}

	return &aiResponse, nil
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

// Chat sends a request to OpenAI's API and returns a non-streaming response
func (s *OpenAIService) Chat(ctx context.Context, systemPrompt string, userPrompt string) (map[string]interface{}, error) {
	url := "https://api.openai.com/v1/chat/completions"
	payload := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		// No "stream": true
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Println("OpenAI API error response:", string(body))
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned in response")
	}

	// Clean markdown if needed
	cleaned := strings.TrimSpace(result.Choices[0].Message.Content)
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %w", err)
	}

	return parsed, nil
}

func (s *OpenAIService) ChatWithVision(ctx context.Context, systemPrompt string, base64Images []string) (map[string]interface{}, error) {
	url := "https://api.openai.com/v1/chat/completions"

	var imageMessages []map[string]interface{}
	for _, b64 := range base64Images {
		imageMessages = append(imageMessages, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": b64,
			},
		})
	}

	var userContent []interface{}

	// Determine prompt based on image count
	if len(base64Images) == 1 {
		userContent = []interface{}{
			map[string]string{
				"type": "text",
				"text": `Kamu adalah pakar analisis kehalalan makanan.

Kamu diberikan **foto kemasan bagian depan** dari sebuah produk makanan. Gambar ini biasanya memuat **nama produk, brand/logo, dan visual tampilan kemasan**.

Tugasmu:
1. Identifikasi **nama produk** dan brand dari gambar depan.
2. Berdasarkan informasi tersebut, **cari data komposisi bahan dari internet**.
3. Analisis status kehalalan produk berdasarkan bahan-bahan tersebut. Fokus pada:
   - Daging babi dan turunannya
   - Alkohol atau bahan hasil fermentasi alkohol
   - Gelatin, enzim, dan bahan hewani yang tidak jelas
   - Bahan sintetis atau kimia yang diragukan (misalnya E-codes)
4. Jika tidak bisa menemukan informasi bahan, jawab "Tidak Dapat Menentukan".

Catatan penting:
- Jangan hanya mengandalkan label halal pada kemasan.
- Jika ada keraguan terhadap bahan, anggap sebagai "Tidak Dapat Menentukan".
- Balasan **HARUS** dalam bentuk **JSON valid dan murni (tanpa markdown)**:

{
  "Status": "Halal" | "Haram" | "Tidak Dapat Menentukan",
  "Reason": "Alasan singkat dan jelas",
  "ProductName": "Nama produk",
  "Suggest": [
    {
      "NamaSugestProduk": "Alternatif halal (jika produk haram)"
    }
  ]
}

Ketentuan tambahan:
- Jika Status = "Halal", maka "Suggest": []
- Jika Status = "Haram", berikan 1-3 alternatif halal yang tersedia di Indonesia
- Jika Status = "Tidak Dapat Menentukan", maka "Suggest": []
`,
			},
			imageMessages[0],
		}

	} else if len(base64Images) >= 2 {
		userContent = []interface{}{
			map[string]string{
				"type": "text",
				"text": `Kamu adalah pakar analisis kehalalan makanan.

Berikut dua gambar kemasan produk:
1. Gambar depan: berisi nama dan tampilan produk.
2. Gambar belakang: berisi daftar bahan, komposisi, dan informasi gizi.

Tugasmu:
- Identifikasi nama produk dari gambar depan.
- Ambil semua informasi bahan dari gambar belakang.
- Analisis kehalalan produk berdasarkan bahan-bahan tersebut. Fokus pada:
  - Daging babi dan turunannya
  - Alkohol atau bahan hasil fermentasi alkohol
  - Gelatin, enzim, dan bahan hewani yang tidak jelas
  - Bahan sintetis atau kimia yang diragukan (misalnya E-codes)

Aturan penting:
- Label halal hanya jadi pendukung, bukan bukti utama.
- Jika gambar tidak cukup jelas untuk menentukan, jawab "Tidak Dapat Menentukan".
- Balas HANYA dalam format JSON valid dan murni (tidak ada markdown atau penjelasan lain):

{
  "Status": "Halal" | "Haram" | "Tidak Dapat Menentukan",
  "Reason": "Alasan singkat dan jelas",
  "ProductName": "Nama produk",
  "Suggest": [
    {
      "NamaSugestProduk": "Alternatif halal (jika produk haram)"
    }
  ]
}

Catatan:
- Jika status = "Halal", maka "Suggest": []
- Jika status = "Haram", beri 1-3 alternatif halal yang tersedia di Indonesia
- Jika "Tidak Dapat Menentukan", maka "Suggest": []`,
			},
			imageMessages[0], // Gambar depan kemasan
			imageMessages[1], // Gambar belakang kemasan
		}

	} else {
		return nil, fmt.Errorf("no images provided")
	}

	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": systemPrompt,
		},
		{
			"role":    "user",
			"content": userContent,
		},
	}

	payload := map[string]interface{}{
		"model":    "gpt-4o", // or "gpt-4-vision-preview"
		"messages": messages,
	}

	jsonData, _ := json.Marshal(payload)
	fmt.Println("Payload:", string(jsonData))

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println("API Error Response:", string(body))
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Raw API Response:", string(body))

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned in response")
	}

	cleaned := strings.TrimSpace(result.Choices[0].Message.Content)
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %w", err)
	}

	return parsed, nil
}

func (s *OpenAIService) ChatWithVisionAndData(ctx context.Context, systemPrompt string, base64Images []string, userPrompt string) (map[string]interface{}, error) {
	url := "https://api.openai.com/v1/chat/completions"

	// Prepare image messages
	var imageMessages []map[string]interface{}
	for _, b64 := range base64Images {
		imageMessages = append(imageMessages, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": b64,
			},
		})
	}

	if len(imageMessages) == 0 {
		return nil, fmt.Errorf("no images provided")
	}

	// Construct user content with both text prompt + images
	userContent := []interface{}{
		map[string]interface{}{
			"type": "text",
			"text": userPrompt,
		},
	}
	for _, img := range imageMessages {
		userContent = append(userContent, img)
	}

	// Log prompts and image count
	log.Println("[OpenAI] System Prompt:", systemPrompt)
	log.Println("[OpenAI] User Prompt:", userPrompt)
	log.Printf("[OpenAI] Sending %d image(s)\n", len(base64Images))

	// Build messages payload
	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": systemPrompt,
		},
		{
			"role":    "user",
			"content": userContent,
		},
	}

	payload := map[string]interface{}{
		"model":    "gpt-4o",
		"messages": messages,
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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Println("OpenAI API error response:", string(body))
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Println("[OpenAI] Raw Response Body:", string(body))

	// Parse response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned in response")
	}

	// Extract and clean content
	cleaned := strings.TrimSpace(result.Choices[0].Message.Content)
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	log.Println("[OpenAI] Cleaned Response Content:", cleaned)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		log.Println("[OpenAI] JSON Unmarshal Error:", err)
		return nil, fmt.Errorf("error parsing JSON: %w", err)
	}

	log.Println("[OpenAI] Parsed Response JSON:", parsed)

	return parsed, nil
}
