package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/PuerkitoBio/goquery"
	// "github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

type ScrapService struct {
	FirestoreClient   *firestore.Client
	OpenAIService     *OpenAIService
	RestaurantService *RestaurantService
}

// NewScrapService initializes ScrapService with Firestore and OpenAI service
func NewScrapService(openAIService *OpenAIService) *ScrapService {
	return &ScrapService{
		FirestoreClient:   database.GetFirestoreClient(),
		OpenAIService:     openAIService,
		RestaurantService: NewRestaurantService(),
	}
}

func (s *ScrapService) ScrapePlaces(searchURLs []string, placeChan chan<- models.Place, doneChan chan<- bool, latitude, longitude string) {
	var wg sync.WaitGroup

	for _, pageURL := range searchURLs {
		wg.Add(1)
		go func(pageURL string) {
			defer wg.Done()

			log.Printf("Scraping started for URL: %s\n", pageURL)
			places := scrapeAllData(pageURL, latitude, longitude)

			var menuWg sync.WaitGroup
			sem := make(chan struct{}, 5) // Limit to 5 concurrent menu scraping goroutines
			maxPlaces := 20
			if len(places) < maxPlaces {
				maxPlaces = len(places)
			}

			// var placesToSave []*models.Place // Slice to collect places for batch save

			var placesToSaveHalal []*models.Place // Slice to collect halal places for batch save
			var placesToSaveHaram []*models.Place // Slice

			for i := 0; i < maxPlaces; i++ {
				exists, _, err := s.RestaurantService.CheckRestaurantExists(context.Background(), places[i].Location.Latitude, places[i].Location.Longitude, places[i].Title)
				if err != nil {
					log.Printf("❌ Error checking restaurant existence for %s: %v\n", places[i].Title, err)
					continue
				}
				if exists {
					log.Printf("⚠️ Skipping duplicate restaurant: %s\n", places[i].Title)
					continue
				}

				menuWg.Add(1)
				sem <- struct{}{} // Acquire a slot

				go func(i int) {
					defer menuWg.Done()
					defer func() { <-sem }() // Release slot after completion

					menuChan := make(chan []string)
					reviewChan := make(chan []string)
					errChan := make(chan error, 2)

					go func() {
						menu, address, err := s.scrapeDataMenu(places[i].MapsLink)
						if err != nil {
							errChan <- err
						}
						menuChan <- menu
						places[i].Address = address
					}()
					go func() {
						reviews, err := scrapeDataReview(places[i].MapsLink, places[i].Title)
						if err != nil {
							errChan <- err
						}
						reviewChan <- reviews
					}()

					// Collect results
					menuLink := <-menuChan
					reviewUser := <-reviewChan

					if menuLink != nil && reviewUser != nil {
						places[i].MenuLink = menuLink
						places[i].Reviews = reviewUser
						menuList, err := s.OpenAIService.AnalyzeImages(context.Background(), menuLink)
						if err != nil {
							errChan <- err
						}
						places[i].Menu = menuList.Menu

						placeChan <- places[i]

						// Collect place in the slice for bulk save
						if menuList.HalalStatus == "halal" {
							placesToSaveHalal = append(placesToSaveHalal, &places[i])
						} else {
							placesToSaveHaram = append(placesToSaveHaram, &places[i])
						}
					}

					close(errChan)
					for err := range errChan {
						log.Printf("❌ Error processing %s: %v\n", places[i].Title, err)
					}
				}(i)
			}

			menuWg.Wait() // Wait for all scraping goroutines to complete

			if len(placesToSaveHalal) > 0 {
				err := s.RestaurantService.SaveRestaurants(context.Background(), placesToSaveHalal)
				if err != nil {
					log.Printf("❌ Bulk save (Halal) failed: %v\n", err)
				} else {
					log.Printf("✅ Successfully saved %d halal restaurants\n", len(placesToSaveHalal))
				}
			}

			// Perform bulk save for haram restaurants
			if len(placesToSaveHaram) > 0 {
				err := s.RestaurantService.SaveRestaurantsHaram(context.Background(), placesToSaveHaram)
				if err != nil {
					log.Printf("❌ Bulk save (Haram) failed: %v\n", err)
				} else {
					log.Printf("✅ Successfully saved %d haram restaurants\n", len(placesToSaveHaram))
				}
			}
		}(pageURL)
	}

	wg.Wait()
	close(placeChan)
	doneChan <- true
}

