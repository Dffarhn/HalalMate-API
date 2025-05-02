package controllers

import (
	"HalalMate/services"
	"HalalMate/utils"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SnackController struct {
	SnackService  *services.SnackService
	OpenAIService *services.OpenAIService
}

type ScanRequest struct {
	Location string `json:"location"`
}

func NewSnackController() *SnackController {
	return &SnackController{
		SnackService:  services.NewSnackService(),
		OpenAIService: services.NewOpenAIService(),
	}
}

func (sc *SnackController) ScanSnackByBarcode(c *gin.Context) {
	barcode := c.Param("barcode")
	if barcode == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Barcode is required")
		return
	}

	fmt.Println("Barcode:", barcode)

	var req ScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	location := req.Location
	fmt.Println("Location:", location)

	product, err := sc.SnackService.GetProductByBarcode(barcode)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch snack information")
		return
	}

	var hasil map[string]interface{}

	if product == nil {
		hasil = map[string]interface{}{
			"Status":      "Tidak Dapat Menentukan",
			"Reason":      "Produk tidak ditemukan di database OpenFoodFacts",
			"ProductName": "",
			"Suggest":     []interface{}{},
		}
	} else {
		fmt.Println("Product:", product)

		systemPrompt := fmt.Sprintf(`Kamu adalah pakar analisis kehalalan makanan.
	
		Tugasmu adalah mengecek apakah produk makanan halal atau haram, berdasarkan bahan-bahan (ingredients), tag bahan, dan informasi lain yang diberikan dalam format JSON. Fokus pada bahan seperti daging babi (pork), alkohol, gelatin non-halal, enzim hewani, dan bahan turunan hewani lain yang mencurigakan.
		
		**Balasan kamu harus selalu dalam format JSON seperti ini:**
		
		{
		  "Status": "Halal" atau "Haram",
		  "Reason": "Alasan singkat mengapa produk ini dianggap halal atau haram",
		  "ProductName": "Nama produk" nama produk yang ada didalam data,
		  "Suggest": [
			{
			  "NamaSugestProduk": "..."
			},
			...
		  ]
		}
		
		- Jika produk **halal**, tulis '"Status": "Halal"' dan kosongkan array 'Suggest' → '"Suggest": []'.
		- Jika produk **haram**, tulis '"Status": "Haram"' dan berikan **1-3 produk snack halal yang nyata dan mirip**, baik dari jenis, rasa, atau bentuk.
		- Produk rekomendasi harus **benar-benar ada di dunia nyata**, **berlabel halal**, dan **mudah ditemukan di wilayah %s**.
		- Jika tidak yakin, beri '"Status": "Tidak Dapat Menentukan"' dan kosongkan 'Suggest'.
		
		JANGAN gunakan format markdown seperti "json" atau tanda lainnya.
		Kembalikan hanya JSON murni tanpa tanda apapun di sekelilingnya.`, location)
	
		productString := fmt.Sprintf("%v", product) // Convert product to string
		hasil, err = sc.OpenAIService.Chat(c, systemPrompt, productString)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to process snack information")
			return
	
		}
	}

	utils.SuccessResponse(c, http.StatusOK, "Scan snack berhasil", hasil)
}

func (sc *SnackController) ScanSnackByImage(c *gin.Context) {

	// var req ScanRequest
	location := c.PostForm("location")
	if location == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Location is required")
		return
	}

	fmt.Println("Location:", location)

	frontFile, err := c.FormFile("frontImage")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Front image is required")
		return
	}

	backFile, err := c.FormFile("backImage")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Back image is required")
		return
	}

	frontImg, err := frontFile.Open()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to open front image")
		return
	}
	defer frontImg.Close()

	backImg, err := backFile.Open()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to open back image")
		return
	}
	defer backImg.Close()

	frontBase64, err := EncodeImageToBase64(frontImg)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to encode front image")
		return
	}

	backBase64, err := EncodeImageToBase64(backImg)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to encode back image")
		return
	}

	systemPrompt := fmt.Sprintf(`Kamu adalah pakar analisis kehalalan makanan berbasis citra (gambar).

	Tugasmu adalah:
	1. Menganalisis gambar produk makanan yang diberikan untuk mengidentifikasi nama produk secara akurat.
	2. Mencari informasi bahan-bahan (ingredients), label, dan detail lain yang relevan berdasarkan identifikasi produk tersebut.
	3. Mengecek apakah produk tersebut halal atau haram, dengan fokus pada bahan seperti daging babi (pork), alkohol, gelatin non-halal, enzim hewani, dan bahan turunan hewani lain yang mencurigakan.
	
	**Balasan kamu harus selalu dalam format JSON seperti ini:**
	
	{
	  "Status": "Halal" atau "Haram" atau "Tidak Dapat Menentukan",
	  "Reason": "Alasan singkat mengapa produk ini dianggap halal atau haram",
	  "ProductName": "Nama produk yang terdeteksi dari gambar",
	  "Suggest": [
		{
		  "NamaSugestProduk": "..."
		},
		...
	  ]
	}
	
	- Jika produk **halal**, tulis '"Status": "Halal"' dan kosongkan array 'Suggest' → '"Suggest": []'.
	- Jika produk **haram**, tulis '"Status": "Haram"' dan berikan **1-3 produk snack halal yang nyata dan mirip**, baik dari jenis, rasa, atau bentuk.
	- Produk rekomendasi harus **berlabel halal**, **benar-benar ada di dunia nyata**, dan **mudah ditemukan di wilayah %s**.
	- Jika tidak yakin (misalnya gambar kurang jelas atau tidak ada cukup data), beri '"Status": "Tidak Dapat Menentukan"' dan kosongkan 'Suggest'.
	
	JANGAN gunakan format markdown seperti "json" atau tanda lainnya.
	Kembalikan hanya JSON murni tanpa tanda apapun di sekelilingnya.
	`, location)
	

	// Call ke OpenAI
	result, err := sc.OpenAIService.ChatWithVision(c, systemPrompt, []string{frontBase64, backBase64})
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to process images with AI")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Scan hasil dari gambar berhasil", result)
}

func EncodeImageToBase64(file multipart.File) (string, error) {
	buf := new(bytes.Buffer)
	_, err := io.Copy(buf, file)
	if err != nil {
		return "", err
	}
	mimeType := http.DetectContentType(buf.Bytes())
	base64Str := base64.StdEncoding.EncodeToString(buf.Bytes())
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64Str), nil
}
