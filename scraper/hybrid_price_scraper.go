package scraper

import (
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"distrack/models"
	"distrack/repository"

	"github.com/go-rod/rod"
)

// HybridPriceScraper combines network extraction and OCR for maximum reliability
type HybridPriceScraper struct {
	priceScraper     *PriceScraper
	ocrExtractor     *EnhancedOCRExtractor
	localeParser     *LocaleParser
	// Simple preference system
	preferences map[string]string // URL -> preferred method
	prefRepo    *repository.PreferenceRepository
}

// PriceMatchResult represents the result of price matching between methods
type PriceMatchResult struct {
	NetworkPrice    float64
	OCRPrice        float64
	NetworkSuccess  bool
	OCRSuccess      bool
	PricesMatch     bool
	MatchConfidence float64
	FinalPrice      float64
	Method          string
	Error           string
	// Enhanced fields for intelligent decision making
	NetworkSource   string
	NetworkConfidence float64
	OCRConfidence   float64
	Reasons         []string
}

// NewHybridPriceScraper creates a new hybrid price scraper
func NewHybridPriceScraper() (*HybridPriceScraper, error) {
	priceScraper, err := NewPriceScraper()
	if err != nil {
		return nil, fmt.Errorf("failed to create price scraper: %v", err)
	}

	// Use HTTP-based extractor for separate containers
	// Use environment variables for service URLs
	yoloServiceURL := os.Getenv("YOLO_SERVICE_URL")
	if yoloServiceURL == "" {
		yoloServiceURL = "http://yolo-service:8000"
	}
	
	ocrServiceURL := os.Getenv("OCR_SERVICE_URL")
	if ocrServiceURL == "" {
		ocrServiceURL = "http://tesseract-service:5000"
	}

	ocrExtractor := NewEnhancedOCRExtractor(yoloServiceURL, ocrServiceURL)
	localeParser := NewLocaleParser()

	return &HybridPriceScraper{
		priceScraper:     priceScraper,
		ocrExtractor:     ocrExtractor,
		localeParser:     localeParser,
		preferences:      make(map[string]string),
		prefRepo:         repository.NewPreferenceRepository(),
	}, nil
}

// Close closes the hybrid scraper
func (hps *HybridPriceScraper) Close() {
	if hps.priceScraper != nil {
		hps.priceScraper.Close()
	}
}

// SetPreference sets the preferred method for a URL (in-memory)
func (hps *HybridPriceScraper) SetPreference(url, method string) {
	hps.preferences[url] = method
	log.Printf("✅ Set preference for %s: %s", url, method)
}

// GetPreference gets the preferred method for a URL (in-memory)
func (hps *HybridPriceScraper) GetPreference(url string) (string, bool) {
	method, exists := hps.preferences[url]
	return method, exists
}

// SaveUserChoice saves the user's choice to database
func (hps *HybridPriceScraper) SaveUserChoice(urlID int, choice *models.UserChoiceRequest) error {
	if hps.prefRepo != nil {
		err := hps.prefRepo.SavePreference(urlID, choice.ChosenMethod, 0.9) // High confidence for user choice
		if err != nil {
			return fmt.Errorf("failed to save preference: %v", err)
		}
	}
	
	// Also save to in-memory preferences
	hps.preferences[choice.ChosenSource] = choice.ChosenMethod
	log.Printf("✅ User choice saved: URL=%d, method=%s, price=%.2f", 
		urlID, choice.ChosenMethod, choice.ChosenPrice)
	
	return nil
}

// ScrapePriceWithHybridMethod performs hybrid price scraping with preference support
func (hps *HybridPriceScraper) ScrapePriceWithHybridMethod(url string, urlID int) (*models.PriceData, error) {
	// Check if we have a preferred method for this URL (database first, then memory)
	var preferredMethod string
	var usePreferred bool
	
	if hps.prefRepo != nil && urlID > 0 {
		if pref, err := hps.prefRepo.GetPreference(urlID); err == nil && pref != nil {
			if pref.SuccessRate > 0.8 { // Only use if high success rate
				preferredMethod = pref.Method
				usePreferred = true
				log.Printf("🎯 Using database preference '%s' for URL %d (success rate: %.2f)", 
					preferredMethod, urlID, pref.SuccessRate)
			}
		}
	}
	
	// Fall back to in-memory preferences
	if !usePreferred {
		if method, exists := hps.GetPreference(url); exists {
			preferredMethod = method
			usePreferred = true
			log.Printf("🎯 Using memory preference '%s' for %s", preferredMethod, url)
		}
	}

	// Use preferred method if available
	if usePreferred {
		switch preferredMethod {
		case "network":
			return hps.priceScraper.ScrapePrice(url)
		case "ocr":
			return hps.scrapeWithOCRMethod(url)
		case "hybrid":
			// Continue with hybrid approach
		}
	}

	// Always use hybrid approach for first scrape or when no preference exists
	return hps.performHybridScraping(url, urlID)
}

