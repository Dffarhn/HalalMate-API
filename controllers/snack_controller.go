package controllers

import (
	"HalalMate/services"
	"HalalMate/utils"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
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

		Tugasmu adalah mengecek apakah produk makanan halal atau haram, hanya berdasarkan bahan-bahan (ingredients), tag bahan, dan informasi eksplisit lain yang diberikan dalam format JSON. Jangan membuat asumsi atau spekulasi tentang proses produksi yang tidak disebutkan.
		
		Fokus pada bahan yang jelas haram seperti:
		- Daging babi (pork, bacon, ham, lard, dsb)
		- Alkohol (ethanol, wine, beer, dsb)
		- Gelatin yang tidak dijelaskan kehalalannya
		- Enzim hewani yang tidak dijelaskan sumbernya
		- Bahan turunan hewani lain yang mencurigakan jika sumbernya tidak dijelaskan
		
		**Balasan kamu harus selalu dalam format JSON seperti ini:**
		
		{
		  "Status": "Halal" atau "Haram" atau "Tidak Dapat Menentukan",
		  "Reason": "Alasan singkat mengapa produk ini dianggap halal, haram, atau tidak dapat ditentukan",
		  "ProductName": "Nama produk",
		  "Suggest": [
			{
			  "NamaSugestProduk": "..."
			}
		  ]
		}
		
		- Jika produk **halal**, tulis "Status": "Halal" dan kosongkan array Suggest → "Suggest": []
		- Jika produk **haram**, tulis "Status": "Haram" dan beri 1-3 produk snack halal nyata dan mirip, yang mudah ditemukan di wilayah %s
		- Jika bahan tidak cukup untuk menentukan status, tulis "Status": "Tidak Dapat Menentukan" dan kosongkan Suggest → "Suggest": []
		
		Jangan gunakan format markdown seperti "json" atau tanda lainnya.
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
	1. Menganalisis gambar produk makanan untuk mengidentifikasi nama produk secara akurat.
	2. Berdasarkan nama produk, cari informasi bahan-bahan (ingredients), jenis produk, serta detail lain yang relevan dari sumber tepercaya.
	3. **Penilaian halal/haram dilakukan BERDASARKAN komposisi bahan**, bukan hanya berdasarkan ada/tidaknya label halal pada kemasan.
	4. Fokuslah pada bahan-bahan seperti daging babi (pork), alkohol, gelatin non-halal, enzim hewani, dan bahan turunan hewani mencurigakan lainnya.
	5. Label halal hanya boleh digunakan sebagai pendukung jika informasi bahan tidak lengkap atau ambigu.
	6. Jika produk mengandung bahan haram atau mencurigakan, nyatakan sebagai "Haram". Jika tidak ditemukan bahan haram atau mencurigakan, nyatakan sebagai "Halal".
	
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
	- Produk rekomendasi harus **berlabel halal** (dari lembaga seperti MUI, JAKIM, HFA, dll), **benar-benar ada di dunia nyata**, dan **mudah ditemukan di wilayah %s secara lokal atau melalui platform e-commerce umum**.
	- Jika tidak yakin (misalnya gambar kurang jelas atau tidak ada cukup data bahan), beri '"Status": "Tidak Dapat Menentukan"' dan kosongkan 'Suggest'.
	
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

func (sc *SnackController) SearchSnackByInput(c *gin.Context) {
	var req struct {
		NameProduct string `json:"name_product"`
		Location    string `json:"location"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Println("[ERROR] Invalid request format:", err)
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format")
		return
	}

	log.Printf("[INFO] User login attempt: Email - %s", req.NameProduct)

	systemPrompt := fmt.Sprintf(`Kamu adalah pakar makanan halal.

Langkah-langkahmu adalah:
1. Identifikasi produk dari Nama Produk yang diberikan yaitu "%s".
2. Jika nama produk tidak jelas atau terlalu umum (contoh: "Permen Manis", "Mie Instan"), cari kemungkinan nama produk nyata atau merek yang relevan menggunakan informasi dari situs seperti Wikipedia, OpenFoodFacts, atau situs brand resmi.
3. Gunakan nama produk yang paling cocok untuk mencari informasi bahan-bahan produk tersebut.
4. Berdasarkan informasi bahan tersebut, tentukan status halal produk.
5. Jika kamu tidak menemukan informasi bahan, kamu bisa memberikan konfirmasi tentang nama produk tersebut dengan beberapa alternatif sugesti nama produk nyata.
6. Jika tidak cukup data, nyatakan bahwa kamu tidak bisa menentukan.

**Selalu balas dalam format JSON murni:**
{
  "Status": "Halal" atau "Haram" atau "Tidak Dapat Menentukan",
  "Reason": "Alasan singkat",
  "ProductName": "Nama produk dari user",
  "Suggest": [
    {
      "NamaSugestProduk": "Alternatif nama produk nyata"
    }
  ],
}

- Jika Halal → "Suggest": []
- Jika Haram → beri 1-3 alternatif halal nyata dan tersedia di %s
- Jika tidak cukup data → "Status": "Tidak Dapat Menentukan", dan berikan beberapa suggest nama produk yang memungkinkan

Jangan gunakan markdown atau format tambahan lain.`, req.NameProduct, req.Location)

	userPrompt := fmt.Sprintf(`Saya ingin mengecek status halal dari produk dengan nama: "%s".

Jika produk tersebut berasal dari merek ternama dan tidak mengandung bahan mencurigakan seperti gelatin babi atau alkohol, kamu boleh mengasumsikan produk tersebut HALAL. Tapi tetap sebutkan bahan yang membuatmu yakin.

Jika kamu tidak menemukan bahan-bahannya, berikan alternatif nama produk nyata yang mirip.`, req.NameProduct)

	result, err := sc.OpenAIService.Chat(c, systemPrompt, userPrompt)
	if err != nil {
		log.Println("[ERROR] Failed to process snack search:", err)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to process snack search")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Search snack berhasil", result)
	log.Printf("[INFO] Search snack completed successfully for: %s", req.NameProduct)

}

func (sc *SnackController) ScanWithFrontOnly(c *gin.Context) {
	location := c.PostForm("location")
	if location == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Location is required")
		return
	}

	frontFile, err := c.FormFile("frontImage")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Front image is required")
		return
	}

	frontImg, err := frontFile.Open()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to open front image")
		return
	}
	defer frontImg.Close()

	frontBase64, err := EncodeImageToBase64(frontImg)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to encode front image")
		return
	}

	systemPrompt := fmt.Sprintf(`Kamu adalah pakar makanan halal.

	Langkah-langkahmu adalah:
	1. Lihat dan identifikasi produk dari **gambar depan kemasan**. Fokus pada teks, logo, dan tampilan visual.
	2. Gunakan hasil identifikasi nama produk untuk **mencari informasi bahan-bahan produk tersebut** dari sumber seperti Wikipedia, OpenFoodFacts, atau situs brand resmi.
	3. Berdasarkan informasi bahan tersebut, tentukan status halal produk.
	4. Jika tidak cukup data, nyatakan bahwa kamu tidak bisa menentukan.

	**Selalu balas dalam format JSON murni:**
	{
	"Status": "Halal" atau "Haram" atau "Tidak Dapat Menentukan",
	"Reason": "Alasan singkat",
	"ProductName": "Nama produk",
	"Suggest": [
		{
		"NamaSugestProduk": "..."
		}
	]
	}

	- Jika halal → "Suggest": []
	- Jika haram → beri 1-3 alternatif halal nyata dan tersedia di %s
	- Jika tidak cukup data → "Status": "Tidak Dapat Menentukan", "Suggest": []

	Jangan gunakan markdown atau format tambahan lain.`, location)

	result, err := sc.OpenAIService.ChatWithVision(c, systemPrompt, []string{frontBase64})
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to process front image")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Scan berhasil dari gambar depan saja", result)
}

func (sc *SnackController) ScanWithFrontAndBack(c *gin.Context) {
	location := c.PostForm("location")
	if location == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Location is required")
		return
	}

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

	frontImg, _ := frontFile.Open()
	backImg, _ := backFile.Open()
	defer frontImg.Close()
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

Tugasmu:
1. Identifikasi nama produk dari **gambar depan kemasan**.
2. Ambil dan baca daftar **bahan/komposisi** dari **gambar belakang kemasan**.
3. Analisis kehalalan berdasarkan bahan-bahan tersebut. Fokus pada:
   - Daging babi dan turunannya (lard, bacon, pork, dsb)
   - Alkohol atau bahan fermentasi yang mengandung alkohol
   - Gelatin, enzim, atau bahan hewani yang tidak jelas asalnya
   - Bahan kontroversial (E-codes, emulsifier, dll)
4. Label halal di kemasan hanya sebagai pendukung, bukan bukti utama.
5. Jika tidak cukup informasi dari gambar, beri jawaban "Tidak Dapat Menentukan".

**Wajib balas dalam format JSON murni, tanpa markdown atau tambahan lain:**

{
  "Status": "Halal" | "Haram" | "Tidak Dapat Menentukan",
  "Reason": "Alasan singkat dan jelas",
  "ProductName": "Nama produk (hasil identifikasi dari gambar depan)",
  "Suggest": [
    {
      "NamaSugestProduk": "Alternatif halal (jika produk haram)"
    }
  ]
}

- Jika status "Halal" → "Suggest": []
- Jika "Haram" → Beri 1–3 produk alternatif halal yang nyata dan tersedia di %s
- Jika "Tidak Dapat Menentukan" → Suggest juga harus kosong

Balas hanya dengan JSON valid. Jangan beri narasi tambahan.
`, location)

	result, err := sc.OpenAIService.ChatWithVision(c, systemPrompt, []string{frontBase64, backBase64})
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to process front and back images")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Scan berhasil dari gambar depan dan belakang", result)
}

