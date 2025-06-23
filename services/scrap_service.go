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
			sem := make(chan struct{}, 3) // Reduced from 5 to 3 concurrent menu scraping goroutines

			// Increase maxPlaces to handle duplicates better
			maxPlaces := 50 // Increased from 20 to 50
			if len(places) < maxPlaces {
				maxPlaces = len(places)
			}

			// var placesToSave []*models.Place // Slice to collect places for batch save

			var placesToSaveHalal []*models.Place // Slice to collect halal places for batch save
			var placesToSaveHaram []*models.Place // Slice

			// Track how many places we've processed and how many are new
			processedCount := 0
			newPlacesFound := 0
			targetNewPlaces := 4 // We want to find at least 20 new places

			for i := 0; i < maxPlaces && newPlacesFound < targetNewPlaces; i++ {
				place := places[i] // capture the value here
				processedCount++

				// Skip places with empty titles
				if place.Title == "" {
					log.Printf("⚠️ Skipping place with empty title (processed %d, found %d new)\n", processedCount, newPlacesFound)
					continue
				}

				exists, _, err := s.RestaurantService.CheckRestaurantExists(context.Background(), places[i].Location.Latitude, places[i].Location.Longitude, places[i].Title)
				if err != nil {
					log.Printf("❌ Error checking restaurant existence for %s: %v\n", places[i].Title, err)
					continue
				}
				if exists {
					log.Printf("⚠️ Skipping duplicate restaurant: %s (processed %d, found %d new)\n", places[i].Title, processedCount, newPlacesFound)
					continue
				}

				newPlacesFound++
				log.Printf("✅ Found new restaurant: %s (processed %d, found %d new)\n", places[i].Title, processedCount, newPlacesFound)

				menuWg.Add(1)
				sem <- struct{}{} // Acquire a slot
				go func(p models.Place) {
					defer menuWg.Done()
					defer func() { <-sem }()

					// Add timeout for the entire goroutine
					ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
					defer cancel()

					menuChan := make(chan []string, 1)
					reviewChan := make(chan []string, 1)
					errChan := make(chan error, 2)

					// Menu scraping with timeout
					go func() {
						menu, address, err := s.scrapeDataMenu(p.MapsLink)
						if err != nil {
							log.Printf("⚠️ Menu scraping failed for %s: %v\n", p.Title, err)
							errChan <- err
							menuChan <- nil
							return
						}
						select {
						case menuChan <- menu:
						case <-ctx.Done():
							log.Printf("⚠️ Menu scraping timeout for %s\n", p.Title)
						}
						p.Address = address
					}()

					// Review scraping with timeout
					go func() {
						reviews, err := scrapeDataReview(p.MapsLink, p.Title)
						if err != nil {
							log.Printf("⚠️ Review scraping failed for %s: %v\n", p.Title, err)
							errChan <- err
							reviewChan <- nil
							return
						}
						select {
						case reviewChan <- reviews:
						case <-ctx.Done():
							log.Printf("⚠️ Review scraping timeout for %s\n", p.Title)
						}
					}()

					// Wait for both operations with timeout
					var menuLink []string
					var reviewUser []string

					select {
					case menuLink = <-menuChan:
					case <-ctx.Done():
						log.Printf("⚠️ Menu channel timeout for %s\n", p.Title)
						menuLink = nil
					}

					select {
					case reviewUser = <-reviewChan:
					case <-ctx.Done():
						log.Printf("⚠️ Review channel timeout for %s\n", p.Title)
						reviewUser = nil
					}

					// Process results even if one operation failed
					if menuLink != nil || reviewUser != nil {
						if menuLink != nil {
							p.MenuLink = menuLink
						}
						if reviewUser != nil {
							p.Reviews = reviewUser
						}

						// Only analyze images if we have menu links
						if len(menuLink) > 0 {
							menuList, err := s.OpenAIService.AnalyzeImages(context.Background(), menuLink)
							if err != nil {
								log.Printf("❌ Error analyzing images for %s: %v\n", p.Title, err)
								// Continue with the place even if image analysis fails
								placeChan <- p
								// Mark as haram by default if analysis fails
								placesToSaveHaram = append(placesToSaveHaram, &p)
							} else if menuList != nil {
								// Only set menu if menuList is not nil
								p.Menu = menuList.Menu
								placeChan <- p

								if menuList.HalalStatus == "halal" {
									placesToSaveHalal = append(placesToSaveHalal, &p)
								} else {
									placesToSaveHaram = append(placesToSaveHaram, &p)
								}
							} else {
								// Handle case where menuList is nil but no error
								log.Printf("⚠️ menuList is nil for %s, marking as haram\n", p.Title)
								placeChan <- p
								placesToSaveHaram = append(placesToSaveHaram, &p)
							}
						} else {
							// No menu links available, save as haram
							log.Printf("⚠️ No menu links for %s, marking as haram\n", p.Title)
							placeChan <- p
							placesToSaveHaram = append(placesToSaveHaram, &p)
						}
					} else {
						log.Printf("⚠️ Both menu and review scraping failed for %s\n", p.Title)
					}

					close(errChan)
					for err := range errChan {
						log.Printf("❌ Error processing %s: %v\n", p.Title, err)
					}

				}(place)
			}

			log.Printf("📊 Scraping summary: Processed %d places, found %d new restaurants\n", processedCount, newPlacesFound)

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

	ctx, cancel = context.WithTimeout(ctx, 90*time.Second) // Increased from 60 to 90 seconds
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

	// Convert latitude and longitude strings to float64
	searchLat, err1 := strconv.ParseFloat(latitude, 64)
	searchLong, err2 := strconv.ParseFloat(longitude, 64)
	if err1 != nil || err2 != nil {
		log.Printf("Error converting coordinates: %v, %v", err1, err2)
		return nil
	}

	return extractData(pageHTML, searchLat, searchLong)
}

