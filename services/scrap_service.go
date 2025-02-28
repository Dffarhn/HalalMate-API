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
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

type ScrapService struct {
	FirestoreClient *firestore.Client
	OpenAIService   *OpenAIService
	RestaurantService *RestaurantService
}

// NewScrapService initializes ScrapService with Firestore and OpenAI service
func NewScrapService(openAIService *OpenAIService) *ScrapService {
	return &ScrapService{
		FirestoreClient: database.GetFirestoreClient(),
		OpenAIService:   openAIService,
		RestaurantService: NewRestaurantService(),
	}
}

// ScrapePlaces fetches places from Google Maps
func (s *ScrapService) ScrapePlaces(searchURLs []string, placeChan chan<- models.Place, doneChan chan<- bool) {
	var wg sync.WaitGroup

	for _, pageURL := range searchURLs {
		wg.Add(1)
		go func(pageURL string) {
			defer wg.Done()

			log.Printf("Scraping started for URL: %s\n", pageURL)
			places := scrapeData(pageURL)

			// Another WaitGroup for menu scraping
			var menuWg sync.WaitGroup
			sem := make(chan struct{}, 5) // Limit to 5 concurrent menu scraping goroutines

			maxPlaces := 20

			for i := 0; i < maxPlaces; i++ { // Limit to maxPlaces
				menuWg.Add(1)
				sem <- struct{}{} // Acquire a slot

				go func(i int) {
					defer menuWg.Done()
					defer func() { <-sem }() // Release slot after completion

					// Run menu and review scraping in parallel
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

					// Only add to the channel if menuLink and reviewUser are not empty
					if menuLink != nil && reviewUser != nil {
						places[i].MenuLink = menuLink
						places[i].Reviews = reviewUser
						menuList, err := s.OpenAIService.AnalyzeImages(context.Background(), menuLink)
						if err != nil {
							errChan <- err
						}
						places[i].Menu = menuList

						placeChan <- places[i]

						// Save the place to Firestore
						err = s.RestaurantService.SaveRestaurant(context.Background(), &places[i])
						if err != nil {
							log.Printf("❌ Failed to save restaurant %s: %v\n", places[i].Title, err)
						} else {
							log.Printf("✅ Successfully saved restaurant: %s\n", places[i].Title)
						}
					}

					// Handle errors (if any)
					close(errChan)
					for err := range errChan {
						log.Printf("❌ Error processing %s: %v\n", places[i].Title, err)
					}
				}(i)
			}

			menuWg.Wait() // Wait for all menu scraping goroutines to complete
		}(pageURL)
	}

	wg.Wait()
	close(placeChan) // Close channel after all places are scraped
	doneChan <- true // Signal that scraping is complete
}

func scrapeData(pageURL string) []models.Place {
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
			return emulation.SetGeolocationOverride().
				WithLatitude(3.1127).
				WithLongitude(101.5501).
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

// func scrapeData(pageURL string) []models.Place {
// 	ctx, cancel := chromedp.NewContext(context.Background())
// 	defer cancel()

// 	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
// 	defer cancel()

// 	searchQuery := extractSearchQuery(pageURL)
// 	if searchQuery == "" {
// 		log.Println("Failed to extract search query from URL")
// 		return nil
// 	}

// 	var pageHTML string
// 	log.Printf("Navigating to page: %s\n", pageURL)
// 	err := chromedp.Run(ctx,

// 		chromedp.ActionFunc(func(ctx context.Context) error {
// 			return emulation.SetGeolocationOverride().
// 				WithLatitude(3.1127).
// 				WithLongitude(101.5501).
// 				WithAccuracy(1).
// 				Do(ctx)
//         }),
// 		chromedp.Navigate(pageURL),
// 		chromedp.Sleep(2*time.Second),
// 		chromedp.WaitVisible(`[aria-label="Hasil untuk `+searchQuery+`"]`, chromedp.ByQuery),
// 		scrollMultipleTimes(5, searchQuery),
// 		chromedp.OuterHTML("body", &pageHTML),
// 	)
// 	if err != nil {
// 		log.Printf("Failed to load page %s: %v\n", pageURL, err)
// 		return nil
// 	}

// 	log.Println("Extracting data from page...")
// 	return extractData(pageHTML)
// }

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

		// scrollPhotoMenu(3), // Scroll multiple times to load more images
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < 5; i++ { // Scroll multiple times
				err := chromedp.Run(ctx,
					chromedp.Evaluate(`window.scrollBy(0, document.body.scrollHeight)`, nil),
					chromedp.Sleep(2*time.Second), // Wait for more images to load
				)
				if err != nil {
					return err
				}
			}
			return nil
		}),
		

		// Extract menu images if found
		// chromedp.ActionFunc(func(ctx context.Context) error {
		// 	// Wait for images to load before checking
		// 	err := chromedp.WaitVisible("div.U39Pmb div.Uf0tqf.loaded", chromedp.ByQuery).Do(ctx)
		// 	if err != nil {
		// 		log.Println("⚠️ No menu images found (WaitVisible failed).")
		// 		return nil // Avoid stopping execution
		// 	}

		// 	var exists bool
		// 	if err := chromedp.Evaluate(`document.querySelector('div.U39Pmb div.Uf0tqf') !== null`, &exists).Do(ctx); err != nil {
		// 		return err
		// 	}

		// 	if exists {
		// 		return chromedp.AttributesAll("div.U39Pmb div.Uf0tqf", &imageElements, chromedp.ByQueryAll).Do(ctx)
		// 	}

		// 	log.Println("⚠️ No menu images found.")
		// 	return nil
		// }),

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

	// imageMenuList := extractImageURLs(imageElements)

	return imageMenuList, addressRestaurant, nil
}

