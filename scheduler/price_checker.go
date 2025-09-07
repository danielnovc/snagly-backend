package scheduler

import (
	"fmt"
	"log"

	"distrack/models"
	"distrack/repository"
	"distrack/scraper"

	"github.com/robfig/cron/v3"
)

type PriceChecker struct {
	cron    *cron.Cron
	urlRepo *repository.URLRepository
	alertRepo *repository.AlertRepository
	scraper *scraper.PriceScraper
}

func NewPriceChecker() (*PriceChecker, error) {
	priceScraper, err := scraper.NewPriceScraper()
	if err != nil {
		return nil, err
	}

	return &PriceChecker{
		cron:      cron.New(cron.WithSeconds()),
		urlRepo:   repository.NewURLRepository(),
		alertRepo: repository.NewAlertRepository(),
		scraper:   priceScraper,
	}, nil
}

// Start starts the scheduled price checking
func (pc *PriceChecker) Start() {
	// Schedule price checks every 12 hours (at 00:00 and 12:00)
	_, err := pc.cron.AddFunc("0 0 */12 * * *", pc.checkAllPrices)
	if err != nil {
		log.Printf("Failed to schedule price checker: %v", err)
		return
	}

	// Also run immediately on startup
	go pc.checkAllPrices()

	pc.cron.Start()
	log.Println("Price checker scheduled to run every 12 hours")
}

// Stop stops the scheduled price checking
func (pc *PriceChecker) Stop() {
	if pc.cron != nil {
		pc.cron.Stop()
	}
	if pc.scraper != nil {
		pc.scraper.Close()
	}
}

// checkAllPrices checks prices for all tracked URLs
func (pc *PriceChecker) checkAllPrices() {
	log.Println("Starting scheduled price check for all tracked URLs")

	urls, err := pc.urlRepo.GetTrackedURLs()
	if err != nil {
		log.Printf("Failed to get tracked URLs: %v", err)
		return
	}

	if len(urls) == 0 {
		log.Println("No URLs to check")
		return
	}

	log.Printf("Checking prices for %d URLs", len(urls))

	for _, url := range urls {
		go pc.checkURLPrice(url)
	}
}

// checkURLPrice checks the price for a specific URL
func (pc *PriceChecker) checkURLPrice(url models.TrackedURL) {
	log.Printf("Checking price for: %s (%s)", url.Name, url.URL)

	// Scrape current price
	priceData, err := pc.scraper.ScrapePrice(url.URL)
	if err != nil {
		log.Printf("Failed to scrape price for %s: %v", url.URL, err)
		return
	}

	// Validate price change before updating and triggering alerts
	priceChangeReason := url.GetPriceChangeReason(priceData.CurrentPrice)
	isRealistic := url.IsPriceChangeRealistic(priceData.CurrentPrice)
	
	oldPrice := "N/A"
	if url.CurrentPrice.Valid {
		oldPrice = fmt.Sprintf("$%.2f", url.CurrentPrice.Float64)
	}
	
	if isRealistic {
		log.Printf("Current price for %s: $%.2f (was %s) - %s", url.Name, priceData.CurrentPrice, oldPrice, priceChangeReason)
	} else {
		log.Printf("⚠️  UNREALISTIC PRICE for %s: $%.2f (was %s) - %s", url.Name, priceData.CurrentPrice, oldPrice, priceChangeReason)
	}

	// Only update price and check alerts if the change is realistic
	if isRealistic {
		// Update URL with new price
		if err := pc.urlRepo.UpdateURLPrice(url.ID, priceData); err != nil {
			log.Printf("Failed to update URL price for %s: %v", url.URL, err)
			return
		}

		// Add to price history
		if err := pc.urlRepo.AddPriceHistory(url.ID, priceData); err != nil {
			log.Printf("Failed to add price history for %s: %v", url.URL, err)
		}

		// Check for alerts
		triggeredAlerts, err := pc.alertRepo.CheckAlerts(url.ID, priceData.CurrentPrice)
		if err != nil {
			log.Printf("Failed to check alerts for %s: %v", url.URL, err)
		} else {
			// Log triggered alerts
			for _, alert := range triggeredAlerts {
				log.Printf("🚨 ALERT TRIGGERED for %s: Price dropped to $%.2f", url.Name, priceData.CurrentPrice)
				
				switch alert.AlertType {
				case "price_drop":
					log.Printf("   Target price: $%.2f", alert.TargetPrice)
				case "percentage_drop":
					log.Printf("   Target percentage: %.1f%%", alert.Percentage)
				}
			}
		}

		// Log price changes
		if url.CurrentPrice.Valid && priceData.CurrentPrice != url.CurrentPrice.Float64 {
			change := priceData.CurrentPrice - url.CurrentPrice.Float64
			changePercent := (change / url.CurrentPrice.Float64) * 100
			
			if change < 0 {
				log.Printf("📉 Price DROPPED for %s: $%.2f → $%.2f (%.1f%%)", 
					url.Name, url.CurrentPrice.Float64, priceData.CurrentPrice, changePercent)
			} else {
				log.Printf("📈 Price INCREASED for %s: $%.2f → $%.2f (+%.1f%%)", 
					url.Name, url.CurrentPrice.Float64, priceData.CurrentPrice, changePercent)
			}
		}
	} else {
		log.Printf("🚫 Skipping price update and alert checks for %s due to unrealistic price change", url.Name)
	}
}

// ManualCheck allows manual triggering of price checks
func (pc *PriceChecker) ManualCheck() {
	log.Println("Manual price check triggered")
	pc.checkAllPrices()
}