func (s *ScrapService) scrapeDataMenu(pageURL string) ([]string, string, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 90*time.Second) // Increased timeout to 90 seconds
	defer cancel()

	var imageMenuList []string
	var addressRestaurant string

	log.Printf("🚀 Navigating to: %s\n", pageURL)

	err := chromedp.Run(ctx,
		chromedp.Navigate(pageURL),
		chromedp.Sleep(3*time.Second), // Reduced from 5 to 3 seconds

		// Extract address if available (with shorter timeout)
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			var address string
			err := chromedp.AttributeValue(`button[data-tooltip="Salin alamat"]`, "aria-label", &address, nil).Do(ctx)
			if err != nil {
				log.Printf("⚠️ Could not extract address: %v\n", err)
				return nil // Don't fail the entire operation
			}
			if address != "" {
				log.Printf("📍 Address found: %s\n", address)
			}
			addressRestaurant = address
			return nil
		}),

		// Check if menu button exists before clicking (with shorter timeout)
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			var exists bool
			if err := chromedp.Evaluate(`document.querySelector('div.ofKBgf button.K4UgGe[aria-label="Menu"]') !== null`, &exists).Do(ctx); err != nil {
				log.Printf("⚠️ Could not check menu button: %v\n", err)
				return nil // Don't fail the entire operation
			}
			if !exists {
				log.Println("⚠️ Menu button not found, skipping menu extraction.")
				return nil // Continue execution without clicking
			}
			return chromedp.Click(`div.ofKBgf button.K4UgGe[aria-label="Menu"]`, chromedp.ByQuery).Do(ctx)
		}),

		// Wait for menu items loading (with shorter timeout and better error handling)
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 8*time.Second) // Reduced from 10 to 8 seconds
			defer cancel()

			err := chromedp.WaitVisible("div.m6QErb.DxyBCb.kA9KIf.dS8AEf.XiKgde div.m6QErb.XiKgde", chromedp.ByQuery).Do(ctx)
			if err != nil {
				log.Printf("⚠️ Menu items not visible after timeout: %v\n", err)
				return nil // Don't fail, try to extract what we can
			}
			return nil
		}),

		// Reduced scrolling iterations and sleep time
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < 3; i++ { // Reduced from 5 to 3 iterations
				err := chromedp.Evaluate(`
			(function() {
				const el = document.querySelector('div.m6QErb.DxyBCb.kA9KIf.dS8AEf.XiKgde');
				if (el) el.scrollTop = el.scrollHeight;
			})()
		`, nil).Do(ctx)

				if err != nil {
					log.Printf("⚠️ Scroll error on iteration %d: %v\n", i+1, err)
					continue // Continue with next iteration
				}
				time.Sleep(1 * time.Second) // Reduced from 2 to 1 second
			}
			return nil
		}),

		// Extract menu images (with shorter timeout)
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			var imageURLs []string
			err := chromedp.Evaluate(`Array.from(document.querySelectorAll('div.Uf0tqf.loaded'))
				.map(el => el.style.backgroundImage.replace(/url\(["']?(.*?)["']?\)/, '$1'))`, &imageURLs).Do(ctx)

			if err != nil {
				log.Printf("⚠️ Failed to extract menu images: %v\n", err)
				return nil // Don't fail the entire operation
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

	ctx, cancel = context.WithTimeout(ctx, 90*time.Second) // Increased timeout to 90 seconds
	defer cancel()

	log.Printf("🚀 Navigating to: %s\n", pageURL)

	reviewButtonSelector := fmt.Sprintf(`div.RWPxGd button.hh2c6[aria-label="Ulasan untuk %s"]`, nameRestaurant)
	var reviewTexts []string
	err := chromedp.Run(ctx,
		chromedp.Navigate(pageURL),
		chromedp.Sleep(3*time.Second), // Reduced from 10 to 3 seconds

		// Check if review button exists before clicking (with shorter timeout)
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			var exists bool
			if err := chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s') !== null`, reviewButtonSelector), &exists).Do(ctx); err != nil {
				log.Printf("⚠️ Could not check review button: %v\n", err)
				return nil // Don't fail the entire operation
			}
			if !exists {
				log.Println("⚠️ Review button not found, skipping review extraction.")
				return nil
			}
			return chromedp.Click(reviewButtonSelector, chromedp.ByQuery).Do(ctx)
		}),

		// Wait for reviews to load (with shorter timeout)
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()

			err := chromedp.WaitVisible("div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.GHT2ce div.MyEned span.wiI7pd", chromedp.ByQuery).Do(ctx)
			if err != nil {
				log.Printf("⚠️ Review elements not visible after timeout: %v\n", err)
				return nil // Don't fail, try to extract what we can
			}
			return nil
		}),

		// Reduced scrolling iterations and sleep time
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < 3; i++ { // Reduced from 5 to 3 iterations
				err := chromedp.Evaluate(`
			(function() {
				const el = document.querySelector('div.m6QErb.DxyBCb.kA9KIf.dS8AEf.XiKgde');
				if (el) el.scrollTop = el.scrollHeight;
			})()
		`, nil).Do(ctx)

				if err != nil {
					log.Printf("⚠️ Scroll error on iteration %d: %v\n", i+1, err)
					continue // Continue with next iteration
				}
				time.Sleep(1 * time.Second) // Reduced from 2 to 1 second
			}
			return nil
		}),

		// Extract review texts (with shorter timeout)
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			var exists bool

			// Check if at least one review element is present
			if err := chromedp.Evaluate(`document.querySelector('div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.GHT2ce div.MyEned span.wiI7pd') !== null`, &exists).Do(ctx); err != nil {
				log.Printf("⚠️ Could not check for review elements: %v\n", err)
				return nil // Don't fail the entire operation
			}
			if !exists {
				log.Println("⚠️ No reviews found.")
				return nil
			}

			// Extract inner text of all matched review elements
			err := chromedp.Evaluate(`
		Array.from(document.querySelectorAll('div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.GHT2ce div.MyEned span.wiI7pd'))
			.map(el => el.innerText)
	`, &reviewTexts).Do(ctx)

			if err != nil {
				log.Printf("⚠️ Failed to extract review texts: %v\n", err)
				return nil // Don't fail the entire operation
			}

			return nil
		}),
	)

	if err != nil {
		log.Printf("❌ Failed to scrape reviews from %s: %v\n", pageURL, err)
		return nil, err
	}

	log.Printf("Reviews: %s", strings.Join(reviewTexts, ", "))

	return reviewTexts, nil
}