func scrapeDataReview(pageURL, nameRestaurant string) ([]string, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 60*time.Second) // Set timeout
	defer cancel()

	var nodes []*cdp.Node // Stores review text elements

	log.Printf("🚀 Navigating to: %s\n", pageURL)

	reviewButtonSelector := fmt.Sprintf(`div.RWPxGd button.hh2c6[aria-label="Ulasan untuk %s"]`, nameRestaurant)

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
			for i := 0; i < 5; i++ { // Scroll multiple times
				err := chromedp.Run(ctx,
					chromedp.Evaluate(`window.scrollBy(0, document.body.scrollHeight)`, nil),
					chromedp.Sleep(2*time.Second), // Wait for more images to load
				)
				if err != nil {
					return err
				}
			}
			return nil
		}),

		// Extract review texts if section is visible
		chromedp.ActionFunc(func(ctx context.Context) error {
			var exists bool
			if err := chromedp.Evaluate(`document.querySelector('div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.GHT2ce div.MyEned span.wiI7pd') !== null`, &exists).Do(ctx); err != nil {
				return err
			}
			if exists {
				return chromedp.Nodes(`div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.GHT2ce div.MyEned span.wiI7pd`, &nodes, chromedp.ByQueryAll).Do(ctx)
			}
			log.Println("⚠️ No reviews found.")
			return nil
		}),
	)

	if err != nil {
		log.Printf("❌ Failed to scrape reviews from %s: %v\n", pageURL, err)
		return nil, err
	}

	return extractReviewTexts(nodes), nil
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

		place := models.Place{
			Title:       s.Find(".qBF1Pd").Text(),
			Rating:      s.Find(".MW4etd").Text(),
			ReviewCount: s.Find(".UY7F9").Text(),
			Location: models.GeoLocation{

				Latitude:  lat,
				Longitude: long,
			},
			OpeningStatus: s.Find(".W4Efsd span[style*='color']").Text(),
			ImageURL:      s.Find("img").AttrOr("src", "N/A"),
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

// func extractImageURLs(elements []map[string]string) []string {
// 	re := regexp.MustCompile(`url\(["']?(.*?)["']?\)`) // Extract URLs from "style" attributes
// 	var urls []string
// 	for _, elem := range elements {
// 		if style, ok := elem["style"]; ok {
// 			if match := re.FindStringSubmatch(style); len(match) > 1 {
// 				url := match[1]
// 				if url != "//:0" { // Exclude "//:0"
// 					urls = append(urls, url)
// 				}
// 			}
// 		}
// 	}
// 	return urls
// }

func extractReviewTexts(nodes []*cdp.Node) []string {
	var reviews []string
	for _, node := range nodes {
		if text := node.NodeValue; text != "" {
			reviews = append(reviews, text)
		} else if len(node.Children) > 0 {
			reviews = append(reviews, node.Children[0].NodeValue)
		}
	}
	return reviews
}

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

// func scrollReviews(times int) chromedp.Tasks {
// 	tasks := chromedp.Tasks{}
// 	for i := 0; i < times; i++ {
// 		tasks = append(tasks,
// 			chromedp.ScrollIntoView(`div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.MyEned span.wiI7pd`, chromedp.ByQuery),
// 			chromedp.Sleep(2*time.Second),
// 		)
// 	}
// 	return tasks
// }

// func scrollPhotoMenu(times int) chromedp.Tasks {
// 	tasks := chromedp.Tasks{}
// 	for i := 0; i < times; i++ {
// 		tasks = append(tasks,
// 			chromedp.Evaluate(`document.querySelector('div.m6QErb.DxyBCb.kA9KIf.dS8AEf.XiKgde div.m6QErb.XiKgde').scrollBy(0, 500);`, nil),
// 			chromedp.Sleep(500*time.Millisecond),
// 		)
// 	}
// 	return tasks
// }

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