func scrapeAllData(pageURL string, latitude, longitude string) []models.Place {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-geolocation", false),
		chromedp.Flag("use-mock-keychain", true),
		// Optional: Use proxy for IP-based location
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	searchQuery := extractSearchQuery(pageURL)
	if searchQuery == "" {
		log.Println("Failed to extract search query from URL")
		return nil
	}

	var pageHTML string
	log.Printf("Navigating to page: %s\n", pageURL)
	err := chromedp.Run(ctx,
		// Grant geolocation permission
		chromedp.ActionFunc(func(ctx context.Context) error {
			err := chromedp.Evaluate(`navigator.permissions.query({name: "geolocation"}).then(p => p.state)`, nil).Do(ctx)
			if err != nil {
				log.Println("Geolocation permission issue:", err)
			}
			return nil
		}),
		// Set dynamic geolocation
		chromedp.ActionFunc(func(ctx context.Context) error {
			lat, err1 := strconv.ParseFloat(latitude, 64)
			long, err2 := strconv.ParseFloat(longitude, 64)
			if err1 != nil || err2 != nil {
				return fmt.Errorf("invalid latitude or longitude: %v, %v", err1, err2)
			}
			return emulation.SetGeolocationOverride().
				WithLatitude(lat).
				WithLongitude(long).
				WithAccuracy(1).
				Do(ctx)
		}),
		chromedp.Navigate(pageURL),
		chromedp.Sleep(2*time.Second),
		chromedp.WaitVisible(`[aria-label="Hasil untuk `+searchQuery+`"]`, chromedp.ByQuery),
		scrollMultipleTimes(5, searchQuery),
		chromedp.OuterHTML("body", &pageHTML),
	)
	if err != nil {
		log.Printf("Failed to load page %s: %v\n", pageURL, err)
		return nil
	}

	log.Println("Extracting data from page...")
	return extractData(pageHTML)
}

func (s *ScrapService) scrapeDataMenu(pageURL string) ([]string, string, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 60*time.Second) // Set timeout
	defer cancel()

	// var imageElements []map[string]string

	var imageMenuList []string
	var addressRestaurant string

	log.Printf("🚀 Navigating to: %s\n", pageURL)

	err := chromedp.Run(ctx,
		chromedp.Navigate(pageURL),
		chromedp.Sleep(5*time.Second), // Wait for page to load

		// Extract address if available
		chromedp.ActionFunc(func(ctx context.Context) error {
			var address string
			err := chromedp.AttributeValue(`button[data-tooltip="Salin alamat"]`, "aria-label", &address, nil).Do(ctx)
			if err != nil {
				return err
			}
			if address != "" {
				log.Printf("📍 Address found: %s\n", address)
			}
			addressRestaurant = address
			return nil
		}),

		// Check if menu button exists before clicking
		chromedp.ActionFunc(func(ctx context.Context) error {
			var exists bool
			if err := chromedp.Evaluate(`document.querySelector('div.ofKBgf button.K4UgGe[aria-label="Menu"]') !== null`, &exists).Do(ctx); err != nil {
				return err
			}
			if !exists {
				log.Println("⚠️ Menu button not found, skipping menu extraction.")
				return nil // Continue execution without clicking
			}
			return chromedp.Click(`div.ofKBgf button.K4UgGe[aria-label="Menu"]`, chromedp.ByQuery).Do(ctx)
		}),

		// Shorter timeout for menu items loading
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second) // Only wait 5 seconds
			defer cancel()
			return chromedp.WaitVisible("div.m6QErb.DxyBCb.kA9KIf.dS8AEf.XiKgde div.m6QErb.XiKgde", chromedp.ByQuery).Do(ctx)
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < 5; i++ {
				err := chromedp.Evaluate(`
			(function() {
				const el = document.querySelector('div.m6QErb.DxyBCb.kA9KIf.dS8AEf.XiKgde');
				if (el) el.scrollTop = el.scrollHeight;
			})()
		`, nil).Do(ctx)

				if err != nil {
					return err
				}
				time.Sleep(2 * time.Second)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			var imageURLs []string
			err := chromedp.Evaluate(`Array.from(document.querySelectorAll('div.Uf0tqf.loaded'))
				.map(el => el.style.backgroundImage.replace(/url\(["']?(.*?)["']?\)/, '$1'))`, &imageURLs).Do(ctx)

			if err != nil {
				log.Println("⚠️ Failed to extract menu images:", err)
				return nil
			}

			log.Printf("📸 Found %d menu images.\n", len(imageURLs))
			imageMenuList = imageURLs
			return nil
		}),
	)

	if err != nil {
		log.Printf("❌ Failed to scrape menu from %s: %v\n", pageURL, err)
		return nil, "", err
	}

	return imageMenuList, addressRestaurant, nil
}

