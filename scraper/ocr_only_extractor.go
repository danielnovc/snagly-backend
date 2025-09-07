package scraper

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"net/http"
	"time"

	"distrack/models"
	"github.com/go-rod/rod"
)

// OCROnlyExtractor provides OCR-based price extraction without network algorithm
type OCROnlyExtractor struct {
	serviceURL string
	client     *http.Client
}

// NewOCROnlyExtractor creates a new OCR-only extractor
func NewOCROnlyExtractor(serviceURL string) *OCROnlyExtractor {
	return &OCROnlyExtractor{
		serviceURL: serviceURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ExtractPriceFromPage extracts price using only OCR from a loaded page
func (oe *OCROnlyExtractor) ExtractPriceFromPage(page *rod.Page) (*models.PriceData, error) {
	log.Println("🔍 Starting OCR-only price extraction...")
	
	// Wait for page to be stable
	page.MustWaitStable()
	time.Sleep(2 * time.Second)
	
	// Capture full page screenshot
	screenshot, err := page.Screenshot(true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to capture page screenshot: %v", err)
	}
	
	log.Printf("📸 Captured screenshot: %d bytes", len(screenshot))
	
	// Extract price using OCR
	priceData, err := oe.extractPriceFromImage(screenshot)
	if err != nil {
		return nil, fmt.Errorf("OCR extraction failed: %v", err)
	}
	
	log.Printf("✅ OCR extracted price: $%.2f", priceData.CurrentPrice)
	return priceData, nil
}

// ExtractPriceFromURL loads a page and extracts price using only OCR
func (oe *OCROnlyExtractor) ExtractPriceFromURL(url string, browser *rod.Browser) (*models.PriceData, error) {
	log.Printf("🌐 Loading page for OCR extraction: %s", url)
	
	// Create a new page
	page := browser.MustPage(url)
	defer page.MustClose()
	
	// Set viewport
	page.MustSetViewport(1366, 768, 1.0, false)
	
	// Wait for page to load
	page.MustWaitLoad()
	time.Sleep(3 * time.Second)
	
	// Extract price using OCR
	return oe.ExtractPriceFromPage(page)
}

// extractPriceFromImage sends image to OCR service and extracts price
func (oe *OCROnlyExtractor) extractPriceFromImage(imageData []byte) (*models.PriceData, error) {
	// Encode image data as base64
	imageDataBase64 := base64.StdEncoding.EncodeToString(imageData)
	
	// Create JSON payload
	payload := map[string]string{
		"image_data": imageDataBase64,
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}
	
	// Prepare request
	req, err := http.NewRequest("POST", oe.serviceURL+"/extract-price", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	// Send request
	resp, err := oe.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send OCR request: %v", err)
	}
	defer resp.Body.Close()
	
	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OCR response: %v", err)
	}
	
	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OCR service returned status %d: %s", resp.StatusCode, string(body))
	}
	
	// Parse response
	var ocrResponse struct {
		Success bool    `json:"success"`
		Price   float64 `json:"price"`
		Error   string  `json:"error,omitempty"`
	}
	
	if err := json.Unmarshal(body, &ocrResponse); err != nil {
		return nil, fmt.Errorf("failed to parse OCR response: %v", err)
	}
	
	if !ocrResponse.Success {
		return nil, fmt.Errorf("OCR extraction failed: %s", ocrResponse.Error)
	}
	
	// Create price data
	priceData := &models.PriceData{
		CurrentPrice:  ocrResponse.Price,
		OriginalPrice: ocrResponse.Price,
		Currency:      "USD", // Default to USD for OCR
		IsOnSale:      false,
		Source:        "OCR",
	}
	
	return priceData, nil
}

// HealthCheck checks if the OCR service is available
func (oe *OCROnlyExtractor) HealthCheck() error {
	resp, err := oe.client.Get(oe.serviceURL + "/health")
	if err != nil {
		return fmt.Errorf("OCR service health check failed: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OCR service health check returned status %d", resp.StatusCode)
	}
	
	return nil
}

// TestOCR tests the OCR service with a simple image
func (oe *OCROnlyExtractor) TestOCR() error {
	// Create a simple test image
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	
	// Encode as PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("failed to encode test image: %v", err)
	}
	
	// Test OCR extraction
	_, err := oe.extractPriceFromImage(buf.Bytes())
	return err
}

// Close cleans up resources
func (oe *OCROnlyExtractor) Close() error {
	// Nothing to close for HTTP client
	return nil
}