// performHybridScraping is the original hybrid logic
func (hps *HybridPriceScraper) performHybridScraping(url string, urlID int) (*models.PriceData, error) {
	log.Printf("🔍 Starting hybrid price extraction for: %s", url)

	// Create a new page for this extraction
	page := hps.priceScraper.GetBrowser().MustPage(url)
	defer page.MustClose()

	// Set viewport to 1600x1200 and wait for page to load
	page.MustSetViewport(1600, 1200, 1.0, false)
	page.MustWaitLoad()
	time.Sleep(8 * time.Second) // Wait for dynamic content
	page.MustWaitStable()
	time.Sleep(3 * time.Second)

	// Extract prices using both methods
	matchResult := hps.extractAndMatchPrices(page, url, urlID)

	// Determine the final result based on matching logic
	priceData, err := hps.determineFinalResult(matchResult, url)
	if err != nil {
		return nil, err
	}

	// Update success rate in database if available
	if hps.prefRepo != nil && urlID > 0 && priceData != nil {
		hps.prefRepo.UpdateSuccessRate(urlID, true)
	}
	
	return priceData, nil
}

// extractAndMatchPrices extracts prices using both network and OCR methods
// PRIORITY: YOLO+OCR first, then network extraction as fallback
func (hps *HybridPriceScraper) extractAndMatchPrices(page *rod.Page, url string, urlID int) *PriceMatchResult {
	result := &PriceMatchResult{
		Reasons: []string{},
	}

	// Method 1: YOLO+OCR extraction - PRIORITY METHOD (Internal) with timeout
	log.Printf("🎯 Attempting internal YOLO+OCR extraction for URL ID: %d...", urlID)
	ocrStart := time.Now()
	
	var ocrPrice float64
	var ocrErr error
	var ocrDuration time.Duration
	var ocrConfidence float64
	
	// Use a channel to implement timeout for OCR extraction
	ocrDone := make(chan struct{})
	go func() {
		defer close(ocrDone)
		
		// Take a screenshot for OCR processing
		screenshotPath, err := hps.takeScreenshot(page)
		if err != nil {
			log.Printf("❌ Failed to take screenshot: %v", err)
			ocrPrice = 0
			ocrErr = err
			ocrDuration = time.Since(ocrStart)
			return
		}
		
		// Use HTTP-based extractor for separate containers
		httpResult, httpErr := hps.ocrExtractor.ExtractPriceWithYOLO(screenshotPath)
		if httpErr == nil && httpResult != nil && httpResult.Success {
			ocrPrice = httpResult.Price
			ocrConfidence = httpResult.OCRConfidence
			log.Printf("✅ HTTP YOLO OCR successful: $%.2f (YOLO: %.3f, OCR: %.3f, %d detections)", 
				httpResult.Price, httpResult.YOLOConfidence, httpResult.OCRConfidence, httpResult.TotalDetections)
		} else {
			ocrPrice = 0
			ocrErr = httpErr
			if httpErr != nil {
				log.Printf("❌ HTTP YOLO OCR failed: %v", httpErr)
			} else if httpResult != nil {
				log.Printf("❌ HTTP YOLO OCR failed: %s", httpResult.Error)
			}
		}
		
		ocrDuration = time.Since(ocrStart)
		// Clean up screenshot file
		os.Remove(screenshotPath)
	}()
	
	// Wait for OCR completion or timeout (30 seconds)
	select {
	case <-ocrDone:
		// OCR completed
	case <-time.After(30 * time.Second):
		log.Printf("⏰ YOLO+OCR extraction timed out after 30 seconds")
		ocrPrice = 0
		ocrErr = fmt.Errorf("YOLO+OCR extraction timed out")
		ocrDuration = time.Since(ocrStart)
	}

	if ocrErr == nil && ocrPrice > 0 {
		// Try to parse with locale parser for better accuracy
		parsedPrice, _, parseErr := hps.localeParser.ParsePrice(fmt.Sprintf("%.2f", ocrPrice))
		if parseErr == nil {
			ocrPrice = parsedPrice
		}
		
		result.OCRPrice = ocrPrice
		result.OCRSuccess = true
		result.OCRConfidence = ocrConfidence
		log.Printf("✅ YOLO OCR extraction successful: $%.2f (took %v)", result.OCRPrice, ocrDuration)
	} else {
		log.Printf("❌ YOLO OCR extraction failed: %v (took %v)", ocrErr, ocrDuration)
	}

	// Method 2: Network extraction - FALLBACK METHOD with timeout
	log.Printf("🌐 Attempting network extraction - FALLBACK METHOD...")
	networkStart := time.Now()
	
	var networkPriceData *models.PriceData
	var networkErr error
	var networkDuration time.Duration
	
	// Use a channel to implement timeout for network extraction
	networkDone := make(chan struct{})
	go func() {
		defer close(networkDone)
		networkPriceData, networkErr = hps.priceScraper.ExtractFromNetworkRequests(page)
		networkDuration = time.Since(networkStart)
	}()
	
	// Wait for network extraction completion or timeout (20 seconds)
	select {
	case <-networkDone:
		// Network extraction completed
	case <-time.After(20 * time.Second):
		log.Printf("⏰ Network extraction timed out after 20 seconds")
		networkPriceData = nil
		networkErr = fmt.Errorf("Network extraction timed out")
		networkDuration = time.Since(networkStart)
	}

	if networkErr == nil && networkPriceData != nil && networkPriceData.CurrentPrice > 0 {
		result.NetworkPrice = networkPriceData.CurrentPrice
		result.NetworkSuccess = true
		result.NetworkSource = networkPriceData.Source
		result.NetworkConfidence = networkPriceData.Confidence
		if result.NetworkConfidence == 0 {
			result.NetworkConfidence = 0.8 // Default confidence for network
		}
		log.Printf("✅ Network extraction successful: $%.2f via %s (took %v)", 
			result.NetworkPrice, result.NetworkSource, networkDuration)
	} else {
		log.Printf("❌ Network extraction failed: %v (took %v)", networkErr, networkDuration)
	}

	// Check if prices match
	if result.NetworkSuccess && result.OCRSuccess {
		result.PricesMatch = hps.pricesMatch(result.NetworkPrice, result.OCRPrice)
		result.MatchConfidence = hps.calculateMatchConfidence(result.NetworkPrice, result.OCRPrice)
		
		if result.PricesMatch {
			log.Printf("🎯 Price match detected! Network: $%.2f, OCR: $%.2f (confidence: %.1f%%)", 
				result.NetworkPrice, result.OCRPrice, result.MatchConfidence*100)
		} else {
			log.Printf("⚠️ Price mismatch: Network: $%.2f, OCR: $%.2f (confidence: %.1f%%)", 
				result.NetworkPrice, result.OCRPrice, result.MatchConfidence*100)
		}
	}

	return result
}

