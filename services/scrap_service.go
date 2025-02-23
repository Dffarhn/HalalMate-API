package services

import (
	"HalalMate/config/database"
	"HalalMate/models"
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

type ScrapService struct {
	FirestoreClient *firestore.Client
}

// NewAuthService initializes AuthService with Firestore client
func NewScrapService() *ScrapService {
	return &ScrapService{
		FirestoreClient: database.GetFirestoreClient(),
	}
}

// ScrapePlaces fetches places from Google Maps
func ScrapePlaces(searchURLs []string, placeChan chan<- models.Place, doneChan chan<- bool) {
	var wg sync.WaitGroup

	for _, pageURL := range searchURLs {
		wg.Add(1)
		go func(pageURL string) {
			defer wg.Done()

			log.Printf("Scraping started for URL: %s\n", pageURL)
			places := scrapeData(pageURL)

			// Another WaitGroup for menu scraping
			var menuWg sync.WaitGroup
			sem := make(chan struct{}, 10) // Limit to 10 concurrent menu scraping goroutines

			for i := range places {
				menuWg.Add(1)
				sem <- struct{}{} // Acquire a slot

				go func(i int) {
					defer menuWg.Done()
					defer func() { <-sem }() // Release slot after completion

					menuLink, reviewUser := scrapeDataMenu(places[i].MapsLink, places[i].Title)
					places[i].MenuLink = menuLink
					places[i].Reviews = reviewUser

					// Stream the updated place via channel
					placeChan <- places[i]
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
	ctx, cancel := chromedp.NewContext(context.Background())
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

func scrapeDataMenu(pageURL string, nameRestaurant string) (string, []string) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var imageURL string
	var reviewTexts []string
	var nodes []*cdp.Node // Menampung semua elemen review

	log.Printf("Navigating to: %s\n", pageURL)

	reviewButtonSelector := fmt.Sprintf(`div.RWPxGd button.hh2c6[aria-label="Ulasan untuk %s"]`, nameRestaurant)

	err := chromedp.Run(ctx,
		chromedp.Navigate(pageURL),

		chromedp.WaitVisible(`div.ofKBgf button.K4UgGe[aria-label="Menu"] img`, chromedp.ByQuery),
		chromedp.AttributeValue(`div.ofKBgf button.K4UgGe[aria-label="Menu"] img`, "src", &imageURL, nil),

		chromedp.Click(reviewButtonSelector, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second), // Increase sleep time to allow reviews to load

		// Scroll multiple times to ensure all reviews are loaded
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < 5; i++ { // Adjust based on review loading behavior
				if err := chromedp.Run(ctx,
					chromedp.ScrollIntoView(`div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.MyEned span.wiI7pd`, chromedp.ByQuery),
					chromedp.Sleep(2*time.Second),
				); err != nil {
					return err
				}
			}
			return nil
		}),

		chromedp.WaitVisible(`div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.GHT2ce div.MyEned span.wiI7pd`, chromedp.ByQuery),

		chromedp.Nodes(`div.m6QErb.XiKgde div.jftiEf.fontBodyMedium div.GHT2ce div.MyEned span.wiI7pd`, &nodes, chromedp.ByQueryAll),
	)

	if err != nil {
		log.Printf("❌ Gagal mengambil data dari %s: %v\n", pageURL, err)
		return "N/A", nil
	}

	// Jika gambar menu tidak ditemukan
	if imageURL == "" {
		log.Printf("⚠️ Gambar menu tidak ditemukan di %s\n", pageURL)
		return "N/A", nil
	}

	// Konversi nodes ke teks ulasan
	for _, node := range nodes {
		var text string
		if node.NodeValue != "" {
			text = node.NodeValue
		} else if len(node.Children) > 0 {
			text = node.Children[0].NodeValue
		}

		if text != "" {
			reviewTexts = append(reviewTexts, text)
		}
	}

	log.Printf("✅ Ulasan ditemukan (%d): %+v\n", len(reviewTexts), reviewTexts)

	log.Printf("✅ Gambar menu ditemukan: %s\n", imageURL)
	log.Printf("✅ Ulasan ditemukan (%d): %+v\n", len(reviewTexts), reviewTexts)

	return imageURL, reviewTexts
}

func extractData(html string) []models.Place {
	var places []models.Place
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Println("Error parsing HTML:", err)
		return nil
	}

	doc.Find(".Nv2PK").Each(func(i int, s *goquery.Selection) {
		place := models.Place{
			Title:         s.Find(".qBF1Pd").Text(),
			Rating:        s.Find(".MW4etd").Text(),
			ReviewCount:   s.Find(".UY7F9").Text(),
			PriceRange:    s.Find(".e4rVHe fontBodyMedium").Text(),
			Category:      s.Find(".W4Efsd span").Text(),
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