func scrapeDataReview(pageURL, nameRestaurant string) ([]string, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 60*time.Second) // Set timeout
	defer cancel()

	log.Printf("🚀 Navigating to: %s\n", pageURL)

	reviewButtonSelector := fmt.Sprintf(`div.RWPxGd button.hh2c6[aria-label="Ulasan untuk %s"]`, nameRestaurant)
	var reviewTexts []string
	err := chromedp.Run(ctx,
		chromedp.Navigate(pageURL),
		chromedp.Sleep(10*time.Second), // Wait for page to load

		// Check if review button exists before clicking
		chromedp.ActionFunc(func(ctx context.Context) error {
			var exists bool
			if err := chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s') !== null`, reviewButtonSelector), &exists).Do(ctx); err != nil {
				return err
			}
			if !exists {
				log.Println("⚠️ Review button not found, skipping review extraction.")
				return nil
			}
			return chromedp.Click(reviewButtonSelector, chromedp.ByQuery).Do(ctx)
		}),

		// Scroll & Wait for reviews to load
		// scrollReviews(3),

		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < 5; i++ {
				err := chromedp.Evaluate(`
			(function() {
				const el = document.querySelector('div.m6QErb.DxyBCb.kA9KIf.dS8AEf.XiKgde');
				if (el) el.scrollTop = el.scrollHeight;
			})()
		`, nil).Do(ctx)

				if err != nil {
					return err
				}
				time.Sleep(2 * time.Second)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			var exists bool

			// Check if at least one review element is present
			if err := chromedp.Evaluate(`document.querySelector('div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.GHT2ce div.MyEned span.wiI7pd') !== null`, &exists).Do(ctx); err != nil {
				return err
			}
			if !exists {
				log.Println("⚠️ No reviews found.")
				return nil
			}

			// Extract inner text of all matched review elements
			return chromedp.Evaluate(`
		Array.from(document.querySelectorAll('div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.GHT2ce div.MyEned span.wiI7pd'))
			.map(el => el.innerText)
	`, &reviewTexts).Do(ctx)
		}),
	)

	if err != nil {
		log.Printf("❌ Failed to scrape reviews from %s: %v\n", pageURL, err)
		return nil, err
	}

	log.Printf("Reviews: %s", strings.Join(reviewTexts, ", "))

	return reviewTexts, nil
}

func extractData(html string) []models.Place {
	var places []models.Place
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Println("Error parsing HTML:", err)
		return nil
	}

	doc.Find(".Nv2PK").Each(func(i int, s *goquery.Selection) {
		mapsLink := s.Find("a[href]").AttrOr("href", "N/A")
		lat, long, _ := extractLatLong(mapsLink) // Call extractLatLong only once
		reviewCount := s.Find(".UY7F9").Text()
		reviewCountClean := cleanReviewCount(reviewCount)

		rawImageURL := s.Find("img").AttrOr("src", "N/A")
		enhancedImageURL := replaceImageProfileQuality(rawImageURL)

		place := models.Place{
			Title:       s.Find(".qBF1Pd").Text(),
			Rating:      s.Find(".MW4etd").Text(),
			ReviewCount: reviewCountClean,
			Location: models.GeoLocation{

				Latitude:  lat,
				Longitude: long,
			},
			OpeningStatus: s.Find(".W4Efsd span[style*='color']").Text(),
			ImageURL:      enhancedImageURL,
			MapsLink:      s.Find("a[href]").AttrOr("href", "N/A"),
		}
		places = append(places, place)
	})

	return places
}

