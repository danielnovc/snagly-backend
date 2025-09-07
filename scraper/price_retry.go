package scraper

import (
	"fmt"
	"log"
	"time"

	"distrack/models"
)

// PriceRetryOptions configures retry behavior
type PriceRetryOptions struct {
	MaxRetries     int           // Maximum number of retries
	RetryDelay     time.Duration // Delay between retries
	UseOCROnFail   bool          // Use OCR when regular extraction fails
	UseOCROnRetry  bool          // Use OCR for retries
	ValidatePrice  func(float64) bool // Function to validate if price is reasonable
}

// DefaultPriceRetryOptions returns default retry options
func DefaultPriceRetryOptions() *PriceRetryOptions {
	return &PriceRetryOptions{
		MaxRetries:    3,
		RetryDelay:    2 * time.Second,
		UseOCROnFail:  true,
		UseOCROnRetry: true,
		ValidatePrice: func(price float64) bool {
			// Default validation: price should be positive and reasonable
			return price > 0 && price < 1000000
		},
	}
}

// RetryPriceExtraction attempts to extract price with retry logic
func (ps *PriceScraper) RetryPriceExtraction(url string, options *PriceRetryOptions) (*models.PriceData, error) {
	if options == nil {
		options = DefaultPriceRetryOptions()
	}

	log.Printf("🔄 Starting price extraction with retry for: %s", url)

	// First attempt with regular extraction
	priceData, err := ps.ScrapePrice(url)
	if err != nil {
		log.Printf("❌ Regular extraction failed: %v", err)
		
		// Try OCR if enabled
		if options.UseOCROnFail && ps.ocrExtractor != nil {
			log.Println("🔄 Retrying with OCR...")
			return ps.retryWithOCR(url)
		}
		
		return nil, fmt.Errorf("price extraction failed: %v", err)
	}

	// Validate the extracted price
	if options.ValidatePrice != nil && !options.ValidatePrice(priceData.CurrentPrice) {
		log.Printf("⚠️  Extracted price $%.2f seems unreasonable, retrying...", priceData.CurrentPrice)
		
		// Try OCR if enabled
		if options.UseOCROnRetry && ps.ocrExtractor != nil {
			log.Println("🔄 Retrying with OCR...")
			return ps.retryWithOCR(url)
		}
	}

	log.Printf("✅ Price extraction successful: $%.2f", priceData.CurrentPrice)
	return priceData, nil
}

// retryWithOCR retries price extraction using OCR only
func (ps *PriceScraper) retryWithOCR(url string) (*models.PriceData, error) {
	if ps.ocrExtractor == nil {
		return nil, fmt.Errorf("OCR extractor not available")
	}

	log.Println("🔍 Using OCR-only extraction...")
	
	// Create a new page for OCR
	page := ps.browser.MustPage(url)
	defer page.MustClose()
	
	// Set viewport and wait for page to load
	page.MustSetViewport(1366, 768, 1.0, false)
	page.MustWaitLoad()
	time.Sleep(3 * time.Second)
	
	// Extract price using OCR with common price selectors
	selectors := []string{
		"[class*='price']", "[class*='Price']", "[class*='PRICE']",
		"[id*='price']", "[id*='Price']", "[id*='PRICE']",
		".price", ".Price", ".PRICE",
		"span", "div", "p", "h1", "h2", "h3", "h4", "h5", "h6",
	}
	
	price, err := ps.ocrExtractor.ExtractPriceWithOCR(page, selectors)
	if err != nil {
		return nil, fmt.Errorf("OCR extraction failed: %v", err)
	}
	
	// Create price data
	priceData := &models.PriceData{
		CurrentPrice:  price,
		OriginalPrice: price,
		Currency:      "USD", // Default to USD for OCR
		IsOnSale:      false,
		Source:        "OCR",
	}
	
	log.Printf("✅ OCR extraction successful: $%.2f", priceData.CurrentPrice)
	return priceData, nil
}

// ForceOCRExtraction forces OCR extraction regardless of regular extraction result
func (ps *PriceScraper) ForceOCRExtraction(url string) (*models.PriceData, error) {
	if ps.ocrExtractor == nil {
		return nil, fmt.Errorf("OCR extractor not available")
	}

	log.Printf("🔍 Force OCR extraction for: %s", url)
	return ps.retryWithOCR(url)
}

// CompareExtractionMethods compares regular and OCR extraction results
func (ps *PriceScraper) CompareExtractionMethods(url string) (*PriceComparison, error) {
	log.Printf("🔍 Comparing extraction methods for: %s", url)
	
	comparison := &PriceComparison{
		URL: url,
	}
	
	// Try regular extraction
	start := time.Now()
	regularData, regularErr := ps.ScrapePrice(url)
	comparison.RegularTime = time.Since(start)
	
	if regularErr != nil {
		comparison.RegularError = regularErr.Error()
		log.Printf("❌ Regular extraction failed: %v", regularErr)
	} else {
		comparison.RegularPrice = regularData.CurrentPrice
		comparison.RegularSource = "network" // Default source for regular extraction
		log.Printf("✅ Regular extraction: $%.2f", regularData.CurrentPrice)
	}
	
	// Try OCR extraction
	if ps.ocrExtractor != nil {
		start = time.Now()
		ocrData, ocrErr := ps.ForceOCRExtraction(url)
		comparison.OCRTime = time.Since(start)
		
		if ocrErr != nil {
			comparison.OCRError = ocrErr.Error()
			log.Printf("❌ OCR extraction failed: %v", ocrErr)
		} else {
			comparison.OCRPrice = ocrData.CurrentPrice
			comparison.OCRSource = ocrData.Source
			log.Printf("✅ OCR extraction: $%.2f", ocrData.CurrentPrice)
		}
	} else {
		comparison.OCRError = "OCR extractor not available"
	}
	
	return comparison, nil
}

// PriceComparison holds results from comparing extraction methods
type PriceComparison struct {
	URL          string        `json:"url"`
	RegularPrice float64       `json:"regular_price,omitempty"`
	RegularError string        `json:"regular_error,omitempty"`
	RegularTime  time.Duration `json:"regular_time"`
	RegularSource string       `json:"regular_source,omitempty"`
	OCRPrice     float64       `json:"ocr_price,omitempty"`
	OCRError     string        `json:"ocr_error,omitempty"`
	OCRTime      time.Duration `json:"ocr_time"`
	OCRSource    string        `json:"ocr_source,omitempty"`
}

// String returns a formatted string representation of the comparison
func (pc *PriceComparison) String() string {
	result := fmt.Sprintf("Price Comparison for: %s\n", pc.URL)
	result += fmt.Sprintf("Regular Method: ")
	if pc.RegularError != "" {
		result += fmt.Sprintf("❌ %s (%v)", pc.RegularError, pc.RegularTime)
	} else {
		result += fmt.Sprintf("✅ $%.2f (%v) [%s]", pc.RegularPrice, pc.RegularTime, pc.RegularSource)
	}
	result += "\n"
	
	result += fmt.Sprintf("OCR Method:     ")
	if pc.OCRError != "" {
		result += fmt.Sprintf("❌ %s (%v)", pc.OCRError, pc.OCRTime)
	} else {
		result += fmt.Sprintf("✅ $%.2f (%v) [%s]", pc.OCRPrice, pc.OCRTime, pc.OCRSource)
	}
	result += "\n"
	
	if pc.RegularPrice > 0 && pc.OCRPrice > 0 {
		diff := pc.RegularPrice - pc.OCRPrice
		result += fmt.Sprintf("Difference:     $%.2f\n", diff)
	}
	
	return result
}
