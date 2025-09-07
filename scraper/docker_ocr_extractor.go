package scraper

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"distrack/models"
	"github.com/go-rod/rod"
)

// DockerOCRExtractor handles price extraction using Docker OCR service
type DockerOCRExtractor struct {
	serviceURL string
	client     *http.Client
}

// OCRResponse represents the response from the OCR service
type OCRResponse struct {
	Success        bool    `json:"success"`
	Price          float64 `json:"price,omitempty"`
	Currency       string  `json:"currency,omitempty"`
	ExtractedText  string  `json:"extracted_text,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	Strategy       string  `json:"strategy,omitempty"`
	Error          string  `json:"error,omitempty"`
	StrategiesTried []string `json:"strategies_tried,omitempty"`
}

// NewDockerOCRExtractor creates a new Docker OCR price extractor
func NewDockerOCRExtractor(serviceURL string) *DockerOCRExtractor {
	if serviceURL == "" {
		serviceURL = "http://ocr-service:5000" // Default to Docker service
	}

	return &DockerOCRExtractor{
		serviceURL: serviceURL,
		client: &http.Client{
			Timeout: 90 * time.Second, // Longer timeout for enhanced OCR processing
		},
	}
}

// ExtractPriceFromImage extracts price from an image using Docker OCR service
func (d *DockerOCRExtractor) ExtractPriceFromImage(imgBytes []byte) (float64, error) {
	// Encode image to base64
	base64Image := base64.StdEncoding.EncodeToString(imgBytes)

	// Prepare request payload
	payload := map[string]string{
		"image_data": base64Image,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make request to OCR service
	resp, err := d.client.Post(
		d.serviceURL+"/extract-price",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to call OCR service: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %v", err)
	}

	// Parse response
	var ocrResp OCRResponse
	if err := json.Unmarshal(body, &ocrResp); err != nil {
		return 0, fmt.Errorf("failed to parse OCR response: %v", err)
	}

	if !ocrResp.Success {
		strategies := strings.Join(ocrResp.StrategiesTried, ", ")
		return 0, fmt.Errorf("OCR service error: %s (strategies tried: %s, extracted text: '%s')", 
			ocrResp.Error, strategies, ocrResp.ExtractedText)
	}

	log.Printf("Docker OCR found price: $%.2f (confidence: %.0f, strategy: %s) from text: '%s'", 
		ocrResp.Price, ocrResp.Confidence, ocrResp.Strategy, ocrResp.ExtractedText)
	return ocrResp.Price, nil
}

// CapturePriceAreaScreenshot captures a screenshot of the price area with enhanced detection
func (d *DockerOCRExtractor) CapturePriceAreaScreenshot(page *rod.Page, selectors []string) ([]byte, error) {
	// Enhanced price selectors to try
	priceSelectors := []string{
		// Data attributes (most reliable)
		"[data-testid*='price']", "[data-testid*='Price']",
		"[data-price]", "[data-current-price]", "[data-product-price]",
		"[data-testid*='cost']", "[data-testid*='Cost']",
		
		// Class-based selectors
		"[class*='price']", "[class*='Price']", "[class*='cost']", "[class*='Cost']",
		".price", ".product-price", ".current-price", ".sale-price",
		".price__current", ".price__value", ".product-price__current",
		".price-current", ".price-value", ".product-price-value",
		
		// ID-based selectors
		"[id*='price']", "[id*='Price']", "[id*='cost']", "[id*='Cost']",
		
		// Money/currency specific selectors
		".money", ".price-money", ".product-money", ".price__money",
		".price-amount", ".price-currency", ".product-price-amount",
		".amount", ".currency", ".cost-amount", ".price-cost",
		
		// Product-specific price containers
		".product__price", ".product-price", ".item__price", ".item-price",
		".product-single__price", ".product__current-price",
	}

	// Combine with provided selectors
	allSelectors := append(selectors, priceSelectors...)

	// Try each selector
	for _, selector := range allSelectors {
		elements, err := page.Elements(selector)
		if err != nil || len(elements) == 0 {
			continue
		}

		log.Printf("Found %d elements with selector: %s", len(elements), selector)

		// Try each element
		for i, element := range elements {
			// Check if element is visible
			visible, err := element.Visible()
			if err != nil || !visible {
				continue
			}

			// Get element text to verify it might contain a price
			text, err := element.Text()
			if err != nil {
				continue
			}

			// Check if text contains price-like patterns
			if d.containsPricePattern(text) {
				log.Printf("Element %d with selector %s contains price pattern: '%s'", i+1, selector, text)
				
										// Capture screenshot of this element
			img, err := element.Screenshot("png", 100)
			if err != nil {
				log.Printf("Failed to capture screenshot for selector %s element %d: %v", selector, i+1, err)
				continue
			}

			log.Printf("Successfully captured price area screenshot using selector: %s (element %d)", selector, i+1)
			return img, nil
			}
		}
	}

	// If no specific price area found, try to capture larger regions
	return d.captureFallbackRegions(page)
}

// captureFallbackRegions captures larger regions when specific price elements aren't found
func (d *DockerOCRExtractor) captureFallbackRegions(page *rod.Page) ([]byte, error) {
	// Try to capture main content area
	fallbackSelectors := []string{
		"main", ".main", ".content", ".product", ".product-details",
		".product-info", ".product-summary", ".product-main",
		"article", ".article", ".product-article",
	}

	for _, selector := range fallbackSelectors {
		elements, err := page.Elements(selector)
		if err != nil || len(elements) == 0 {
			continue
		}

		// Use the first visible element
		for _, element := range elements {
			visible, err := element.Visible()
			if err != nil || !visible {
				continue
			}

			// Get text to check if it contains price information
			text, err := element.Text()
			if err != nil {
				continue
			}

			// Check if the region contains price-like content
			if d.containsPricePattern(text) {
				img, err := element.Screenshot("png", 100)
				if err != nil {
					log.Printf("Failed to capture fallback region screenshot for selector %s: %v", selector, err)
					continue
				}

				log.Printf("Captured fallback region screenshot using selector: %s", selector)
				return img, nil
			}
		}
	}

	return nil, fmt.Errorf("no price area or fallback region found to capture")
}

// containsPricePattern checks if text contains price-like patterns with enhanced detection
func (d *DockerOCRExtractor) containsPricePattern(text string) bool {
	if text == "" {
		return false
	}

	// Price indicators
	priceIndicators := []string{
		"$", "€", "£", "¥", "₹", "₽", "₩", "₪", "₦", "₨", "₫", "₴", "₸", "₺", "₼", "₾", "₿",
		"price", "cost", "amount", "total", "sum", "value",
		"buy", "sale", "discount", "save", "offer", "deal",
	}

	textLower := strings.ToLower(text)
	for _, indicator := range priceIndicators {
		if strings.Contains(textLower, strings.ToLower(indicator)) {
			return true
		}
	}

	// Check for number patterns with currency context
	// Look for numbers that might be prices (with decimal places, reasonable ranges)
	numberPattern := regexp.MustCompile(`\d+(?:,\d{3})*(?:\.\d{2})?`)
	matches := numberPattern.FindAllString(text, -1)
	
	for _, match := range matches {
		// Remove commas and convert to float
		cleanMatch := strings.ReplaceAll(match, ",", "")
		if num, err := parseFloat(cleanMatch); err == nil {
			// Check if it's in a reasonable price range
			if num >= 0.01 && num <= 100000 {
				return true
			}
		}
	}

	return false
}

// ExtractPriceWithOCR attempts to extract price using Docker OCR with enhanced strategies
func (d *DockerOCRExtractor) ExtractPriceWithOCR(page *rod.Page, selectors []string) (float64, error) {
	// Strategy 1: Try to capture screenshot of specific price area
	log.Println("OCR Strategy 1: Capturing specific price area...")
	imgBytes, err := d.CapturePriceAreaScreenshot(page, selectors)
	if err == nil {
		price, err := d.ExtractPriceFromImage(imgBytes)
		if err == nil && price > 0 {
			log.Printf("OCR Strategy 1 successful: $%.2f", price)
			return price, nil
		}
		log.Printf("OCR Strategy 1 failed: %v", err)
	}

	// Strategy 2: Capture larger page regions
	log.Println("OCR Strategy 2: Capturing larger page regions...")
	imgBytes, err = d.captureFallbackRegions(page)
	if err == nil {
		price, err := d.ExtractPriceFromImage(imgBytes)
		if err == nil && price > 0 {
			log.Printf("OCR Strategy 2 successful: $%.2f", price)
			return price, nil
		}
		log.Printf("OCR Strategy 2 failed: %v", err)
	}

	// Strategy 3: Capture full page screenshot as last resort
	log.Println("OCR Strategy 3: Capturing full page screenshot...")
	fullScreenshot, err := page.Screenshot(true, nil)
	if err == nil {
		price, err := d.ExtractPriceFromImage(fullScreenshot)
		if err == nil && price > 0 {
			log.Printf("OCR Strategy 3 successful: $%.2f", price)
			return price, nil
		}
		log.Printf("OCR Strategy 3 failed: %v", err)
	}

	return 0, fmt.Errorf("all OCR strategies failed")
}

// ExtractPriceWithOCRRetry attempts to extract price with multiple retry strategies
func (d *DockerOCRExtractor) ExtractPriceWithOCRRetry(page *rod.Page, selectors []string, maxRetries int) (float64, error) {
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("OCR extraction attempt %d/%d", attempt, maxRetries)
		
		// Wait a bit between attempts
		if attempt > 1 {
			time.Sleep(2 * time.Duration(attempt) * time.Second)
		}

		price, err := d.ExtractPriceWithOCR(page, selectors)
		if err == nil && price > 0 {
			log.Printf("OCR extraction successful on attempt %d: $%.2f", attempt, price)
			return price, nil
		}

		lastErr = err
		log.Printf("OCR extraction attempt %d failed: %v", attempt, err)
	}

	return 0, fmt.Errorf("OCR extraction failed after %d attempts: %v", maxRetries, lastErr)
}

// HealthCheck checks if the Docker OCR service is healthy
func (d *DockerOCRExtractor) HealthCheck() error {
	resp, err := d.client.Get(d.serviceURL + "/health")
	if err != nil {
		return fmt.Errorf("OCR service health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("OCR service unhealthy (status: %d)", resp.StatusCode)
	}

	// Parse response to check version
	var healthResp struct {
		Status  string `json:"status"`
		Service string `json:"service"`
		Version string `json:"version"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read health response: %v", err)
	}

	if err := json.Unmarshal(body, &healthResp); err != nil {
		return fmt.Errorf("failed to parse health response: %v", err)
	}

	log.Printf("OCR service health check passed: %s %s", healthResp.Service, healthResp.Version)
	return nil
}