// isAmazonDomain checks if the URL is from Amazon
func (hps *HybridPriceScraper) isAmazonDomain(url string) bool {
	return strings.Contains(strings.ToLower(url), "amazon.com") || 
		   strings.Contains(strings.ToLower(url), "amazon.") ||
		   strings.Contains(strings.ToLower(url), "amazon")
}

// determineFinalResult determines the final price based on domain-specific priority
func (hps *HybridPriceScraper) determineFinalResult(matchResult *PriceMatchResult, url string) (*models.PriceData, error) {
	isAmazon := hps.isAmazonDomain(url)
	
	// AMAZON DOMAIN LOGIC: Prioritize HTTP scraping over YOLO+OCR
	if isAmazon {
		log.Printf("🛒 Amazon domain detected - prioritizing HTTP scraping over YOLO+OCR")
		
		// Case 1: Network extraction succeeds = Primary Success for Amazon
		if matchResult.NetworkSuccess && matchResult.NetworkConfidence > 0.5 {
			matchResult.FinalPrice = matchResult.NetworkPrice
			matchResult.Method = "network_primary_amazon"
			matchResult.Reasons = append(matchResult.Reasons, 
				fmt.Sprintf("Amazon domain: HTTP scraping succeeded with confidence: $%.2f (confidence: %.3f)", 
					matchResult.NetworkPrice, matchResult.NetworkConfidence))
			log.Printf("🎉 AMAZON HTTP PRIMARY SUCCESS: $%.2f (confidence: %.3f)", matchResult.FinalPrice, matchResult.NetworkConfidence)
			
			return &models.PriceData{
				CurrentPrice: matchResult.FinalPrice,
				OriginalPrice: matchResult.FinalPrice,
				Currency: "USD",
				Source: "network_amazon",
				ExtractionMethod: "network_primary_amazon",
				Confidence: matchResult.NetworkConfidence,
			}, nil
		}
		
		// Case 2: Network fails, fallback to YOLO+OCR for Amazon
		if !matchResult.NetworkSuccess && matchResult.OCRSuccess {
			matchResult.FinalPrice = matchResult.OCRPrice
			matchResult.Method = "ocr_fallback_amazon"
			matchResult.Reasons = append(matchResult.Reasons, "Amazon domain: HTTP failed, using YOLO+OCR fallback")
			log.Printf("🔄 AMAZON OCR FALLBACK: HTTP failed, using YOLO+OCR: $%.2f", matchResult.FinalPrice)
			
			return &models.PriceData{
				CurrentPrice: matchResult.FinalPrice,
				OriginalPrice: matchResult.FinalPrice,
				Currency: "USD",
				Source: "yolo_ocr_amazon_fallback",
				ExtractionMethod: "ocr_fallback_amazon",
				Confidence: matchResult.OCRConfidence,
			}, nil
		}
	}
	
	// NON-AMAZON DOMAIN LOGIC: Prioritize YOLO+OCR over HTTP scraping
	// Case 1: YOLO+OCR succeeds with high confidence = Primary Success
	if matchResult.OCRSuccess && matchResult.OCRConfidence > 0.7 {
		matchResult.FinalPrice = matchResult.OCRPrice
		matchResult.Method = "yolo_ocr_primary"
		matchResult.Reasons = append(matchResult.Reasons, 
			fmt.Sprintf("YOLO+OCR succeeded with high confidence: $%.2f (confidence: %.3f)", 
				matchResult.OCRPrice, matchResult.OCRConfidence))
		log.Printf("🎉 YOLO+OCR PRIMARY SUCCESS: $%.2f (confidence: %.3f)", matchResult.FinalPrice, matchResult.OCRConfidence)
		
		return &models.PriceData{
			CurrentPrice: matchResult.FinalPrice,
			OriginalPrice: matchResult.FinalPrice,
			Currency: "USD",
			Source: "yolo_ocr",
			ExtractionMethod: "yolo_ocr_primary",
			Confidence: matchResult.OCRConfidence,
		}, nil
	}

	// Case 2: Both methods succeed and prices match = Hybrid Success
	if matchResult.NetworkSuccess && matchResult.OCRSuccess && matchResult.PricesMatch {
		if isAmazon {
			// For Amazon: prefer network result when prices match
			matchResult.FinalPrice = matchResult.NetworkPrice
			matchResult.Method = "hybrid_match_network_priority_amazon"
			matchResult.Reasons = append(matchResult.Reasons, 
				fmt.Sprintf("Amazon domain: Prices match, using HTTP result (network: $%.2f, OCR: $%.2f)", 
					matchResult.NetworkPrice, matchResult.OCRPrice))
			log.Printf("🎉 AMAZON HYBRID MATCH WITH HTTP PRIORITY: $%.2f", matchResult.FinalPrice)
			
			return &models.PriceData{
				CurrentPrice: matchResult.FinalPrice,
				OriginalPrice: matchResult.FinalPrice,
				Currency: "USD",
				Source: "network_amazon_hybrid",
				ExtractionMethod: "hybrid_match_network_priority_amazon",
				Confidence: matchResult.NetworkConfidence,
			}, nil
		} else {
			// For non-Amazon: prefer YOLO+OCR result when prices match
			matchResult.FinalPrice = matchResult.OCRPrice
			matchResult.Method = "hybrid_match_yolo_priority"
			matchResult.Reasons = append(matchResult.Reasons, 
				fmt.Sprintf("Prices match, using YOLO+OCR result (network: $%.2f, OCR: $%.2f)", 
					matchResult.NetworkPrice, matchResult.OCRPrice))
			log.Printf("🎉 HYBRID MATCH WITH YOLO PRIORITY: $%.2f", matchResult.FinalPrice)
			
			return &models.PriceData{
				CurrentPrice: matchResult.FinalPrice,
				OriginalPrice: matchResult.FinalPrice,
				Currency: "USD",
				Source: "yolo_ocr_hybrid",
				ExtractionMethod: "hybrid_match_yolo_priority",
				Confidence: matchResult.OCRConfidence,
			}, nil
		}
	}

	// Case 3: Network extraction fails > OCR success = Display OCR price
	if !matchResult.NetworkSuccess && matchResult.OCRSuccess {
		matchResult.FinalPrice = matchResult.OCRPrice
		matchResult.Method = "ocr_fallback"
		matchResult.Reasons = append(matchResult.Reasons, "Only OCR extraction succeeded")
		log.Printf("🔄 OCR FALLBACK: Network failed, using OCR price: $%.2f", matchResult.FinalPrice)
		
		return &models.PriceData{
			CurrentPrice: matchResult.FinalPrice,
			OriginalPrice: matchResult.FinalPrice,
			Currency: "USD",
			Source: "ocr",
			ExtractionMethod: "ocr_fallback",
			Confidence: matchResult.OCRConfidence,
		}, nil
	}

	// Case 4: Network extraction success > OCR fail = Display network price
	if matchResult.NetworkSuccess && !matchResult.OCRSuccess {
		matchResult.FinalPrice = matchResult.NetworkPrice
		matchResult.Method = "network_fallback"
		matchResult.Reasons = append(matchResult.Reasons, 
			fmt.Sprintf("Network extraction fallback: $%.2f", matchResult.NetworkPrice))
		log.Printf("⚠️ NETWORK FALLBACK: $%.2f", matchResult.FinalPrice)
		
		return &models.PriceData{
			CurrentPrice: matchResult.FinalPrice,
			OriginalPrice: matchResult.FinalPrice,
			Currency: "USD",
			Source: matchResult.NetworkSource,
			ExtractionMethod: "network_fallback",
			Confidence: matchResult.NetworkConfidence,
		}, nil
	}

	// Case 5: Both methods succeed but prices don't match = Intelligent selection
	if matchResult.NetworkSuccess && matchResult.OCRSuccess && !matchResult.PricesMatch {
		matchResult.Reasons = append(matchResult.Reasons, 
			fmt.Sprintf("Prices don't match, using intelligent selection. Network: $%.2f, OCR: $%.2f", 
				matchResult.NetworkPrice, matchResult.OCRPrice))

		var selectedPrice float64
		var selectedSource string
		var selectedConfidence float64
		var method string

		if isAmazon {
			// Amazon domain logic: prefer HTTP scraping unless YOLO+OCR has significantly higher confidence
			if matchResult.OCRConfidence > matchResult.NetworkConfidence + 0.15 {
				selectedPrice = matchResult.OCRPrice
				selectedSource = "yolo_ocr_amazon"
				selectedConfidence = matchResult.OCRConfidence
				method = "yolo_ocr_priority_amazon_high_confidence"
				matchResult.Reasons = append(matchResult.Reasons, 
					"Amazon domain: Preferring YOLO+OCR due to significantly higher confidence")
			} else {
				// Default to HTTP scraping for Amazon
				selectedPrice = matchResult.NetworkPrice
				selectedSource = "network_amazon"
				selectedConfidence = matchResult.NetworkConfidence
				method = "network_priority_amazon_default"
				matchResult.Reasons = append(matchResult.Reasons, 
					"Amazon domain: Defaulting to HTTP scraping for consistency")
			}
		} else {
			// Non-Amazon domain logic: prefer YOLO+OCR unless network has significantly higher confidence
			if matchResult.OCRConfidence > matchResult.NetworkConfidence + 0.05 {
				selectedPrice = matchResult.OCRPrice
				selectedSource = "yolo_ocr"
				selectedConfidence = matchResult.OCRConfidence
				method = "yolo_ocr_priority_confidence"
				matchResult.Reasons = append(matchResult.Reasons, 
					"Preferring YOLO+OCR over network due to higher confidence")
			} else if matchResult.NetworkConfidence > matchResult.OCRConfidence + 0.1 {
				selectedPrice = matchResult.NetworkPrice
				selectedSource = matchResult.NetworkSource
				selectedConfidence = matchResult.NetworkConfidence
				method = "network_priority_confidence"
				matchResult.Reasons = append(matchResult.Reasons, 
					"Preferring network over YOLO+OCR due to significantly higher confidence")
			} else {
				// Default to YOLO+OCR for non-Amazon domains
				selectedPrice = matchResult.OCRPrice
				selectedSource = "yolo_ocr"
				selectedConfidence = matchResult.OCRConfidence
				method = "yolo_ocr_priority_default"
				matchResult.Reasons = append(matchResult.Reasons, 
					"Defaulting to YOLO+OCR price for consistency")
			}
		}

		matchResult.FinalPrice = selectedPrice
		matchResult.Method = method
		log.Printf("⚠️ PRICE MISMATCH: Using %s price $%.2f (network: $%.2f, OCR: $%.2f)", 
			selectedSource, selectedPrice, matchResult.NetworkPrice, matchResult.OCRPrice)
		
		return &models.PriceData{
			CurrentPrice: matchResult.FinalPrice,
			OriginalPrice: matchResult.FinalPrice,
			Currency: "USD",
			Source: selectedSource,
			ExtractionMethod: method,
			Confidence: selectedConfidence,
		}, nil
	}

	// Case 5: Both methods fail = Extraction not possible
	matchResult.Method = "failed"
	matchResult.Error = "Both network extraction and OCR failed"
	log.Printf("❌ EXTRACTION FAILED: Both methods failed for URL: %s", url)
	
	return nil, fmt.Errorf("price extraction not possible: both network extraction and OCR failed")
}

