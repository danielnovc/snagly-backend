package scheduler

import (
	"log"
	"time"
)

// RetryServiceFuncs contains the functions needed by RetryService
type RetryServiceFuncs struct {
	GetURLsForRetry     func() ([]interface{}, error)
	ScrapePrice         func(url string, urlID int) (interface{}, error)
	MarkPriceCheckFailed func(urlID int) error
	MarkPriceCheckSuccess func(urlID int) error
	UpdateURLPrice      func(urlID int, priceData interface{}) error
	AddPriceHistory     func(urlID int, priceData interface{}) error
	CheckAlerts         func(urlID int, price float64) ([]interface{}, error)
}

type RetryService struct {
	funcs    *RetryServiceFuncs
	stopChan chan bool
}

func NewRetryService(funcs *RetryServiceFuncs) *RetryService {
	return &RetryService{
		funcs:    funcs,
		stopChan: make(chan bool),
	}
}

// Start starts the retry service
func (rs *RetryService) Start() {
	log.Println("🔄 Starting retry service...")
	
	go func() {
		ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				rs.processRetries()
			case <-rs.stopChan:
				log.Println("🛑 Retry service stopped")
				return
			}
		}
	}()
}

// Stop stops the retry service
func (rs *RetryService) Stop() {
	close(rs.stopChan)
}

// processRetries processes URLs that need to be retried
func (rs *RetryService) processRetries() {
	urls, err := rs.funcs.GetURLsForRetry()
	if err != nil {
		log.Printf("❌ Failed to get URLs for retry: %v", err)
		return
	}

	if len(urls) == 0 {
		return
	}

	log.Printf("🔄 Processing %d URLs for retry", len(urls))

	for _, urlInterface := range urls {
		// Type assertion to get URL data
		url, ok := urlInterface.(interface {
			ShouldRetry() bool
			GetID() int
			GetURL() string
			GetName() string
			GetRetryCount() int
		})
		
		if !ok {
			log.Printf("❌ Invalid URL type for retry")
			continue
		}
		
		if !url.ShouldRetry() {
			continue
		}

		log.Printf("🔄 Retrying price check for %s (attempt %d/5)", url.GetName(), url.GetRetryCount()+1)

		// Try to scrape price
		priceData, err := rs.funcs.ScrapePrice(url.GetURL(), url.GetID())
		if err != nil {
			log.Printf("❌ Retry failed for %s: %v", url.GetName(), err)
			
			// Mark as failed and schedule next retry
			if retryErr := rs.funcs.MarkPriceCheckFailed(url.GetID()); retryErr != nil {
				log.Printf("❌ Failed to mark retry as failed: %v", retryErr)
			}
			continue
		}

		// Success! Update price and reset retry count
		log.Printf("✅ Retry successful for %s", url.GetName())

		// Mark as successful
		if retryErr := rs.funcs.MarkPriceCheckSuccess(url.GetID()); retryErr != nil {
			log.Printf("❌ Failed to mark retry as successful: %v", retryErr)
		}

		// Update URL with new price
		if err := rs.funcs.UpdateURLPrice(url.GetID(), priceData); err != nil {
			log.Printf("❌ Failed to update URL price after retry: %v", err)
			continue
		}

		// Add to price history
		if err := rs.funcs.AddPriceHistory(url.GetID(), priceData); err != nil {
			log.Printf("❌ Failed to add price history after retry: %v", err)
		}

		// Check for alerts (we'll need to extract price from priceData)
		// This is a simplified version - you might need to adjust based on your PriceData structure
		if priceDataInterface, ok := priceData.(interface{ GetCurrentPrice() float64 }); ok {
			price := priceDataInterface.GetCurrentPrice()
			triggeredAlerts, err := rs.funcs.CheckAlerts(url.GetID(), price)
			if err != nil {
				log.Printf("❌ Failed to check alerts after retry: %v", err)
			}
			
			if len(triggeredAlerts) > 0 {
				log.Printf("🔔 %d alerts triggered for URL ID %d", len(triggeredAlerts), url.GetID())
			}
		}
	}
}