func extractSearchQuery(pageURL string) string {
	parsedURL, err := url.Parse(pageURL)
	if err != nil {
		log.Println("Error parsing URL:", err)
		return ""
	}

	// Extract the query part (e.g., "makanan+terdekat" or "restaurant+nearby")
	pathSegments := strings.Split(parsedURL.Path, "/search/")
	if len(pathSegments) < 2 {
		return ""
	}

	// Get the actual search query and replace "+" with spaces
	queryPart := strings.Split(pathSegments[1], "/")[0] // Get first segment after "search/"
	return strings.ReplaceAll(queryPart, "+", " ")      // Convert to readable format
}

// func extractReviewTexts(nodes []*cdp.Node) []string {
// 	var reviews []string
// 	for _, node := range nodes {
// 		if text := node.NodeValue; text != "" {
// 			reviews = append(reviews, text)
// 		} else if len(node.Children) > 0 {
// 			reviews = append(reviews, node.Children[0].NodeValue)
// 		}
// 	}
// 	return reviews
// }

func scrollMultipleTimes(times int, searchQuery string) chromedp.Tasks {
	var tasks chromedp.Tasks
	for i := 0; i < times; i++ {
		tasks = append(tasks,
			chromedp.Evaluate(`document.querySelector('[aria-label="Hasil untuk `+searchQuery+`"]').scrollBy(0, 500);`, nil),
			chromedp.Sleep(500*time.Millisecond),
		)
		log.Printf("Scrolling down (%d/%d)...\n", i+1, times)
	}
	return tasks
}

func extractLatLong(url string) (float64, float64, error) {
	re := regexp.MustCompile(`3d(-?\d+\.\d+)!4d(-?\d+\.\d+)`)
	matches := re.FindStringSubmatch(url)

	if len(matches) < 3 {
		return 0, 0, fmt.Errorf("latitude and longitude not found")
	}

	latitude, err1 := strconv.ParseFloat(matches[1], 64)
	longitude, err2 := strconv.ParseFloat(matches[2], 64)

	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("error converting latitude/longitude to float")
	}

	return latitude, longitude, nil
}

func cleanReviewCount(reviewText string) string {
	re := regexp.MustCompile(`\d+\.\d+`) // Match a number like "1.297"
	match := re.FindString(reviewText)
	return match
}

// Function to replace any resolution with "s1600-k-no"
func replaceImageProfileQuality(imageURL string) string {
	// Regex to match the pattern for resolution like w80-h106 or any size (s<number>-k-no)
	re := regexp.MustCompile(`w\d+-h\d+-k-no`)

	// Replace it with "s1600-k-no" to get higher quality
	return re.ReplaceAllString(imageURL, "s1600-k-no")
}

//

type RestaurantStatusResponse struct {
	Status string `json:"status"`
	Title  string `json:"title"`
}