// pricesMatch checks if two prices are within an acceptable range
func (hps *HybridPriceScraper) pricesMatch(price1, price2 float64) bool {
	if price1 <= 0 || price2 <= 0 {
		return false
	}

	// Calculate percentage difference
	diff := math.Abs(price1 - price2)
	avgPrice := (price1 + price2) / 2
	percentageDiff := (diff / avgPrice) * 100

	// Allow 5% difference for prices under $100, 3% for prices over $100
	threshold := 5.0
	if avgPrice > 100 {
		threshold = 3.0
	}

	return percentageDiff <= threshold
}

// calculateMatchConfidence calculates confidence based on price similarity
func (hps *HybridPriceScraper) calculateMatchConfidence(price1, price2 float64) float64 {
	if price1 <= 0 || price2 <= 0 {
		return 0.0
	}

	// Calculate percentage difference
	diff := math.Abs(price1 - price2)
	avgPrice := (price1 + price2) / 2
	percentageDiff := (diff / avgPrice) * 100

	// Convert to confidence score (0-1)
	// 0% difference = 100% confidence
	// 5% difference = 80% confidence
	// 10% difference = 60% confidence
	// 20% difference = 20% confidence
	confidence := 1.0 - (percentageDiff / 20.0)
	if confidence < 0.0 {
		confidence = 0.0
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// GetBrowser returns the browser instance
func (hps *HybridPriceScraper) GetBrowser() *rod.Browser {
	return hps.priceScraper.GetBrowser()
}



// scrapeWithOCRMethod extracts price using enhanced OCR (YOLO + OCR) only
func (hps *HybridPriceScraper) scrapeWithOCRMethod(url string) (*models.PriceData, error) {
	log.Printf("🎯 Extracting price with enhanced OCR (YOLO + OCR) for: %s", url)
	
	// Create a new page for this extraction
	page := hps.priceScraper.GetBrowser().MustPage(url)
	defer page.MustClose()

	// Set viewport to 1600x1200 and wait for page to load
	page.MustSetViewport(1600, 1200, 1.0, false)
	page.MustWaitLoad()
	time.Sleep(8 * time.Second) // Wait for dynamic content
	page.MustWaitStable()
	time.Sleep(3 * time.Second)
	
	// Take a screenshot for YOLO + OCR processing
	screenshotPath, err := hps.takeScreenshot(page)
	if err != nil {
		log.Printf("❌ Failed to take screenshot: %v", err)
		return nil, fmt.Errorf("failed to take screenshot: %v", err)
	}
	defer os.Remove(screenshotPath)
	
	// Use HTTP-based extractor for separate containers
	enhancedResult, err := hps.ocrExtractor.ExtractPriceWithYOLO(screenshotPath)
	if err != nil {
		log.Printf("❌ Enhanced OCR extraction failed: %v", err)
		return nil, fmt.Errorf("enhanced OCR extraction failed: %v", err)
	}
	
	if !enhancedResult.Success {
		log.Printf("❌ Enhanced OCR failed: %s", enhancedResult.Error)
		return nil, fmt.Errorf("enhanced OCR failed: %s", enhancedResult.Error)
	}
	
	log.Printf("✅ HTTP OCR extraction successful: $%.2f (YOLO: %.3f, OCR: %.3f, %d detections)", 
		enhancedResult.Price, enhancedResult.YOLOConfidence, enhancedResult.OCRConfidence, enhancedResult.TotalDetections)
	
	// Calculate overall confidence as average of YOLO and OCR confidences
	overallConfidence := (enhancedResult.YOLOConfidence + enhancedResult.OCRConfidence) / 2
	
	return &models.PriceData{
		CurrentPrice:     enhancedResult.Price,
		OriginalPrice:    enhancedResult.Price,
		Currency:         enhancedResult.Currency,
		Source:           "enhanced_ocr",
		ExtractionMethod: "yolo_ocr",
		Confidence:       overallConfidence,
	}, nil
}

// takeScreenshot takes a screenshot of the visible area for OCR processing
func (hps *HybridPriceScraper) takeScreenshot(page *rod.Page) (string, error) {
	// Create a temporary file for the screenshot
	tempFile, err := os.CreateTemp("", "screenshot_*.png")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer tempFile.Close()
	
	// Take screenshot of only the visible area (not full page)
	// This focuses on the top portion where product prices are usually located
	screenshotBytes, err := page.Screenshot(false, nil)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to take screenshot: %v", err)
	}
	
	// Write screenshot bytes to file
	err = os.WriteFile(tempFile.Name(), screenshotBytes, 0644)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to write screenshot file: %v", err)
	}
	
	return tempFile.Name(), nil
}

