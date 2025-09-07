package scraper

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// EnhancedOCRExtractor combines YOLO price detection with OCR text extraction
type EnhancedOCRExtractor struct {
	yoloServiceURL string
	ocrServiceURL  string
	client         *http.Client
}

// YOLOResponse represents the response from the YOLO service
type YOLOResponse struct {
	Success           bool                   `json:"success"`
	TotalDetections   int                    `json:"total_detections"`
	Detections        []YOLODetection        `json:"detections"`
	ProcessingTime    float64                `json:"processing_time"`
	Error             string                 `json:"error,omitempty"`
}

// YOLODetection represents a single YOLO detection
type YOLODetection struct {
	DetectionID int       `json:"detection_id"`
	BBox        []float64 `json:"bbox"`        // [x1, y1, x2, y2]
	Confidence  float64   `json:"confidence"`
	Class       int       `json:"class"`
	Center      []float64 `json:"center,omitempty"`
	Area        float64   `json:"area,omitempty"`
}

// EnhancedOCRResponse represents the response from the OCR service for enhanced extraction
type EnhancedOCRResponse struct {
	Success        bool    `json:"success"`
	Price          float64 `json:"price,omitempty"`
	Currency       string  `json:"currency,omitempty"`
	ExtractedText  string  `json:"extracted_text,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	Strategy       string  `json:"strategy,omitempty"`
	Error          string  `json:"error,omitempty"`
	StrategiesTried []string `json:"strategies_tried,omitempty"`
}

// EnhancedOCRResult represents the combined YOLO + OCR result
type EnhancedOCRResult struct {
	Success           bool
	Price             float64
	Currency          string
	TotalDetections   int
	ProcessingTime    float64
	YOLOConfidence    float64
	OCRConfidence     float64
	ExtractedText     string
	Strategy          string
	Error             string
	DetectionDetails  []DetectionDetail
}

// DetectionDetail represents details about each detection and OCR result
type DetectionDetail struct {
	BBox            []float64
	YOLOConfidence  float64
	OCRPrice        float64
	OCRText         string
	OCRConfidence   float64
	OCRStrategy     string
	AllOCRResults   []OCRAttempt // Store all OCR attempts for debugging
}

// OCRAttempt represents a single OCR attempt with its results
type OCRAttempt struct {
	Strategy        string
	ExpansionFactor float64
	Price           float64
	Text            string
	Confidence      float64
	Success         bool
	Error           string
}

// NewEnhancedOCRExtractor creates a new enhanced OCR extractor with YOLO integration
func NewEnhancedOCRExtractor(yoloServiceURL, ocrServiceURL string) *EnhancedOCRExtractor {
	if yoloServiceURL == "" {
		yoloServiceURL = "http://yolo-service:8000" // Default to Docker service
	}
	if ocrServiceURL == "" {
		ocrServiceURL = "http://ocr-service:5000" // Default to Docker service
	}

	return &EnhancedOCRExtractor{
		yoloServiceURL: yoloServiceURL,
		ocrServiceURL:  ocrServiceURL,
		client: &http.Client{
			Timeout: 180 * time.Second, // Longer timeout for comprehensive OCR processing
		},
	}
}

// ExtractPriceWithYOLO performs YOLO price detection followed by comprehensive OCR text extraction
func (e *EnhancedOCRExtractor) ExtractPriceWithYOLO(screenshotPath string) (*EnhancedOCRResult, error) {
	log.Printf("🎯 Starting comprehensive YOLO + OCR price extraction for: %s", screenshotPath)

	// Step 1: Detect price regions using YOLO
	yoloResult, err := e.detectPricesWithYOLO(screenshotPath)
	if err != nil {
		return nil, fmt.Errorf("YOLO detection failed: %v", err)
	}

	if yoloResult.TotalDetections == 0 {
		return &EnhancedOCRResult{
			Success: false,
			Error:   "No price regions detected by YOLO",
		}, nil
	}

	log.Printf("✅ YOLO detected %d price regions", yoloResult.TotalDetections)

	// Step 2: Extract text from each detected region using comprehensive OCR strategies
	detectionDetails := make([]DetectionDetail, 0)
	var bestPrice float64
	var bestConfidence float64
	var bestText string
	var bestStrategy string

	for i, detection := range yoloResult.Detections {
		log.Printf("🔍 Processing detection %d/%d (confidence: %.3f)", 
			i+1, len(yoloResult.Detections), detection.Confidence)

		// Extract text using comprehensive multi-strategy OCR
		ocrResult, err := e.extractPriceWithComprehensiveOCR(screenshotPath, detection.BBox, detection.Confidence)
		if err != nil {
			log.Printf("⚠️ OCR failed for detection %d: %v", i+1, err)
			continue
		}

		detectionDetail := DetectionDetail{
			BBox:           detection.BBox,
			YOLOConfidence: detection.Confidence,
			OCRPrice:       ocrResult.Price,
			OCRText:        ocrResult.Text,
			OCRConfidence:  ocrResult.Confidence,
			OCRStrategy:    ocrResult.Strategy,
			AllOCRResults:  ocrResult.AllAttempts,
		}
		detectionDetails = append(detectionDetails, detectionDetail)

		// Track the best result (highest combined confidence)
		combinedConfidence := (detection.Confidence + ocrResult.Confidence) / 2
		if combinedConfidence > bestConfidence && ocrResult.Price > 0 {
			bestPrice = ocrResult.Price
			bestConfidence = combinedConfidence
			bestText = ocrResult.Text
			bestStrategy = ocrResult.Strategy
		}

		log.Printf("✅ Detection %d: YOLO(%.3f) + OCR(%.3f) = $%.2f ('%s') [Strategy: %s]", 
			i+1, detection.Confidence, ocrResult.Confidence, ocrResult.Price, ocrResult.Text, ocrResult.Strategy)
		
		// Log all OCR attempts for debugging
		for j, attempt := range ocrResult.AllAttempts {
			if attempt.Success {
				log.Printf("   📝 OCR Attempt %d: %s (%.1fx) = $%.2f ('%s') [%.1f]", 
					j+1, attempt.Strategy, attempt.ExpansionFactor, attempt.Price, attempt.Text, attempt.Confidence)
			} else {
				log.Printf("   ❌ OCR Attempt %d: %s (%.1fx) = FAILED: %s", 
					j+1, attempt.Strategy, attempt.ExpansionFactor, attempt.Error)
			}
		}
	}

	if bestPrice <= 0 {
		return &EnhancedOCRResult{
			Success:         false,
			Error:           "No valid prices extracted from detected regions",
			TotalDetections: yoloResult.TotalDetections,
			ProcessingTime:  yoloResult.ProcessingTime,
			DetectionDetails: detectionDetails,
		}, nil
	}

	return &EnhancedOCRResult{
		Success:           true,
		Price:             bestPrice,
		Currency:          "USD", // Default, could be enhanced
		TotalDetections:   yoloResult.TotalDetections,
		ProcessingTime:    yoloResult.ProcessingTime,
		YOLOConfidence:    yoloResult.Detections[0].Confidence, // Use first detection confidence
		OCRConfidence:     bestConfidence,
		ExtractedText:     bestText,
		Strategy:          bestStrategy,
		DetectionDetails:  detectionDetails,
	}, nil
}

// extractPriceWithComprehensiveOCR performs multiple OCR strategies for maximum redundancy
func (e *EnhancedOCRExtractor) extractPriceWithComprehensiveOCR(imagePath string, bbox []float64, yoloConfidence float64) (*OCRResult, error) {
	allAttempts := make([]OCRAttempt, 0)
	
	// Strategy 1: Multiple expansion factors
	expansionFactors := e.getExpansionFactors(yoloConfidence)
	for _, expansion := range expansionFactors {
		attempt := e.extractWithExpansion(imagePath, bbox, expansion)
		allAttempts = append(allAttempts, attempt)
		
		// If we get a good result, we can stop early
		if attempt.Success && attempt.Confidence > 0.8 && attempt.Price > 0 {
			log.Printf("🎯 Early success with expansion %.1fx: $%.2f (confidence: %.1f)", 
				expansion, attempt.Price, attempt.Confidence)
			break
		}
	}

	// Strategy 2: Multiple OCR preprocessing strategies
	preprocessingStrategies := []string{"default", "enhanced", "aggressive", "conservative"}
	for _, strategy := range preprocessingStrategies {
		attempt := e.extractWithPreprocessing(imagePath, bbox, strategy)
		allAttempts = append(allAttempts, attempt)
	}

	// Strategy 3: Full image fallback (if no bbox results)
	if !e.hasSuccessfulBboxResults(allAttempts) {
		attempt := e.extractFromFullImage(imagePath)
		allAttempts = append(allAttempts, attempt)
	}

	// Select the best result
	bestResult := e.selectBestOCRResult(allAttempts)
	bestResult.AllAttempts = allAttempts

	return bestResult, nil
}

// OCRResult represents the result of comprehensive OCR extraction
type OCRResult struct {
	Price       float64
	Text        string
	Confidence  float64
	Strategy    string
	AllAttempts []OCRAttempt
}

// getExpansionFactors returns expansion factors based on YOLO confidence
func (e *EnhancedOCRExtractor) getExpansionFactors(yoloConfidence float64) []float64 {
	if yoloConfidence > 0.8 {
		return []float64{1.0, 1.5, 2.0} // High confidence = smaller expansions
	} else if yoloConfidence > 0.6 {
		return []float64{1.5, 2.0, 2.5} // Medium confidence = standard expansions
	} else {
		return []float64{2.0, 2.5, 3.0, 3.5} // Low confidence = larger expansions
	}
}

// extractWithExpansion extracts price with a specific expansion factor
func (e *EnhancedOCRExtractor) extractWithExpansion(imagePath string, bbox []float64, expansionFactor float64) OCRAttempt {
	expandedBbox := e.expandBoundingBox(bbox, expansionFactor)
	
	// Read image file
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("expansion_%.1fx", expansionFactor),
			ExpansionFactor: expansionFactor,
			Success:         false,
			Error:           fmt.Sprintf("failed to read image: %v", err),
		}
	}

	// Encode image to base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// Prepare request payload
	payload := map[string]interface{}{
		"image_data": base64Image,
		"bbox":       expandedBbox,
		"strategy":   "default",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("expansion_%.1fx", expansionFactor),
			ExpansionFactor: expansionFactor,
			Success:         false,
			Error:           fmt.Sprintf("failed to marshal request: %v", err),
		}
	}

	// Make request to OCR service
	resp, err := e.client.Post(
		e.ocrServiceURL+"/extract-price",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("expansion_%.1fx", expansionFactor),
			ExpansionFactor: expansionFactor,
			Success:         false,
			Error:           fmt.Sprintf("OCR service error: %v", err),
		}
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("expansion_%.1fx", expansionFactor),
			ExpansionFactor: expansionFactor,
			Success:         false,
			Error:           fmt.Sprintf("failed to read response: %v", err),
		}
	}

	// Parse response
	var ocrResp EnhancedOCRResponse
	if err := json.Unmarshal(body, &ocrResp); err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("expansion_%.1fx", expansionFactor),
			ExpansionFactor: expansionFactor,
			Success:         false,
			Error:           fmt.Sprintf("failed to parse response: %v", err),
		}
	}

	if !ocrResp.Success {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("expansion_%.1fx", expansionFactor),
			ExpansionFactor: expansionFactor,
			Success:         false,
			Error:           ocrResp.Error,
		}
	}

	return OCRAttempt{
		Strategy:        fmt.Sprintf("expansion_%.1fx", expansionFactor),
		ExpansionFactor: expansionFactor,
		Price:           ocrResp.Price,
		Text:            ocrResp.ExtractedText,
		Confidence:      ocrResp.Confidence,
		Success:         true,
	}
}

// extractWithPreprocessing extracts price with different preprocessing strategies
func (e *EnhancedOCRExtractor) extractWithPreprocessing(imagePath string, bbox []float64, strategy string) OCRAttempt {
	// Use optimal expansion factor for preprocessing strategies
	optimalExpansion := 2.0
	expandedBbox := e.expandBoundingBox(bbox, optimalExpansion)
	
	// Read image file
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("preprocessing_%s", strategy),
			ExpansionFactor: optimalExpansion,
			Success:         false,
			Error:           fmt.Sprintf("failed to read image: %v", err),
		}
	}

	// Encode image to base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// Prepare request payload
	payload := map[string]interface{}{
		"image_data": base64Image,
		"bbox":       expandedBbox,
		"strategy":   strategy,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("preprocessing_%s", strategy),
			ExpansionFactor: optimalExpansion,
			Success:         false,
			Error:           fmt.Sprintf("failed to marshal request: %v", err),
		}
	}

	// Make request to OCR service
	resp, err := e.client.Post(
		e.ocrServiceURL+"/extract-price",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("preprocessing_%s", strategy),
			ExpansionFactor: optimalExpansion,
			Success:         false,
			Error:           fmt.Sprintf("OCR service error: %v", err),
		}
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("preprocessing_%s", strategy),
			ExpansionFactor: optimalExpansion,
			Success:         false,
			Error:           fmt.Sprintf("failed to read response: %v", err),
		}
	}

	// Parse response
	var ocrResp EnhancedOCRResponse
	if err := json.Unmarshal(body, &ocrResp); err != nil {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("preprocessing_%s", strategy),
			ExpansionFactor: optimalExpansion,
			Success:         false,
			Error:           fmt.Sprintf("failed to parse response: %v", err),
		}
	}

	if !ocrResp.Success {
		return OCRAttempt{
			Strategy:        fmt.Sprintf("preprocessing_%s", strategy),
			ExpansionFactor: optimalExpansion,
			Success:         false,
			Error:           ocrResp.Error,
		}
	}

	return OCRAttempt{
		Strategy:        fmt.Sprintf("preprocessing_%s", strategy),
		ExpansionFactor: optimalExpansion,
		Price:           ocrResp.Price,
		Text:            ocrResp.ExtractedText,
		Confidence:      ocrResp.Confidence,
		Success:         true,
	}
}

// extractFromFullImage extracts price from the full image as fallback
func (e *EnhancedOCRExtractor) extractFromFullImage(imagePath string) OCRAttempt {
	// Read image file
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return OCRAttempt{
			Strategy:        "full_image",
			ExpansionFactor: 0.0,
			Success:         false,
			Error:           fmt.Sprintf("failed to read image: %v", err),
		}
	}

	// Encode image to base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// Prepare request payload (no bbox = full image)
	payload := map[string]interface{}{
		"image_data": base64Image,
		"strategy":   "full_image",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return OCRAttempt{
			Strategy:        "full_image",
			ExpansionFactor: 0.0,
			Success:         false,
			Error:           fmt.Sprintf("failed to marshal request: %v", err),
		}
	}

	// Make request to OCR service
	resp, err := e.client.Post(
		e.ocrServiceURL+"/extract-price",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return OCRAttempt{
			Strategy:        "full_image",
			ExpansionFactor: 0.0,
			Success:         false,
			Error:           fmt.Sprintf("OCR service error: %v", err),
		}
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return OCRAttempt{
			Strategy:        "full_image",
			ExpansionFactor: 0.0,
			Success:         false,
			Error:           fmt.Sprintf("failed to read response: %v", err),
		}
	}

	// Parse response
	var ocrResp EnhancedOCRResponse
	if err := json.Unmarshal(body, &ocrResp); err != nil {
		return OCRAttempt{
			Strategy:        "full_image",
			ExpansionFactor: 0.0,
			Success:         false,
			Error:           fmt.Sprintf("failed to parse response: %v", err),
		}
	}

	if !ocrResp.Success {
		return OCRAttempt{
			Strategy:        "full_image",
			ExpansionFactor: 0.0,
			Success:         false,
			Error:           ocrResp.Error,
		}
	}

	return OCRAttempt{
		Strategy:        "full_image",
		ExpansionFactor: 0.0,
		Price:           ocrResp.Price,
		Text:            ocrResp.ExtractedText,
		Confidence:      ocrResp.Confidence,
		Success:         true,
	}
}

// hasSuccessfulBboxResults checks if any bbox-based OCR attempts were successful
func (e *EnhancedOCRExtractor) hasSuccessfulBboxResults(attempts []OCRAttempt) bool {
	for _, attempt := range attempts {
		if attempt.Success && attempt.Price > 0 && !strings.Contains(attempt.Strategy, "full_image") {
			return true
		}
	}
	return false
}

// selectBestOCRResult selects the best OCR result from all attempts
func (e *EnhancedOCRExtractor) selectBestOCRResult(attempts []OCRAttempt) *OCRResult {
	var bestAttempt OCRAttempt
	bestScore := 0.0

	for _, attempt := range attempts {
		if !attempt.Success || attempt.Price <= 0 {
			continue
		}

		// Calculate score based on confidence and strategy preference
		score := attempt.Confidence
		
		// Prefer bbox-based results over full image
		if !strings.Contains(attempt.Strategy, "full_image") {
			score *= 1.2
		}
		
		// Prefer optimal expansion factors
		if attempt.ExpansionFactor >= 1.5 && attempt.ExpansionFactor <= 2.5 {
			score *= 1.1
		}

		if score > bestScore {
			bestScore = score
			bestAttempt = attempt
		}
	}

	if bestScore == 0 {
		return &OCRResult{
			Price:       0,
			Text:        "",
			Confidence:  0,
			Strategy:    "no_successful_attempts",
			AllAttempts: attempts,
		}
	}

	return &OCRResult{
		Price:       bestAttempt.Price,
		Text:        bestAttempt.Text,
		Confidence:  bestAttempt.Confidence,
		Strategy:    bestAttempt.Strategy,
		AllAttempts: attempts,
	}
}

// detectPricesWithYOLO calls the YOLO service to detect price regions
func (e *EnhancedOCRExtractor) detectPricesWithYOLO(imagePath string) (*YOLOResponse, error) {
	// Read image file
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image file: %v", err)
	}

	// Encode image to base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// Prepare request payload
	payload := map[string]interface{}{
		"image_data": base64Image,
		"confidence_threshold": 0.5,
		"return_crops": false,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make request to YOLO service
	resp, err := e.client.Post(
		e.yoloServiceURL+"/detect-prices",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call YOLO service: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	// Parse response
	var yoloResp YOLOResponse
	if err := json.Unmarshal(body, &yoloResp); err != nil {
		return nil, fmt.Errorf("failed to parse YOLO response: %v", err)
	}

	if !yoloResp.Success {
		return nil, fmt.Errorf("YOLO service error: %s", yoloResp.Error)
	}

	return &yoloResp, nil
}

// expandBoundingBox expands a bounding box by a given factor for better OCR accuracy
func (e *EnhancedOCRExtractor) expandBoundingBox(bbox []float64, expansionFactor float64) []float64 {
	if len(bbox) != 4 {
		return bbox // Return original if invalid
	}

	x1, y1, x2, y2 := bbox[0], bbox[1], bbox[2], bbox[3]

	// Calculate center and dimensions
	centerX := (x1 + x2) / 2
	centerY := (y1 + y2) / 2
	width := x2 - x1
	height := y2 - y1

	// Expand dimensions
	newWidth := width * expansionFactor
	newHeight := height * expansionFactor

	// Calculate new coordinates
	newX1 := centerX - newWidth/2
	newY1 := centerY - newHeight/2
	newX2 := centerX + newWidth/2
	newY2 := centerY + newHeight/2

	return []float64{newX1, newY1, newX2, newY2}
}

// HealthCheck checks if both YOLO and OCR services are available
func (e *EnhancedOCRExtractor) HealthCheck() error {
	// Check YOLO service
	yoloResp, err := e.client.Get(e.yoloServiceURL + "/health")
	if err != nil {
		return fmt.Errorf("YOLO service health check failed: %v", err)
	}
	yoloResp.Body.Close()

	// Check OCR service
	ocrResp, err := e.client.Get(e.ocrServiceURL + "/health")
	if err != nil {
		return fmt.Errorf("OCR service health check failed: %v", err)
	}
	ocrResp.Body.Close()

	log.Printf("✅ Enhanced OCR extractor health check passed")
	return nil
}