func (s *ScrapService) ScrapeSinglePlace(mapsLink string) (*RestaurantStatusResponse, error) {
	log.Printf("🔍 Scraping single place: %s\n", mapsLink)

	// Step 1: Scrape basic data from HTML
	place := scrapeSinglePlaceHTML(mapsLink)
	if place == nil {
		return nil, fmt.Errorf("failed to scrape base place data from: %s", mapsLink)
	}

	// Step 2: Check if restaurant already exists
	exists, status, err := s.RestaurantService.CheckRestaurantExists(
		context.Background(),
		place.Location.Latitude,
		place.Location.Longitude,
		place.Title,
	)
	if err != nil {
		return nil, fmt.Errorf("error checking existence: %w", err)
	}
	if exists {
		// Restaurant already exists, return its status
		log.Printf("⚠️ Restaurant already exists: %s. Status: %s\n", place.Title, status)
		return &RestaurantStatusResponse{
			Status: status,
			Title:  place.Title,
		}, nil // Return the existing status and title
	}

	// Step 3: Scrape additional async data (menu + reviews)
	menuChan := make(chan []string)
	reviewChan := make(chan []string)
	errChan := make(chan error, 2)

	// Menu scraping
	go func() {
		menu, address, err := s.scrapeDataMenu(mapsLink)
		if err != nil {
			errChan <- err
			menuChan <- nil
			return
		}
		place.Address = address
		menuChan <- menu
	}()

	// Review scraping
	go func() {
		reviews, err := scrapeDataReview(mapsLink, place.Title)
		if err != nil {
			errChan <- err
			reviewChan <- nil
			return
		}
		reviewChan <- reviews
	}()

	menuLink := <-menuChan
	reviewUser := <-reviewChan

	// Step 4: Attach data if available
	if menuLink != nil && reviewUser != nil {
		place.MenuLink = menuLink
		place.Reviews = reviewUser

		menuList, err := s.OpenAIService.AnalyzeImages(context.Background(), menuLink)
		if err != nil {
			log.Printf("❌ Error analyzing images: %v\n", err)
		} else {
			place.Menu = menuList.Menu
			if menuList.HalalStatus == "halal" {
				// Step 5: Save to DB
				err = s.RestaurantService.SaveRestaurants(context.Background(), []*models.Place{place})
				if err != nil {
					return nil, fmt.Errorf("failed to save restaurant: %w", err)
				}

				// Return "halal" status with the restaurant title
				log.Printf("✅ Halal Restaurant saved: %s\n", place.Title)
				return &RestaurantStatusResponse{
					Status: "halal",
					Title:  place.Title,
				}, nil
			} else {

				err := s.RestaurantService.SaveRestaurantsHaram(context.Background(), []*models.Place{place})
				if err != nil {
					log.Printf("❌ Bulk save (Haram) failed: %v\n", err)
				}
				// Return "haram" status with the restaurant title
				log.Printf("✅ Haram Restaurant saved: %s\n", place.Title)
				return &RestaurantStatusResponse{
					Status: "haram",
					Title:  place.Title,
				}, nil
			}
		}
	}

	// Default return if no valid data
	return nil, fmt.Errorf("failed to scrape or analyze data for: %s", mapsLink)
}

func scrapeSinglePlaceHTML(pageURL string) *models.Place {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-geolocation", false),
		chromedp.Flag("use-mock-keychain", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var pageHTML string
	var currentURL string // Untuk menyimpan URL setelah redirect atau perubahan otomatis

	log.Printf("Navigating to place page: %s\n", pageURL)

	err := chromedp.Run(ctx,
		chromedp.Navigate(pageURL),
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("body", &pageHTML),
		chromedp.Location(&currentURL), // Ambil URL sebenarnya dari browser
	)
	if err != nil {
		log.Printf("Failed to load single place page: %v\n", err)
		return nil
	}

	log.Printf("Current loaded URL: %s\n", currentURL)
	log.Println("Extracting single place data...")

	return extractSinglePlaceData(pageHTML, currentURL) // Kirim URL hasil navigasi
}

func extractSinglePlaceData(html string, mapsLink string) *models.Place {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Println("Error parsing HTML:", err)
		return nil
	}

	// Try finding accurate selectors for a single place view
	title := doc.Find("h1.DUwDvf").Text()
	address := doc.Find("button[data-item-id='address']").Text()
	reviewCount := doc.Find(".UY7F9").Text()
	cleanedReviewCount := cleanReviewCount(reviewCount)
	imageURL := doc.Find("img[src]").First().AttrOr("src", "N/A")
	enhancedImageURL := replaceImageProfileQuality(imageURL)

	lat, long, _ := extractLatLong(mapsLink)

	return &models.Place{
		Title:       title,
		Address:     address,
		Rating:      doc.Find(".MW4etd").Text(),
		ReviewCount: cleanedReviewCount,
		ImageURL:    enhancedImageURL,
		MapsLink:    mapsLink,
		Location: models.GeoLocation{
			Latitude:  lat,
			Longitude: long,
		},
	}
}