// ScrapePriceWithAlternativeMethod forces the alternative scraping method
func (hps *HybridPriceScraper) ScrapePriceWithAlternativeMethod(url string, urlID int) (*models.PriceData, error) {
	log.Printf("🔄 Scraping with alternative method for URL %d", urlID)
	
	// Create a new browser page
	page := hps.priceScraper.browser.MustPage()
	defer page.MustClose()
	
	// Navigate to the URL
	if err := page.Navigate(url); err != nil {
		return nil, fmt.Errorf("failed to navigate to URL: %v", err)
	}
	
	// Wait for page to load
	page.MustWaitLoad()
	time.Sleep(2 * time.Second)
	
	// Force alternative method based on domain
	isAmazon := hps.isAmazonDomain(url)
	
	if isAmazon {
		// For Amazon: force YOLO+OCR (alternative to HTTP)
		log.Printf("🛒 Amazon domain detected - forcing YOLO+OCR as alternative")
		return hps.forceYOLOOCRExtraction(page, urlID)
	} else {
		// For non-Amazon: force HTTP scraping (alternative to YOLO+OCR)
		log.Printf("🌐 Non-Amazon domain - forcing HTTP scraping as alternative")
		return hps.forceNetworkExtraction(page, urlID)
	}
}

// forceYOLOOCRExtraction forces YOLO+OCR extraction
func (hps *HybridPriceScraper) forceYOLOOCRExtraction(page *rod.Page, urlID int) (*models.PriceData, error) {
	log.Printf("🔍 Forcing YOLO+OCR extraction...")
	
	// Take screenshot for OCR processing
	screenshotPath, err := hps.takeScreenshot(page)
	if err != nil {
		return nil, fmt.Errorf("failed to take screenshot: %v", err)
	}
	defer os.Remove(screenshotPath)
	
	// Use HTTP-based extractor for separate containers
	httpResult, err := hps.ocrExtractor.ExtractPriceWithYOLO(screenshotPath)
	if err != nil {
		return nil, fmt.Errorf("YOLO+OCR extraction failed: %v", err)
	}
	
	if httpResult == nil || !httpResult.Success {
		return nil, fmt.Errorf("YOLO+OCR extraction failed: no result")
	}
	
	// Try to parse with locale parser for better accuracy
	parsedPrice, _, parseErr := hps.localeParser.ParsePrice(fmt.Sprintf("%.2f", httpResult.Price))
	if parseErr == nil {
		httpResult.Price = parsedPrice
	}
	
	return &models.PriceData{
		CurrentPrice:     httpResult.Price,
		OriginalPrice:    httpResult.Price,
		Currency:         "USD",
		Source:           "yolo_ocr_forced",
		ExtractionMethod: "yolo_ocr_alternative",
		Confidence:       httpResult.OCRConfidence,
	}, nil
}