func (sc *SnackController) ScanWithImageAndBarcode(c *gin.Context) {
	location := c.PostForm("location")
	barcode := c.PostForm("barcode")
	if location == "" || barcode == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Location and barcode are required")
		return
	}

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

	frontImg, _ := frontFile.Open()
	backImg, _ := backFile.Open()
	defer frontImg.Close()
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

	product, err := sc.SnackService.GetProductByBarcode(barcode)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch product information by barcode")
		return
	}
	var productInfo string
	if product != nil {
		productInfo = fmt.Sprintf("Informasi dari barcode (%s): %v", barcode, product)
	} else {
		productInfo = fmt.Sprintf("Tidak ditemukan informasi tambahan dari barcode (%s).", barcode)
	}

	systemPrompt := fmt.Sprintf(`Kamu adalah AI pakar kehalalan makanan.

Sumber data yang tersedia:
- Gambar depan produk (berisi nama dan visual kemasan)
- Gambar belakang produk (berisi komposisi bahan)
- Informasi dari barcode (%s), jika tersedia

Tugasmu:
- Identifikasi nama produk dari gambar depan
- Ambil bahan-bahan dari gambar belakang
- Gunakan informasi barcode (jika ada) untuk membantu klarifikasi bahan

Fokus analisis kehalalan:
- Daging babi dan turunannya
- Alkohol atau hasil fermentasi
- Gelatin, enzim, atau bahan hewani tak jelas
- Bahan sintetis mencurigakan (seperti E-codes)
- Label halal hanya sebagai pendukung, bukan bukti utama

Balas HANYA dalam format JSON valid berikut:

{
  "Status": "Halal" | "Haram" | "Tidak Dapat Menentukan",
  "Reason": "Penjelasan ringkas dan jelas",
  "ProductName": "Nama produk",
  "Suggest": [
    {
      "NamaSugestProduk": "Alternatif halal (jika produk haram)"
    }
  ]
}

Aturan tambahan:
- Jika status = "Halal", maka "Suggest": []
- Jika status = "Haram", beri 1-3 alternatif halal nyata yang tersedia di %s
- Jika status = "Tidak Dapat Menentukan", maka "Suggest": []`, barcode, location)

	userPrompt := fmt.Sprintf(`Berikut data lengkap yang bisa kamu gunakan:

1. Gambar depan produk (berisi nama dan visual)
2. Gambar belakang produk (berisi daftar bahan)
3. %s

Gabungkan seluruh informasi di atas untuk menganalisis status kehalalan produk ini. Jangan tebak jika tidak cukup informasi.

Balas HANYA dalam format JSON valid berikut:

{
  "Status": "Halal" | "Haram" | "Tidak Dapat Menentukan",
  "Reason": "Penjelasan ringkas dan jelas",
  "ProductName": "Nama produk",
  "Suggest": [
    {
      "NamaSugestProduk": "Alternatif halal (jika produk haram)"
    }
  ]
}

Aturan tambahan:
- Jika status = "Halal", maka "Suggest": []
- Jika status = "Haram", beri 1-3 alternatif halal nyata yang tersedia di %s
- Jika status = "Tidak Dapat Menentukan", maka "Suggest": []


`, productInfo, location)

	result, err := sc.OpenAIService.ChatWithVisionAndData(c, systemPrompt, []string{frontBase64, backBase64}, userPrompt)

	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to process full data")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Scan berhasil dari gambar dan barcode", result)
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
