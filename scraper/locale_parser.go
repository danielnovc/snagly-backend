package scraper

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LocaleParser handles different number formats from various regions
type LocaleParser struct {
	// Common patterns for different locales
	patterns map[string]*regexp.Regexp
}

// NewLocaleParser creates a new locale-aware parser
func NewLocaleParser() *LocaleParser {
	return &LocaleParser{
		patterns: map[string]*regexp.Regexp{
			// US/UK: $1,234.56 or £1,234.56
			"us_uk": regexp.MustCompile(`(?i)(\$|£|€)?\s*([0-9]{1,3}(?:,[0-9]{3})*(?:\.[0-9]{2})?)`),
			
			// European: €1.234,56 or €1 234,56
			"european": regexp.MustCompile(`(?i)(\$|£|€)?\s*([0-9]{1,3}(?:[.\s][0-9]{3})*(?:,[0-9]{2})?)`),
			
			// Simple decimal: 1234.56 or 1234,56
			"simple": regexp.MustCompile(`([0-9]+(?:[.,][0-9]{2})?)`),
			
			// Price with currency symbol but no separators
			"currency_only": regexp.MustCompile(`(?i)(\$|£|€)\s*([0-9]+(?:\.[0-9]{2})?)`),
		},
	}
}

// ParsePrice attempts to parse a price string using multiple locale patterns
func (lp *LocaleParser) ParsePrice(text string) (float64, string, error) {
	text = strings.TrimSpace(text)
	
	// Try different patterns in order of specificity
	patterns := []struct {
		name string
		re   *regexp.Regexp
	}{
		{"us_uk", lp.patterns["us_uk"]},
		{"european", lp.patterns["european"]},
		{"currency_only", lp.patterns["currency_only"]},
		{"simple", lp.patterns["simple"]},
	}
	
	for _, pattern := range patterns {
		if matches := pattern.re.FindStringSubmatch(text); matches != nil {
			currency := strings.ToUpper(matches[1])
			numberStr := matches[2]
			
			// Clean up the number string based on locale
			cleanNumber := lp.cleanNumberString(numberStr, pattern.name)
			
			if value, err := strconv.ParseFloat(cleanNumber, 64); err == nil {
				return value, currency, nil
			}
		}
	}
	
	return 0, "", fmt.Errorf("no valid price pattern found in: %s", text)
}

// cleanNumberString converts locale-specific number formats to standard decimal
func (lp *LocaleParser) cleanNumberString(numberStr, locale string) string {
	switch locale {
	case "us_uk":
		// Remove commas: 1,234.56 -> 1234.56
		return strings.ReplaceAll(numberStr, ",", "")
		
	case "european":
		// Convert European format to standard: 1.234,56 -> 1234.56
		// First replace dots with temporary marker
		temp := strings.ReplaceAll(numberStr, ".", "TEMP")
		// Replace comma with dot
		temp = strings.ReplaceAll(temp, ",", ".")
		// Replace temporary marker with nothing
		return strings.ReplaceAll(temp, "TEMP", "")
		
	case "currency_only":
		// Already in standard format
		return numberStr
		
	case "simple":
		// Handle both . and , as decimal separators
		if strings.Contains(numberStr, ",") && !strings.Contains(numberStr, ".") {
			// Assume comma is decimal separator: 1234,56 -> 1234.56
			return strings.ReplaceAll(numberStr, ",", ".")
		}
		return numberStr
		
	default:
		return numberStr
	}
}

// DetectLocale attempts to detect the locale from the text
func (lp *LocaleParser) DetectLocale(text string) string {
	text = strings.ToLower(text)
	
	// Check for European patterns
	if strings.Contains(text, "€") {
		if strings.Contains(text, ",") && (strings.Contains(text, ".") || strings.Contains(text, " ")) {
			return "european"
		}
	}
	
	// Check for US/UK patterns
	if strings.Contains(text, "$") || strings.Contains(text, "£") {
		if strings.Contains(text, ",") && strings.Contains(text, ".") {
			return "us_uk"
		}
	}
	
	// Default to simple
	return "simple"
}

// ExtractAllPrices finds all price-like numbers in text
func (lp *LocaleParser) ExtractAllPrices(text string) []struct {
	Value    float64
	Currency string
	Text     string
} {
	var results []struct {
		Value    float64
		Currency string
		Text     string
	}
	
	// Try each pattern
	for patternName, pattern := range lp.patterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				currency := strings.ToUpper(match[1])
				numberStr := match[2]
				cleanNumber := lp.cleanNumberString(numberStr, patternName)
				
				if value, err := strconv.ParseFloat(cleanNumber, 64); err == nil {
					results = append(results, struct {
						Value    float64
						Currency string
						Text     string
					}{
						Value:    value,
						Currency: currency,
						Text:     match[0],
					})
				}
			}
		}
	}
	
	return results
}