// forceNetworkExtraction forces network extraction
func (hps *HybridPriceScraper) forceNetworkExtraction(page *rod.Page, urlID int) (*models.PriceData, error) {
	log.Printf("🌐 Forcing network extraction...")
	
	// Extract from network requests
	networkPriceData, err := hps.priceScraper.ExtractFromNetworkRequests(page)
	if err != nil {
		return nil, fmt.Errorf("network extraction failed: %v", err)
	}
	
	if networkPriceData == nil || networkPriceData.CurrentPrice <= 0 {
		return nil, fmt.Errorf("network extraction failed: no price found")
	}
	
	return &models.PriceData{
		CurrentPrice:     networkPriceData.CurrentPrice,
		OriginalPrice:    networkPriceData.CurrentPrice,
		Currency:         "USD",
		Source:           "network_forced",
		ExtractionMethod: "network_alternative",
		Confidence:       networkPriceData.Confidence,
	}, nil
}

// ScrapePriceWithDualResults performs hybrid price scraping and returns both results when they differ
func (hps *HybridPriceScraper) ScrapePriceWithDualResults(url string, urlID int) (*models.PriceCheckResponse, error) {
	log.Printf("🔍 Starting dual price check for URL %d: %s", urlID, url)
	
	// Create a new browser page
	page := hps.priceScraper.browser.MustPage()
	defer page.MustClose()
	
	// Navigate to the URL
	if err := page.Navigate(url); err != nil {
		return nil, fmt.Errorf("failed to navigate to URL: %v", err)
	}
	
	// Wait for page to load
	page.MustWaitLoad()
	time.Sleep(2 * time.Second)
	
	// Perform both extractions and compare results
	matchResult := hps.extractAndMatchPrices(page, url, urlID)
	
	// Construct the response
	response := &models.PriceCheckResponse{
		URLID: urlID,
	}
	
	// Set primary and alternative prices based on results
	if matchResult.NetworkSuccess && matchResult.OCRSuccess {
		// Both methods succeeded
		if matchResult.PricesMatch {
			// Prices match - return single result
			response.PrimaryPrice = &models.PriceData{
				CurrentPrice:     matchResult.FinalPrice,
				OriginalPrice:    matchResult.FinalPrice,
				Currency:         "USD",
				Source:           matchResult.Method,
				ExtractionMethod: matchResult.Method,
				Confidence:       matchResult.MatchConfidence,
			}
			response.NeedsFeedback = false
			response.HasAlternative = false
		} else {
			// Prices differ - return both results
			response.PrimaryPrice = &models.PriceData{
				CurrentPrice:     matchResult.NetworkPrice,
				OriginalPrice:    matchResult.NetworkPrice,
				Currency:         "USD",
				Source:           "network",
				ExtractionMethod: "network",
				Confidence:       matchResult.NetworkConfidence,
			}
			response.AlternativePrice = &models.PriceData{
				CurrentPrice:     matchResult.OCRPrice,
				OriginalPrice:    matchResult.OCRPrice,
				Currency:         "USD",
				Source:           "yolo_ocr",
				ExtractionMethod: "yolo_ocr",
				Confidence:       matchResult.OCRConfidence,
			}
			response.NeedsFeedback = true
			response.HasAlternative = true
			response.FeedbackID = fmt.Sprintf("feedback_%d_%d", urlID, time.Now().Unix())
		}
	} else if matchResult.NetworkSuccess {
		// Only network succeeded
		response.PrimaryPrice = &models.PriceData{
			CurrentPrice:     matchResult.NetworkPrice,
			OriginalPrice:    matchResult.NetworkPrice,
			Currency:         "USD",
			Source:           "network",
			ExtractionMethod: "network",
			Confidence:       matchResult.NetworkConfidence,
		}
		response.NeedsFeedback = false
		response.HasAlternative = false
	} else if matchResult.OCRSuccess {
		// Only OCR succeeded
		response.PrimaryPrice = &models.PriceData{
			CurrentPrice:     matchResult.OCRPrice,
			OriginalPrice:    matchResult.OCRPrice,
			Currency:         "USD",
			Source:           "yolo_ocr",
			ExtractionMethod: "yolo_ocr",
			Confidence:       matchResult.OCRConfidence,
		}
		response.NeedsFeedback = false
		response.HasAlternative = false
	} else {
		// Both methods failed
		return nil, fmt.Errorf("both extraction methods failed: %s", matchResult.Error)
	}
	
	// Set dual result information
	response.YOLOOCRResult = &models.PriceData{
		CurrentPrice:     matchResult.OCRPrice,
		OriginalPrice:    matchResult.OCRPrice,
		Currency:         "USD",
		Source:           "yolo_ocr",
		ExtractionMethod: "yolo_ocr",
		Confidence:       matchResult.OCRConfidence,
	}
	response.NetworkResult = &models.PriceData{
		CurrentPrice:     matchResult.NetworkPrice,
		OriginalPrice:    matchResult.NetworkPrice,
		Currency:         "USD",
		Source:           "network",
		ExtractionMethod: "network",
		Confidence:       matchResult.NetworkConfidence,
	}
	response.PricesMatch = matchResult.PricesMatch
	response.MatchConfidence = matchResult.MatchConfidence
	response.Reasons = matchResult.Reasons
	
	log.Printf("✅ Dual price check completed for URL %d - Match: %v, Confidence: %.2f", 
		urlID, matchResult.PricesMatch, matchResult.MatchConfidence)
	
	return response, nil
}