func extractData(html string, searchLat, searchLong float64) []models.Place {
	var places []models.Place
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Println("Error parsing HTML:", err)
		return nil
	}

	doc.Find(".Nv2PK").Each(func(i int, s *goquery.Selection) {
		reviewCount := s.Find(".UY7F9").Text()
		reviewCountClean := cleanReviewCount(reviewCount)

		rawImageURL := s.Find("img").AttrOr("src", "N/A")
		enhancedImageURL := replaceImageProfileQuality(rawImageURL)

		// Use search coordinates as center point
		// Add small offset based on index to simulate different positions
		offset := float64(i) * 0.001 // Small offset for each restaurant
		restaurantLat := searchLat + offset
		restaurantLong := searchLong + offset

		place := models.Place{
			Title:       s.Find(".qBF1Pd").Text(),
			Rating:      s.Find(".MW4etd").Text(),
			ReviewCount: reviewCountClean,
			Location: models.GeoLocation{
				Latitude:  restaurantLat,
				Longitude: restaurantLong,
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

	// Step 3: Scrape additional async data (menu + reviews) with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	menuChan := make(chan []string, 1)
	reviewChan := make(chan []string, 1)
	errChan := make(chan error, 2)

	// Menu scraping
	go func() {
		menu, address, err := s.scrapeDataMenu(mapsLink)
		if err != nil {
			log.Printf("⚠️ Menu scraping failed for %s: %v\n", place.Title, err)
			errChan <- err
			select {
			case menuChan <- nil:
			case <-ctx.Done():
			}
			return
		}
		place.Address = address
		select {
		case menuChan <- menu:
		case <-ctx.Done():
			log.Printf("⚠️ Menu scraping timeout for %s\n", place.Title)
		}
	}()

	// Review scraping
	go func() {
		reviews, err := scrapeDataReview(mapsLink, place.Title)
		if err != nil {
			log.Printf("⚠️ Review scraping failed for %s: %v\n", place.Title, err)
			errChan <- err
			select {
			case reviewChan <- nil:
			case <-ctx.Done():
			}
			return
		}
		select {
		case reviewChan <- reviews:
		case <-ctx.Done():
			log.Printf("⚠️ Review scraping timeout for %s\n", place.Title)
		}
	}()

	// Wait for both operations with timeout
	var menuLink []string
	var reviewUser []string

	select {
	case menuLink = <-menuChan:
	case <-ctx.Done():
		log.Printf("⚠️ Menu channel timeout for %s\n", place.Title)
		menuLink = nil
	}

	select {
	case reviewUser = <-reviewChan:
	case <-ctx.Done():
		log.Printf("⚠️ Review channel timeout for %s\n", place.Title)
		reviewUser = nil
	}

	// Step 4: Attach data if available
	if menuLink != nil || reviewUser != nil {
		if menuLink != nil {
			place.MenuLink = menuLink
		}
		if reviewUser != nil {
			place.Reviews = reviewUser
		}

		// Only analyze images if we have menu links
		if len(menuLink) > 0 {
			menuList, err := s.OpenAIService.AnalyzeImages(context.Background(), menuLink)
			if err != nil {
				log.Printf("❌ Error analyzing images: %v\n", err)
				// Continue with the place even if image analysis fails
				err := s.RestaurantService.SaveRestaurantsHaram(context.Background(), []*models.Place{place})
				if err != nil {
					log.Printf("❌ Bulk save (Haram) failed: %v\n", err)
				}
				return &RestaurantStatusResponse{
					Status: "haram",
					Title:  place.Title,
				}, nil
			} else if menuList != nil {
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
			} else {
				log.Printf("⚠️ menuList is nil for %s, marking as haram\n", place.Title)
				err := s.RestaurantService.SaveRestaurantsHaram(context.Background(), []*models.Place{place})
				if err != nil {
					log.Printf("❌ Bulk save (Haram) failed: %v\n", err)
				}
				return &RestaurantStatusResponse{
					Status: "haram",
					Title:  place.Title,
				}, nil
			}
		} else {
			// No menu links available, save as haram
			log.Printf("⚠️ No menu links for %s, marking as haram\n", place.Title)
			err := s.RestaurantService.SaveRestaurantsHaram(context.Background(), []*models.Place{place})
			if err != nil {
				log.Printf("❌ Bulk save (Haram) failed: %v\n", err)
			}
			return &RestaurantStatusResponse{
				Status: "haram",
				Title:  place.Title,
			}, nil
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

	ctx, cancel = context.WithTimeout(ctx, 90*time.Second) // Increased from 60 to 90 seconds
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
