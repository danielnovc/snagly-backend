package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TrackedURL represents a URL being monitored for price changes
type TrackedURL struct {
	ID          int             `json:"id" db:"id"`
	URL         string          `json:"url" db:"url"`
	Name        string          `json:"name" db:"name"`
	CurrentPrice sql.NullFloat64 `json:"current_price" db:"current_price"`
	OriginalPrice sql.NullFloat64 `json:"original_price" db:"original_price"`
	Currency    string          `json:"currency" db:"currency"`
	LastChecked *time.Time      `json:"last_checked" db:"last_checked"`
	LastFailedAt *time.Time     `json:"last_failed_at" db:"last_failed_at"`
	RetryCount  int             `json:"retry_count" db:"retry_count"`
	NextRetryAt *time.Time      `json:"next_retry_at" db:"next_retry_at"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
	IsActive    bool            `json:"is_active" db:"is_active"`
}

// GetCurrentPrice returns the current price as float64, or 0 if NULL
func (t *TrackedURL) GetCurrentPrice() float64 {
	if t.CurrentPrice.Valid {
		return t.CurrentPrice.Float64
	}
	return 0.0
}

// GetOriginalPrice returns the original price as float64, or 0 if NULL
func (t *TrackedURL) GetOriginalPrice() float64 {
	if t.OriginalPrice.Valid {
		return t.OriginalPrice.Float64
	}
	return 0.0
}

// HasPrice returns true if the URL has a current price
func (t *TrackedURL) HasPrice() bool {
	return t.CurrentPrice.Valid
}

// CanRetry returns true if the URL can be retried now
func (t *TrackedURL) CanRetry() bool {
	if t.NextRetryAt == nil {
		return true
	}
	return time.Now().After(*t.NextRetryAt)
}

// ShouldRetry returns true if the URL should be retried (has failed and can retry)
func (t *TrackedURL) ShouldRetry() bool {
	return t.LastFailedAt != nil && t.CanRetry() && t.RetryCount < 5
}

// GetRetryDelay returns the delay for the next retry based on retry count
func (t *TrackedURL) GetRetryDelay() time.Duration {
	switch t.RetryCount {
	case 0:
		return 10 * time.Minute
	case 1:
		return 30 * time.Minute
	case 2:
		return 1 * time.Hour
	case 3:
		return 3 * time.Hour
	case 4:
		return 6 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// IsPriceChangeRealistic checks if a price change is realistic and should trigger notifications
func (t *TrackedURL) IsPriceChangeRealistic(newPrice float64) bool {
	if !t.HasPrice() {
		return true // First price, always realistic
	}
	
	oldPrice := t.GetCurrentPrice()
	if oldPrice <= 0 {
		return true // No previous price to compare
	}
	
	// Calculate percentage change
	priceChange := ((newPrice - oldPrice) / oldPrice) * 100
	
	// Define realistic price change thresholds based on product type
	thresholds := t.getPriceChangeThresholds()
	
	// Check if the change is within realistic bounds
	return priceChange >= thresholds.MinDrop && priceChange <= thresholds.MaxIncrease
}

// PriceChangeThresholds defines realistic price change limits
type PriceChangeThresholds struct {
	MinDrop     float64 // Maximum realistic price drop (negative percentage)
	MaxIncrease float64 // Maximum realistic price increase (positive percentage)
}

// getPriceChangeThresholds returns realistic price change thresholds based on product type
func (t *TrackedURL) getPriceChangeThresholds() PriceChangeThresholds {
	productName := strings.ToLower(t.Name)
	
	// Luxury items (bags, designer items) - more volatile but still realistic
	if t.isLuxuryProduct(productName) {
		return PriceChangeThresholds{
			MinDrop:     -80.0, // Up to 80% drop (sales, clearance)
			MaxIncrease: 200.0, // Up to 200% increase (limited editions, demand)
		}
	}
	
	// Electronics - moderate volatility
	if t.isElectronicsProduct(productName) {
		return PriceChangeThresholds{
			MinDrop:     -60.0, // Up to 60% drop (clearance, new models)
			MaxIncrease: 100.0, // Up to 100% increase (shortage, demand)
		}
	}
	
	// Clothing and fashion - moderate volatility
	if t.isFashionProduct(productName) {
		return PriceChangeThresholds{
			MinDrop:     -70.0, // Up to 70% drop (end of season, sales)
			MaxIncrease: 150.0, // Up to 150% increase (limited editions)
		}
	}
	
	// Default thresholds for other products
	return PriceChangeThresholds{
		MinDrop:     -50.0, // Up to 50% drop
		MaxIncrease: 100.0, // Up to 100% increase
	}
}

// isLuxuryProduct checks if the product is a luxury item
func (t *TrackedURL) isLuxuryProduct(productName string) bool {
	luxuryKeywords := []string{
		"chloe", "louis vuitton", "gucci", "hermes", "chanel", "prada", "fendi", "balenciaga",
		"dior", "celine", "givenchy", "saint laurent", "valentino", "bottega", "moynat",
		"goyard", "delvaux", "mansur gavriel", "strathberry", "aspinal", "mulberry",
		"leather", "premium", "luxury", "designer", "handbag", "bag", "purse", "tote",
		"wallet", "clutch", "crossbody", "shoulder bag", "backpack", "duffle",
	}
	
	for _, keyword := range luxuryKeywords {
		if strings.Contains(productName, keyword) {
			return true
		}
	}
	return false
}

// isElectronicsProduct checks if the product is electronics
func (t *TrackedURL) isElectronicsProduct(productName string) bool {
	electronicsKeywords := []string{
		"phone", "smartphone", "laptop", "computer", "tablet", "ipad", "iphone", "samsung",
		"macbook", "dell", "hp", "lenovo", "asus", "acer", "msi", "gaming", "console",
		"playstation", "xbox", "nintendo", "switch", "headphones", "earbuds", "airpods",
		"camera", "canon", "nikon", "sony", "gopro", "drone", "tv", "television", "monitor",
		"keyboard", "mouse", "speaker", "bluetooth", "wireless", "charger", "cable",
	}
	
	for _, keyword := range electronicsKeywords {
		if strings.Contains(productName, keyword) {
			return true
		}
	}
	return false
}

// isFashionProduct checks if the product is fashion/clothing
func (t *TrackedURL) isFashionProduct(productName string) bool {
	fashionKeywords := []string{
		"shirt", "t-shirt", "pants", "jeans", "dress", "skirt", "jacket", "coat", "sweater",
		"sweatshirt", "hoodie", "blazer", "suit", "tie", "scarf", "hat", "cap", "shoes",
		"sneakers", "boots", "sandals", "heels", "flats", "jewelry", "watch", "ring",
		"necklace", "bracelet", "earrings", "sunglasses", "belt", "wallet", "socks",
		"underwear", "lingerie", "swimwear", "activewear", "athletic", "sportswear",
	}
	
	for _, keyword := range fashionKeywords {
		if strings.Contains(productName, keyword) {
			return true
		}
	}
	return false
}

// GetPriceChangeReason returns a human-readable reason for price change validation
func (t *TrackedURL) GetPriceChangeReason(newPrice float64) string {
	if !t.HasPrice() {
		return "First price check"
	}
	
	oldPrice := t.GetCurrentPrice()
	if oldPrice <= 0 {
		return "No previous price to compare"
	}
	
	priceChange := ((newPrice - oldPrice) / oldPrice) * 100
	thresholds := t.getPriceChangeThresholds()
	
	if priceChange < thresholds.MinDrop {
		return fmt.Sprintf("Price drop too extreme (%.1f%%). Possible scraping error.", priceChange)
	}
	
	if priceChange > thresholds.MaxIncrease {
		return fmt.Sprintf("Price increase too extreme (%.1f%%). Possible scraping error.", priceChange)
	}
	
	if priceChange < 0 {
		return fmt.Sprintf("Price dropped by %.1f%%", -priceChange)
	} else if priceChange > 0 {
		return fmt.Sprintf("Price increased by %.1f%%", priceChange)
	}
	
	return "No price change"
}

// MarshalJSON implements custom JSON marshaling for TrackedURL
func (t *TrackedURL) MarshalJSON() ([]byte, error) {
	type Alias TrackedURL
	return json.Marshal(&struct {
		*Alias
		CurrentPrice  *float64  `json:"current_price"`
		OriginalPrice *float64  `json:"original_price"`
	}{
		Alias:         (*Alias)(t),
		CurrentPrice:  t.getCurrentPricePtr(),
		OriginalPrice: t.getOriginalPricePtr(),
	})
}

// getCurrentPricePtr returns a pointer to the current price, or nil if NULL
func (t *TrackedURL) getCurrentPricePtr() *float64 {
	if t.CurrentPrice.Valid {
		price := t.CurrentPrice.Float64
		return &price
	}
	return nil
}

// getOriginalPricePtr returns a pointer to the original price, or nil if NULL
func (t *TrackedURL) getOriginalPricePtr() *float64 {
	if t.OriginalPrice.Valid {
		price := t.OriginalPrice.Float64
		return &price
	}
	return nil
}

// PriceHistory represents a price point in time
type PriceHistory struct {
	ID          int       `json:"id" db:"id"`
	URLID       int       `json:"url_id" db:"url_id"`
	Price       float64   `json:"price" db:"price"`
	Currency    string    `json:"currency" db:"currency"`
	DiscountPercentage float64 `json:"discount_percentage" db:"discount_percentage"`
	OriginalPrice float64 `json:"original_price" db:"original_price"`
	CheckedAt   time.Time `json:"checked_at" db:"checked_at"`
}

// PriceAlert represents a price drop alert
type PriceAlert struct {
	ID          int       `json:"id" db:"id"`
	URLID       int       `json:"url_id" db:"url_id"`
	TargetPrice float64   `json:"target_price" db:"target_price"`
	AlertType   string    `json:"alert_type" db:"alert_type"` // "price_drop", "percentage_drop"
	Percentage  float64   `json:"percentage" db:"percentage"` // for percentage-based alerts
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	TriggeredAt *time.Time `json:"triggered_at" db:"triggered_at"`
}

// AddURLRequest represents the request to add a new URL to track
type AddURLRequest struct {
	URL  string `json:"url" validate:"required,url"`
	Name string `json:"name" validate:"required"`
}

// SetAlertRequest represents the request to set a price alert
type SetAlertRequest struct {
	TargetPrice float64 `json:"target_price"`
	AlertType   string  `json:"alert_type" validate:"required,oneof=price_drop percentage_drop"`
	Percentage  float64 `json:"percentage"` // for percentage-based alerts
}

// PriceData represents extracted price information
type PriceData struct {
	CurrentPrice       float64 `json:"current_price"`
	OriginalPrice      float64 `json:"original_price"`
	Currency           string  `json:"currency"`
	DiscountPercentage float64 `json:"discount_percentage"`
	IsOnSale           bool    `json:"is_on_sale"`
	Source             string  `json:"source"` // "network", "html", "ocr", etc.
	ExtractionMethod   string  `json:"extraction_method"` // "hybrid_match", "ocr_fallback", "network_fallback", etc.
	Confidence         float64 `json:"confidence"` // 0.0 to 1.0 confidence score
}

// UserChoiceRequest represents user's choice when prices don't match
type UserChoiceRequest struct {
	URLID        int     `json:"url_id" validate:"required"`
	ChosenPrice  float64 `json:"chosen_price" validate:"required"`
	ChosenSource string  `json:"chosen_source" validate:"required"` // "network" or "ocr"
	ChosenMethod string  `json:"chosen_method" validate:"required"` // "network", "ocr", "hybrid"
}

// ScrapingPreference represents the preferred scraping method for a URL
type ScrapingPreference struct {
	URLID       int       `json:"url_id" db:"url_id"`
	Method      string    `json:"method" db:"method"` // "network", "ocr", "hybrid"
	Confidence  float64   `json:"confidence" db:"confidence"`
	SuccessRate float64   `json:"success_rate" db:"success_rate"`
	LastUsed    time.Time `json:"last_used" db:"last_used"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
} 