// ExtractPrice extracts price from a URL using OCR and returns PriceData
func (d *DockerOCRExtractor) ExtractPrice(url string) (*models.PriceData, error) {
	// Create a browser instance for this extraction
	browser := rod.New().MustConnect()
	defer browser.MustClose()

	// Navigate to the page
	page := browser.MustPage(url)
	defer page.MustClose()

	// Wait for page to load
	page.MustWaitStable()

	// Try to extract price using OCR
	price, err := d.ExtractPriceWithOCR(page, []string{})
	if err != nil {
		return nil, fmt.Errorf("OCR extraction failed: %v", err)
	}

	// Return PriceData structure
	return &models.PriceData{
		CurrentPrice:     price,
		OriginalPrice:    price, // OCR can't easily distinguish original vs current
		Currency:         "USD", // Default currency
		DiscountPercentage: 0.0, // OCR can't easily detect discounts
		IsOnSale:         false, // OCR can't easily detect sales
		Source:           "ocr",
		ExtractionMethod: "docker_ocr",
		Confidence:       0.7, // OCR typically has lower confidence
	}, nil
}

// TestOCR tests the OCR service with a simple request
func (d *DockerOCRExtractor) TestOCR() error {
	resp, err := d.client.Get(d.serviceURL + "/test")
	if err != nil {
		return fmt.Errorf("OCR service test failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read test response: %v", err)
	}

	log.Printf("OCR service test response: %s", string(body))
	return nil
}

// Close closes the Docker OCR extractor (no cleanup needed for HTTP client)
func (d *DockerOCRExtractor) Close() error {
	// No cleanup needed for HTTP client
	return nil
}

// Helper function to parse float
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
