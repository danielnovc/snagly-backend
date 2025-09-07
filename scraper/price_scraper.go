package scraper

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"distrack/models"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type PriceScraper struct {
	browser *rod.Browser
	ocrExtractor *DockerOCRExtractor
}

// NewPriceScraper creates a new price scraper instance
func NewPriceScraper() (*PriceScraper, error) {
	// Configure launcher - use system Chromium in Docker, auto-detect locally
	launcher := launcher.New().
		Headless(true).
		NoSandbox(true).
		Leakless(false)
	
	// Check if we're in Docker environment (system Chromium available)
	if _, err := os.Stat("/usr/bin/chromium-browser"); err == nil {
		launcher = launcher.Bin("/usr/bin/chromium-browser")
		log.Printf("Using system Chromium in Docker environment")
	} else {
		log.Printf("Using auto-detected Chromium (local environment)")
	}
	
	url := launcher.MustLaunch()
	log.Printf("Using browser at: %s", url)
	
	browser := rod.New().ControlURL(url).MustConnect()
	
	// Initialize Docker OCR extractor
	ocrServiceURL := getEnv("OCR_SERVICE_URL", "http://ocr-service:5000")
	ocrExtractor := NewDockerOCRExtractor(ocrServiceURL)
	
	// Test OCR service connection
	if err := ocrExtractor.HealthCheck(); err != nil {
		log.Printf("Warning: OCR service not available: %v", err)
		log.Printf("OCR will be disabled. Make sure to run: docker-compose up ocr-service")
		ocrExtractor = nil
	} else {
		log.Printf("✅ OCR service connected at: %s", ocrServiceURL)
	}
	
	return &PriceScraper{
		browser: browser,
		ocrExtractor: ocrExtractor,
	}, nil
}

// GetBrowser returns the browser instance
func (ps *PriceScraper) GetBrowser() *rod.Browser {
	return ps.browser
}

// Close closes the browser and OCR extractor
func (ps *PriceScraper) Close() {
	if ps.ocrExtractor != nil {
		ps.ocrExtractor.Close()
	}
	if ps.browser != nil {
		ps.browser.MustClose()
	}
}

// ScrapePrice extracts price information from a URL
func (ps *PriceScraper) ScrapePrice(url string) (*models.PriceData, error) {
	page := ps.browser.MustPage(url)
	defer page.MustClose()

	// Set viewport to avoid detection
	page.MustSetViewport(1920, 1080, 1.0, false)
	
	// Set user agent and other properties to avoid Shopify bot detection
	page.MustEvalOnNewDocument(`
		// Override user agent
		Object.defineProperty(navigator, 'userAgent', {
			get: function () { return 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'; }
		});
		
		// Override webdriver property
		Object.defineProperty(navigator, 'webdriver', {
			get: () => undefined,
		});
		
		// Override plugins
		Object.defineProperty(navigator, 'plugins', {
			get: () => [1, 2, 3, 4, 5],
		});
		
		// Override languages
		Object.defineProperty(navigator, 'languages', {
			get: () => ['en-US', 'en'],
		});
		
		// Override platform
		Object.defineProperty(navigator, 'platform', {
			get: () => 'Win32',
		});
		
		// Override chrome property
		window.chrome = {
			runtime: {},
		};
		
		// Override permissions
		const originalQuery = window.navigator.permissions.query;
		window.navigator.permissions.query = (parameters) => (
			parameters.name === 'notifications' ?
				Promise.resolve({ state: Notification.permission }) :
				originalQuery(parameters)
		);
	`)
	
	// Wait for page to load
	page.MustWaitLoad()

	// Note: Location popups may affect scraping but we'll handle them later if needed
	log.Printf("Page loaded, proceeding with scraping...")

	// Universal dynamic content handling for all modern e-commerce sites
	log.Printf("Waiting for dynamic content to load...")
	time.Sleep(8 * time.Second) // Longer initial wait for dynamic sites
	
	// Wait for price elements to appear (common pattern across sites)
	log.Printf("Waiting for price elements to load...")
	page.MustWaitStable()
	time.Sleep(3 * time.Second)
	
	// Additional wait for JavaScript-heavy sites
	log.Printf("Final wait for JavaScript content...")
	time.Sleep(2 * time.Second)

	// Try to extract price from network requests first
	priceData, err := ps.ExtractFromNetworkRequests(page)
	if err == nil && priceData.CurrentPrice > 0 {
		log.Printf("Successfully extracted price from network requests: $%.2f", priceData.CurrentPrice)
		return priceData, nil
	}

	// Fallback to HTML parsing
	log.Println("Network extraction failed, falling back to HTML parsing")
	priceData, err = ps.extractFromHTML(page)
	if err != nil {
		return nil, fmt.Errorf("failed to extract price from both network and HTML: %v", err)
	}

	return priceData, nil
}

// ExtractFromNetworkRequests intercepts network responses and looks for price data
func (ps *PriceScraper) ExtractFromNetworkRequests(page *rod.Page) (*models.PriceData, error) {
	// First, try to extract product name from URL for smart matching
	productName := ps.extractProductNameFromURL(page.MustInfo().URL)
	log.Printf("Extracted product name from URL: '%s'", productName)
	log.Println("Attempting to intercept network requests for price data...")
	
	// Universal network interception for all e-commerce sites
	page.MustEvalOnNewDocument(`
		window.priceData = null;
		window.allResponses = [];
		window.originalFetch = window.fetch;
		window.fetch = function(...args) {
			const url = args[0];
			console.log('Intercepted fetch:', url);
			return window.originalFetch.apply(this, args).then(response => {
				if (response.ok) {
					response.clone().json().then(data => {
						console.log('Response data:', data);
						window.allResponses.push({url: url, data: data});
						// Universal price data detection for all e-commerce platforms
						if (url.includes('product') || url.includes('api') || url.includes('price') || 
							url.includes('catalog') || url.includes('item') || url.includes('detail') ||
							url.includes('shopify') || url.includes('products') || url.includes('variants') ||
							url.includes('inventory') || url.includes('availability') ||
							url.includes('aliexpress') || url.includes('amazon') || url.includes('newegg') ||
							url.includes('ebay') || url.includes('walmart') || url.includes('target') ||
							url.includes('bestbuy') || url.includes('shein') || url.includes('wildberries') ||
							url.includes('ozon') || url.includes('data') || url.includes('json')) {
							window.priceData = data;
						}
					}).catch(() => {});
				}
				return response;
			});
		};
		
		// Also intercept XMLHttpRequest for Shopify
		window.originalXHR = window.XMLHttpRequest;
		window.XMLHttpRequest = function() {
			const xhr = new window.originalXHR();
			const originalOpen = xhr.open;
			const originalSend = xhr.send;
			
			xhr.open = function(method, url) {
				console.log('Intercepted XHR:', url);
				this._url = url;
				return originalOpen.apply(this, arguments);
			};
			
			xhr.send = function(data) {
				this.addEventListener('load', function() {
					if (this.status === 200) {
						try {
							const responseData = JSON.parse(this.responseText);
							console.log('XHR Response data:', responseData);
							window.allResponses.push({url: this._url, data: responseData});
							if (this._url.includes('product') || this._url.includes('api') || 
								this._url.includes('shopify') || this._url.includes('products')) {
								window.priceData = responseData;
							}
						} catch (e) {}
					}
				});
				return originalSend.apply(this, arguments);
			};
			
			return xhr;
		};
	`)

	// Wait a bit for any initial requests
	time.Sleep(2 * time.Second)

	// Check if we captured any price data
	result, err := page.Eval("window.priceData")
	if err == nil {
		priceDataStr := result.Value.Str()
		if priceDataStr != "null" && priceDataStr != "" {
			log.Printf("Found price data in network response: %v", priceDataStr)
			// Try to parse the JSON string
			var data interface{}
			if err := json.Unmarshal([]byte(priceDataStr), &data); err == nil {
				if priceData := ps.extractPriceFromJSON(data); priceData != nil {
					return priceData, nil
				}
			}
		}
	}

	// Also check all captured responses for price data
	allResponsesResult, err := page.Eval("window.allResponses")
	if err == nil {
		allResponsesStr := allResponsesResult.Value.Str()
		if allResponsesStr != "[]" && allResponsesStr != "" {
			log.Printf("Found %d total network responses", len(allResponsesStr))
			// Try to parse and search through all responses
			var responses []map[string]interface{}
			if err := json.Unmarshal([]byte(allResponsesStr), &responses); err == nil {
				for _, response := range responses {
					if data, ok := response["data"]; ok {
						if priceData := ps.extractPriceFromJSON(data); priceData != nil {
							log.Printf("Found price data in response from: %v", response["url"])
							return priceData, nil
						}
					}
				}
			}
		}
	}

	// Use smart price matching with product name from URL
	log.Println("Using smart price matching with product name...")
	
	// For special cases like H&M, try to extract product name from page content
	if strings.HasPrefix(productName, "hm_product_") {
		pageProductName := ps.extractProductNameFromPage(page)
		if pageProductName != "" {
			log.Printf("Extracted product name from page: '%s'", pageProductName)
			productName = pageProductName
		}
	}
	
	// Extract all price candidates from the page
	candidates := ps.extractAllPriceCandidates(page)
	log.Printf("Found %d price candidates on the page", len(candidates))
	
	if len(candidates) > 0 {
		// Find the best match based on product name and context
		bestCandidate := ps.findBestPriceMatch(candidates, productName)
		if bestCandidate != nil {
			log.Printf("Selected best price match: $%.2f (selector: %s, context: '%s')", 
				bestCandidate.Price, bestCandidate.Selector, bestCandidate.Context)
			return &models.PriceData{
				CurrentPrice:       bestCandidate.Price,
				OriginalPrice:      bestCandidate.Price,
				Currency:           "€",
				DiscountPercentage: 0.0,
				IsOnSale:           false,
			}, nil
		}
	}

	// If no price found with regular methods, try OCR as backup
	if ps.ocrExtractor != nil {
		log.Printf("No price found with regular methods, trying OCR backup...")
		
		// Get the selectors used for price extraction
		selectors := []string{
			"[data-testid*='price']",
			"[class*='price']",
			"[id*='price']",
			".price",
			".product-price",
			".current-price",
			".sale-price",
			"[data-price]",
		}
		
		// Try OCR extraction
		ocrPrice, err := ps.ocrExtractor.ExtractPriceWithOCR(page, selectors)
		if err == nil && ocrPrice > 0 {
			log.Printf("OCR backup successfully extracted price: $%.2f", ocrPrice)
			return &models.PriceData{
				CurrentPrice:       ocrPrice,
				OriginalPrice:      ocrPrice,
				Currency:           "$",
				DiscountPercentage: 0.0,
				IsOnSale:           false,
			}, nil
		} else {
			log.Printf("OCR backup failed: %v", err)
		}
	}

	return nil, fmt.Errorf("no price data found with regular methods or OCR backup")
}

// extractProductNameFromURL extracts a product identifier from the URL
func (ps *PriceScraper) extractProductNameFromURL(url string) string {
	// Enhanced patterns for product URLs - more comprehensive and flexible
	patterns := []string{
		// Standard e-commerce patterns
		`/products/([^/?]+)`,           // /products/product-name
		`/p/([^/?]+)`,                  // /p/product-name
		`/item/([^/?]+)`,               // /item/product-name
		`/product/([^/?]+)`,            // /product/product-name
		`/shop/([^/?]+)`,               // /shop/product-name
		`/buy/([^/?]+)`,                // /buy/product-name
		
		// Collection-based patterns
		`/collections/[^/]+/products/([^/?]+)`, // /collections/name/products/product-name
		`/category/[^/]+/products/([^/?]+)`,    // /category/name/products/product-name
		
		// H&M specific pattern
		`/productpage\.(\d+)\.html`,    // /productpage.1306692001.html
		
		// Generic patterns for various sites
		`/([^/?]+)/([^/?]+)$`,          // /category/product-name
		`/([^/?]+)/([^/?]+)/([^/?]+)$`, // /category/subcategory/product-name
		
		// URL with query parameters
		`/products/([^/?&]+)`,          // /products/product-name?param=value
		`/p/([^/?&]+)`,                 // /p/product-name?param=value
		
		// Subdomain patterns
		`/products/([^/?]+)`,           // subdomain.com/products/product-name
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(url)
		if len(matches) > 1 {
			// For patterns with multiple groups, use the last one (most specific)
			productName := matches[len(matches)-1]
			
			// Special handling for H&M numeric product IDs
			if strings.Contains(pattern, `productpage\.(\d+)\.html`) {
				// For H&M, we'll use the numeric ID as a fallback
				// The actual product name will be extracted from the page content
				log.Printf("H&M product ID detected: %s", productName)
				return "hm_product_" + productName // Prefix to identify H&M products
			}
			
			// Clean up the product name
			productName = strings.ReplaceAll(productName, "-", " ")
			productName = strings.ReplaceAll(productName, "_", " ")
			productName = strings.ReplaceAll(productName, ".", " ")
			
			// Remove common URL artifacts
			productName = strings.TrimSpace(productName)
			
			log.Printf("Extracted product name from URL: '%s'", productName)
			return strings.ToLower(productName)
		}
	}
	
	// Fallback: try to extract from the last path segment
	urlParts := strings.Split(strings.TrimSuffix(url, "/"), "/")
	if len(urlParts) > 0 {
		lastPart := urlParts[len(urlParts)-1]
		// Remove file extensions and query parameters
		lastPart = strings.Split(lastPart, ".")[0]
		lastPart = strings.Split(lastPart, "?")[0]
		lastPart = strings.ReplaceAll(lastPart, "-", " ")
		lastPart = strings.ReplaceAll(lastPart, "_", " ")
		lastPart = strings.TrimSpace(lastPart)
		
		if lastPart != "" && lastPart != "html" && lastPart != "php" {
			log.Printf("Fallback product name from URL: '%s'", lastPart)
			return strings.ToLower(lastPart)
		}
	}
	
	log.Printf("No product name extracted from URL: %s", url)
	return ""
}

// extractProductNameFromPage extracts product name from page content
func (ps *PriceScraper) extractProductNameFromPage(page *rod.Page) string {
	// Try to get product name from common selectors
	productNameSelectors := []string{
		"h1", ".product-title", ".product-name", "[data-product-title]", 
		".product__title", ".product__name", ".product-title", ".product-name",
		"[class*='title']", "[class*='name']", ".title", ".name",
	}
	
	for _, selector := range productNameSelectors {
		elements := page.MustElements(selector)
		for _, element := range elements {
			text := element.MustText()
			text = strings.TrimSpace(text)
			
			// Skip empty or very short text
			if len(text) < 3 {
				continue
			}
			
			// Skip common non-product text
			textLower := strings.ToLower(text)
			if strings.Contains(textLower, "cookie") || 
			   strings.Contains(textLower, "privacy") || 
			   strings.Contains(textLower, "terms") ||
			   strings.Contains(textLower, "shipping") ||
			   strings.Contains(textLower, "delivery") {
				continue
			}
			
			// Clean up the text
			text = strings.ReplaceAll(text, "\n", " ")
			text = strings.ReplaceAll(text, "\t", " ")
			text = strings.ReplaceAll(text, "  ", " ")
			text = strings.TrimSpace(text)
			
			if len(text) > 3 {
				log.Printf("Found product name from page: '%s'", text)
				return strings.ToLower(text)
			}
		}
	}
	
	// Fallback: try to get from page title
	title, err := page.Eval("document.title")
	if err == nil {
		titleText := title.Value.Str()
		if len(titleText) > 3 {
			// Clean up title
			titleText = strings.ReplaceAll(titleText, " | ", " ")
			titleText = strings.ReplaceAll(titleText, " - ", " ")
			titleText = strings.ReplaceAll(titleText, " – ", " ")
			titleText = strings.TrimSpace(titleText)
			
			log.Printf("Found product name from page title: '%s'", titleText)
			return strings.ToLower(titleText)
		}
	}
	
	return ""
}

// PriceCandidate represents a price found on the page with context
type PriceCandidate struct {
	Price     float64
	Text      string
	Selector  string
	ElementID string
	Context   string // Surrounding text for context
}

// extractAllPriceCandidates extracts all prices from the page with context
func (ps *PriceScraper) extractAllPriceCandidates(page *rod.Page) []PriceCandidate {
	var candidates []PriceCandidate
	
	// Universal price selectors for all major e-commerce sites
	selectors := []string{
		// Standard price selectors (works on most sites)
		"[data-price]", "[data-current-price]", "[data-product-price]",
		".price", ".current-price", ".product-price", ".item-price",
		".sale-price", ".final-price", ".regular-price", ".list-price",
		
		// CSS class patterns (covers most naming conventions)
		"[class*='price']", "[class*='Price']", "[class*='cost']", "[class*='Cost']",
		"[id*='price']", "[id*='Price']", "[id*='cost']", "[id*='Cost']",
		
		// Common price element classes
		".price__current", ".price__value", ".price__regular", ".price__sale",
		".product-price__current", ".product-price__value", ".product-price__regular",
		".price-current", ".price-value", ".price-regular", ".price-sale",
		".product-price-current", ".product-price-value", ".product-price-regular",
		
		// Data attributes (modern e-commerce standard)
		"[data-testid*='price']", "[data-testid*='Price']", "[data-testid*='cost']",
		"[data-cy*='price']", "[data-cy*='Price']", "[data-cy*='cost']",
		"[data-test*='price']", "[data-test*='Price']", "[data-test*='cost']",
		
		// Money/currency specific selectors
		".money", ".price-money", ".product-money", ".price__money",
		".price-amount", ".price-currency", ".product-price-amount",
		".amount", ".currency", ".cost-amount", ".price-cost",
		
		// Product-specific price containers
		".product__price", ".product-price", ".item__price", ".item-price",
		".product-single__price", ".product__current-price", ".product__price--regular",
		".product__price--sale", ".product__price--compare", ".price-item--regular",
		".price-item--sale", ".price-item--compare", ".product__price-item",
		
		// Newegg specific selectors
		".price-current", ".price-value", ".product-price-current",
		"[data-price]", "[data-current-price]", ".price-current",
		".price-now", ".price-today", ".current-price-value",
		".price-main", ".price-primary", ".price-display",
		".product-price-main", ".product-price-primary",
		"[data-testid='price-current']", "[data-testid='price-main']",
		".price-current-fraction", ".price-current-whole",
		".price-current-symbol", ".price-current-amount",
		
		// Amazon specific selectors
		".a-price-whole", ".a-price-fraction", ".a-price-symbol",
		".a-offscreen", ".a-price", ".a-price-range",
		
		// Generic fallback selectors (last resort)
		"span", "div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "strong", "b",
	}
	
	// Simple approach: collect all price elements without position calculation
	for _, selector := range selectors {
		elements := page.MustElements(selector)
		for _, element := range elements {
			// Skip image elements and other non-text elements
			tagName, err := element.Eval("this.tagName.toLowerCase()")
			if err == nil {
				tag := tagName.Value.Str()
				if tag == "img" || tag == "image" || tag == "svg" || tag == "canvas" {
					continue // Skip image elements
				}
			}
			
			text := element.MustText()
			if strings.Contains(text, "€") || strings.Contains(text, "$") || strings.Contains(text, "£") {
				// Skip financing/payment text
				textLower := strings.ToLower(text)
				if strings.Contains(textLower, "/mo") || strings.Contains(textLower, "monthly") || 
				   strings.Contains(textLower, "apr") || strings.Contains(textLower, "affirm") ||
				   strings.Contains(textLower, "starting at") || strings.Contains(textLower, "from") {
					continue
				}
				
				// Skip text that contains image filenames or URLs
				if strings.Contains(textLower, ".jpg") || strings.Contains(textLower, ".jpeg") || 
				   strings.Contains(textLower, ".png") || strings.Contains(textLower, ".gif") ||
				   strings.Contains(textLower, ".webp") || strings.Contains(textLower, ".svg") ||
				   strings.Contains(textLower, "cdn/shop/files") || strings.Contains(textLower, "cdn/shop") {
					continue // Skip image filenames and CDN URLs
				}
				
				// Extract all prices from this element
				prices := ps.extractAllPrices(text)
				for _, price := range prices {
									// Universal price validation (works for all product types)
				if price > 0 && price < 1000000 { // Universal reasonable price range
						// Get surrounding context (parent element text)
						context := ps.getElementContext(element)
						
						candidate := PriceCandidate{
							Price:     price,
							Text:      text,
							Selector:  selector,
							ElementID: fmt.Sprintf("selector_%s", selector),
							Context:   context,
						}
						candidates = append(candidates, candidate)
					}
				}
			}
		}
	}
	
	return candidates
}

// getElementContext gets the surrounding context of an element
func (ps *PriceScraper) getElementContext(element *rod.Element) string {
	// Try to get parent element text for context
	parent, err := element.Parent()
	if err == nil {
		parentText := parent.MustText()
		// Limit context length
		if len(parentText) > 200 {
			parentText = parentText[:200] + "..."
		}
		return parentText
	}
	
	// Fallback: try to get the element's own text if parent fails
	text, err := element.Text()
	if err == nil && len(text) > 0 {
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		return text
	}
	
	return ""
}



// findBestPriceMatch finds the best price match based on product name and context
func (ps *PriceScraper) findBestPriceMatch(candidates []PriceCandidate, productName string) *PriceCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return &candidates[0]
	}
	
	var bestCandidate *PriceCandidate
	bestScore := 0.0
	
	// First, filter out obviously wrong prices (like $1.50 for luxury items)
	validCandidates := ps.filterValidPrices(candidates, productName)
	
	if len(validCandidates) == 0 {
		log.Printf("No valid prices found after filtering, using all candidates")
		validCandidates = candidates
	}
	
	// Calculate scores for all valid candidates
	for i := range validCandidates {
		candidate := &validCandidates[i]
		score := ps.calculateMatchScore(candidate, productName)
		
		log.Printf("Price candidate $%.2f: score %.2f (context: '%s')", 
			candidate.Price, score, candidate.Context)
		
		if score > bestScore {
			bestScore = score
			bestCandidate = &validCandidates[i]
		}
	}
	
	if bestCandidate != nil {
		log.Printf("Selected best candidate: $%.2f with final score %.2f", 
			bestCandidate.Price, bestScore)
	}
	
	return bestCandidate
}

// filterValidPrices filters out obviously wrong prices based on context
func (ps *PriceScraper) filterValidPrices(candidates []PriceCandidate, productName string) []PriceCandidate {
	var validCandidates []PriceCandidate
	
	// Find the median price to understand the price range
	var prices []float64
	for _, candidate := range candidates {
		prices = append(prices, candidate.Price)
	}
	
	if len(prices) == 0 {
		return candidates
	}
	
	// Sort prices to find median
	sort.Float64s(prices)
	medianPrice := prices[len(prices)/2]
	
	// Calculate price range
	minPrice := prices[0]
	maxPrice := prices[len(prices)-1]
	priceRange := maxPrice - minPrice
	
	log.Printf("Price analysis: min=%.2f, max=%.2f, median=%.2f, range=%.2f", 
		minPrice, maxPrice, medianPrice, priceRange)
	
	for _, candidate := range candidates {
		// Skip prices that are too low compared to the median
		if medianPrice > 100 && candidate.Price < medianPrice*0.1 {
			log.Printf("Filtering out $%.2f: too low compared to median $%.2f", candidate.Price, medianPrice)
			continue
		}
		
		// Skip prices that are too high compared to the median
		if medianPrice > 0 && candidate.Price > medianPrice*10 {
			log.Printf("Filtering out $%.2f: too high compared to median $%.2f", candidate.Price, medianPrice)
			continue
		}
		
		// Skip very low prices for luxury items (based on product name)
		if ps.isLuxuryProduct(productName) && candidate.Price < 50 {
			log.Printf("Filtering out $%.2f: too low for luxury product", candidate.Price)
			continue
		}
		
		validCandidates = append(validCandidates, candidate)
	}
	
	log.Printf("Filtered %d candidates down to %d valid candidates", len(candidates), len(validCandidates))
	return validCandidates
}

// isLuxuryProduct checks if the product name suggests a luxury item
func (ps *PriceScraper) isLuxuryProduct(productName string) bool {
	luxuryKeywords := []string{
		"chloe", "louis vuitton", "gucci", "hermes", "chanel", "prada", "fendi", "balenciaga",
		"dior", "celine", "givenchy", "saint laurent", "valentino", "bottega", "moynat",
		"goyard", "delvaux", "mansur gavriel", "strathberry", "aspinal", "mulberry",
		"leather", "premium", "luxury", "designer", "handbag", "bag", "purse", "tote",
	}
	
	productNameLower := strings.ToLower(productName)
	for _, keyword := range luxuryKeywords {
		if strings.Contains(productNameLower, keyword) {
			return true
		}
	}
	
	return false
}

// calculateMatchScore calculates how well a price candidate matches the product name
func (ps *PriceScraper) calculateMatchScore(candidate *PriceCandidate, productName string) float64 {
	score := 0.0
	
	// Combine text and context for analysis
	fullText := strings.ToLower(candidate.Text + " " + candidate.Context)
	
	// Enhanced product name matching - more flexible and product-agnostic
	if productName != "" {
		// Handle special cases like H&M product IDs
		if strings.HasPrefix(productName, "hm_product_") {
			// For H&M, we'll rely more on page content and less on URL matching
			// The product name will be extracted from the page title or content
			log.Printf("H&M product detected, using page content for matching")
		} else {
			// Standard product name matching
			keywords := strings.Fields(productName)
			matchedKeywords := 0
			
			for _, keyword := range keywords {
				if len(keyword) > 2 { // Only meaningful keywords
					if strings.Contains(fullText, keyword) {
						score += 15.0 // Increased base score for keyword match
						matchedKeywords++
					}
				}
			}
			
			// Bonus for matching multiple keywords
			if len(keywords) > 0 {
				matchRatio := float64(matchedKeywords) / float64(len(keywords))
				if matchRatio >= 0.6 { // At least 60% of keywords match
					score += matchRatio * 80.0 // Up to 80 points for good keyword coverage
					log.Printf("Price candidate $%.2f: +%.1f bonus for keyword match (%.0f%% of keywords: %d/%d)", 
						candidate.Price, matchRatio*80.0, matchRatio*100, matchedKeywords, len(keywords))
				}
			}
			
			// Exact product name match (very high bonus)
			if strings.Contains(fullText, strings.ToLower(productName)) {
				score += 150.0 // Very high bonus for exact match
				log.Printf("Price candidate $%.2f: +150.0 bonus for exact product name match", candidate.Price)
			}
		}
	}
	
	// Enhanced partial product name matching - more flexible but still accurate
	if productName != "" && !strings.HasPrefix(productName, "hm_product_") {
		productWords := strings.Fields(strings.ToLower(productName))
		contextWords := strings.Fields(strings.ToLower(fullText))
		
		// Count how many product words are found in the context
		matchedWords := 0
		for _, productWord := range productWords {
			if len(productWord) > 2 && !isNumeric(productWord) { // Only meaningful words, skip numbers
				for _, contextWord := range contextWords {
					// More flexible matching: exact match or contains match for longer words
					if contextWord == productWord || 
					   (len(productWord) > 4 && strings.Contains(contextWord, productWord)) ||
					   (len(contextWord) > 4 && strings.Contains(productWord, contextWord)) {
						matchedWords++
						break
					}
				}
			}
		}
		
		// Bonus based on how many product words match
		if len(productWords) > 0 {
			// Filter out numeric words for percentage calculation
			meaningfulWords := 0
			for _, word := range productWords {
				if len(word) > 2 && !isNumeric(word) {
					meaningfulWords++
				}
			}
			
			if meaningfulWords > 0 {
				matchPercentage := float64(matchedWords) / float64(meaningfulWords)
				if matchPercentage >= 0.4 { // At least 40% of meaningful words match (more flexible)
					bonus := matchPercentage * 120.0 // Up to 120 points for good partial matches
					score += bonus
					log.Printf("Price candidate $%.2f: +%.1f bonus for partial product name match (%.0f%% of meaningful words: %d/%d)", 
						candidate.Price, bonus, matchPercentage*100, matchedWords, meaningfulWords)
				}
			}
		}
	}
	
	// Bonus for main product keywords in the URL
	if productName != "" {
		urlKeywords := strings.Fields(productName)
		for _, keyword := range urlKeywords {
			if len(keyword) > 3 { // Only longer keywords
				// Check if this keyword appears in the price context
				if strings.Contains(fullText, keyword) {
					score += 15.0 // High bonus for URL keyword match
				}
			}
		}
	}
	
	// Score based on price context indicators
	positiveIndicators := []string{"sale", "current", "price", "cost", "amount", "add to bag", "add to cart"}
	negativeIndicators := []string{"upgrade", "related", "recommended", "similar", "you might also like", "more from this collection", "you may also like"}
	
	for _, indicator := range positiveIndicators {
		if strings.Contains(fullText, indicator) {
			score += 5.0
		}
	}
	
	for _, indicator := range negativeIndicators {
		if strings.Contains(fullText, indicator) {
			score -= 15.0 // Heavy penalty for related products
		}
	}
	
	// Heavy penalty for contexts that contain multiple product names/prices
	// This helps avoid picking prices from product grids or related products sections
	fullTextLower := strings.ToLower(fullText)
	
	// Count prices in the full context (not just immediate text)
	priceCount := strings.Count(fullTextLower, "€") + strings.Count(fullTextLower, "$") + strings.Count(fullTextLower, "£")
	
	// Heavy penalty for multi-product contexts
	if priceCount > 3 {
		score -= 50.0 // Heavy penalty for contexts with multiple products
		log.Printf("Price candidate $%.2f: -50.0 penalty for multi-product context (%d prices found)", candidate.Price, priceCount)
	}
	
	// Additional penalty for contexts that contain "ADD TO CART" with multiple products
	if strings.Contains(fullTextLower, "add to cart") && priceCount > 2 {
		score -= 30.0 // Heavy penalty for ADD TO CART in multi-product context
		log.Printf("Price candidate $%.2f: -30.0 penalty for ADD TO CART in multi-product context", candidate.Price)
	}
	
	// Generic penalty for prices that contain words not in the product name
	// This helps differentiate between the main product and related items
	productWords := strings.Fields(strings.ToLower(productName))
	contextWords := strings.Fields(strings.ToLower(fullText))
	
	// Count how many context words are NOT in the product name
	unrelatedWords := 0
	for _, contextWord := range contextWords {
		if len(contextWord) > 3 { // Only meaningful words
			found := false
			for _, productWord := range productWords {
				if contextWord == productWord {
					found = true
					break
				}
			}
			if !found {
				unrelatedWords++
			}
		}
	}
	
	// Penalty based on number of unrelated words (but not too harsh)
	if unrelatedWords > 5 {
		score -= 15.0 // Moderate penalty for too many unrelated words
	}
	
	// Generic penalty for contexts that contain unrelated product indicators
	// This helps avoid picking prices from related products sections
	// But only apply if the product name is available and doesn't contain the indicator
	if productName != "" {
		// Look for common product type indicators that might indicate a different product
		productIndicators := []string{"related", "similar", "recommended", "you might also like", "more from this collection"}
		for _, indicator := range productIndicators {
			if strings.Contains(fullText, indicator) {
				score -= 20.0 // Moderate penalty for related products sections
				log.Printf("Price candidate $%.2f: -20.0 penalty for related products indicator '%s'", candidate.Price, indicator)
				break
			}
		}
	}
	
	// Score based on selector specificity (more specific selectors get higher scores)
	specificSelectors := map[string]float64{
		"[data-price]": 20.0,
		"[data-current-price]": 20.0,
		".product-price": 15.0,
		".price": 10.0,
		"span": 1.0,
		"div": 1.0,
	}
	
	if selectorScore, exists := specificSelectors[candidate.Selector]; exists {
		score += selectorScore
	}
	
	// Universal price range scoring (works for all product types)
	if candidate.Price >= 0.01 && candidate.Price <= 1000000 {
		score += 5.0 // Base score for reasonable price range
	}
	
	// Universal context filtering for all e-commerce sites
	negativeContexts := []string{
		"cookie consent", "cookie policy", "privacy policy", "terms of service",
		"shipping", "delivery", "return policy", "contact us", "about us",
		"trade in", "trade-in", "credit toward", "promotional", "rebate",
		"cashback", "bonus", "similar products", "related products",
		"you might also like", "recommended", "more from", "upgrade",
		"accessories", "add-ons", "extended warranty", "protection plan",
		// Historical data indicators
		"popular item", "in 2006", "in 2007", "in 2008", "in 2009", "in 2010",
		"in 2011", "in 2012", "in 2013", "in 2014", "in 2015", "in 2016",
		"in 2017", "in 2018", "in 2019", "in 2020", "in 2021", "in 2022",
		"in 2023", "selling", "units", "priced at", "was a", "historical",
		"archive", "old", "previous", "former", "discontinued", "retired",
		// Newegg specific negative contexts
		"shipping cost", "shipping fee", "handling fee", "processing fee",
		"tax", "estimated tax", "sales tax", "state tax", "local tax",
		"recycling fee", "environmental fee", "battery fee", "disposal fee",
		"restocking fee", "return fee", "cancellation fee", "late fee",
		"monthly", "per month", "/mo", "monthly payment", "installment",
		"financing", "credit", "loan", "apr", "interest", "payment plan",
		"starting at", "from", "as low as", "minimum", "deposit", "down payment",
		"cable", "adapter", "connector", "mount", "stand", "bracket", "screw",
		"manual", "guide", "documentation", "driver", "software", "utility",
		"warranty", "protection", "insurance", "coverage", "plan", "service",
		"subscription", "membership", "club", "rewards", "points", "cashback",
		"rebate", "discount code", "coupon", "promo", "deal", "offer",
		"limited time", "while supplies last", "quantities limited",
		"estimated", "approximate", "around", "about", "roughly", "circa",
		"price range", "price varies", "call for price", "contact for price",
		"price upon request", "quote", "estimate", "bid", "auction",
	}
	
	for _, context := range negativeContexts {
		if strings.Contains(strings.ToLower(fullText), context) {
			score -= 30.0 // Universal penalty for negative contexts
			log.Printf("Price candidate $%.2f: -30.0 penalty for negative context '%s'", candidate.Price, context)
			break
		}
	}
	
	// Heavy penalty for historical price patterns (like "in 2006, selling X units priced at $Y")
	historicalPatterns := []string{
		"in 20", "selling", "units", "priced at", "was a popular item",
		"historical data", "archive", "old price", "previous price",
	}
	for _, pattern := range historicalPatterns {
		if strings.Contains(strings.ToLower(fullText), pattern) {
			score -= 100.0 // Very heavy penalty for historical data
			log.Printf("Price candidate $%.2f: -100.0 penalty for historical pattern '%s'", candidate.Price, pattern)
			break
		}
	}
	
	// Universal positive indicators for main product prices
	positiveIndicators = []string{"add to cart", "add to bag", "buy now", "purchase", "checkout"}
	for _, indicator := range positiveIndicators {
		if strings.Contains(strings.ToLower(fullText), indicator) {
			score += 20.0 // Universal bonus for main product indicators
			log.Printf("Price candidate $%.2f: +20.0 bonus for positive indicator '%s'", candidate.Price, indicator)
			break
		}
	}
	
	// Bonus for prices that are likely the main product based on universal patterns
	// Higher prices are often the main product, but this should be relative to other candidates
	// We'll handle this in the selection logic instead of hardcoding price ranges
	
	// Bonus for clean, single-product contexts
	if priceCount <= 2 {
		score += 25.0 // Bonus for clean contexts
		log.Printf("Price candidate $%.2f: +25.0 bonus for clean context (%d prices)", candidate.Price, priceCount)
	}
	
	// Bonus for contexts that contain the exact product name
	if productName != "" && strings.Contains(fullTextLower, strings.ToLower(productName)) {
		score += 40.0 // High bonus for exact product name in context
		log.Printf("Price candidate $%.2f: +40.0 bonus for exact product name in context", candidate.Price)
	}
	
	return score
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// extractAllPrices extracts all price values from a text string
func (ps *PriceScraper) extractAllPrices(text string) []float64 {
	var prices []float64
	
	// Universal price pattern matching for all currencies and formats
	// Supports: $123.45, €123,45, £123.45, 123.45, 123,45, etc.
	pricePattern := regexp.MustCompile(`([\$€£¥₹₽₩₪₦₨₫₴₸₺₼₾₿])\s*(\d+(?:[.,]\d{2})?)|(\d+(?:[.,]\d{2})?)\s*([\$€£¥₹₽₩₪₦₨₫₴₸₺₼₾₿])|(\d+(?:[.,]\d{2})?)\s*(?:USD|EUR|GBP|JPY|INR|RUB|KRW|ILS|NGN|PKR|VND|UAH|KZT|TRY|AZN|GEL)`)
	matches := pricePattern.FindAllStringSubmatch(text, -1)
	
	for _, match := range matches {
		var priceStr string
		
		// Extract price from different match groups
		if len(match) >= 3 {
			if match[1] != "" && match[2] != "" {
				// Format: $123.45
				priceStr = match[2]
			} else if match[3] != "" && match[4] != "" {
				// Format: 123.45$
				priceStr = match[3]
			} else if match[5] != "" {
				// Format: 123.45 USD
				priceStr = match[5]
			}
		}
		
		if priceStr != "" {
					// Handle different decimal separators
		if strings.Contains(priceStr, ",") && strings.Contains(priceStr, ".") {
			// Both comma and dot present - check if it's thousands separator
			// If the comma is followed by exactly 3 digits, it's likely thousands separator
			if strings.Contains(priceStr, ",") {
				parts := strings.Split(priceStr, ",")
				if len(parts) == 2 && len(parts[0]) <= 4 && len(parts[1]) >= 3 {
					// This looks like thousands separator (e.g., "1,500.00")
					priceStr = strings.ReplaceAll(priceStr, ",", "")
				} else {
					// This might be European format (e.g., "1.500,00")
					priceStr = strings.ReplaceAll(priceStr, ".", "")
					priceStr = strings.ReplaceAll(priceStr, ",", ".")
				}
			}
		} else if strings.Contains(priceStr, ",") {
			// Only comma present - treat as decimal separator (European format)
			priceStr = strings.ReplaceAll(priceStr, ",", ".")
		}
			
			if price, err := strconv.ParseFloat(priceStr, 64); err == nil && price > 0 {
				// Universal price validation (works for all product types)
				if price >= 2020 && price <= 2030 {
					continue // Skip years
				}
				if price < 1.0 {
					continue // Skip very low prices (likely not main product prices)
				}
				if price > 1000000 {
					continue // Skip unreasonably high prices
				}
				
				// Additional validation for very low prices that are likely fees, taxes, etc.
				if price < 10.0 {
					// Check if this looks like a fee, tax, or accessory price
					textLower := strings.ToLower(text)
					if strings.Contains(textLower, "shipping") || strings.Contains(textLower, "tax") ||
					   strings.Contains(textLower, "fee") || strings.Contains(textLower, "handling") ||
					   strings.Contains(textLower, "processing") || strings.Contains(textLower, "cable") ||
					   strings.Contains(textLower, "adapter") || strings.Contains(textLower, "mount") ||
					   strings.Contains(textLower, "stand") || strings.Contains(textLower, "bracket") ||
					   strings.Contains(textLower, "screw") || strings.Contains(textLower, "manual") ||
					   strings.Contains(textLower, "guide") || strings.Contains(textLower, "driver") ||
					   strings.Contains(textLower, "software") || strings.Contains(textLower, "utility") ||
					   strings.Contains(textLower, "threshold") || strings.Contains(textLower, "minimum") {
						continue // Skip these low prices
					}
				}
				
				prices = append(prices, price)
			}
		}
	}
	
	// If no prices found with currency symbols, try a more lenient approach
	// but only for text that looks like it contains actual prices
	if len(prices) == 0 {
		textLower := strings.ToLower(text)
		// Only try lenient matching if the text contains price-related words
		if strings.Contains(textLower, "price") || strings.Contains(textLower, "cost") || 
		   strings.Contains(textLower, "amount") || strings.Contains(textLower, "eur") ||
		   strings.Contains(textLower, "usd") || strings.Contains(textLower, "gbp") {
			
			// More lenient pattern for numbers that could be prices
			lenientPattern := regexp.MustCompile(`(\d+(?:[.,]\d{2})?)`)
			lenientMatches := lenientPattern.FindAllStringSubmatch(text, -1)
			
			for _, match := range lenientMatches {
				if len(match) > 1 {
					priceStr := strings.ReplaceAll(match[1], ",", ".")
					if price, err := strconv.ParseFloat(priceStr, 64); err == nil && price > 0 && price < 10000 {
						// Stricter sanity checks for lenient matching
						if price >= 2020 && price <= 2030 {
							continue // Skip years
						}
						if price > 2000 {
							continue // Skip very high numbers
						}
						if price < 20 {
							continue // Skip very low prices
						}
						prices = append(prices, price)
					}
				}
			}
		}
	}
	
	return prices
}

// selectMainPrice chooses the most likely main product price from multiple candidates
func (ps *PriceScraper) selectMainPrice(prices []float64, text string) float64 {
	if len(prices) == 0 {
		return 0
	}
	if len(prices) == 1 {
		return prices[0]
	}
	
	textLower := strings.ToLower(text)
	
	// Priority order for price selection:
	// 1. Sale price (if text contains "sale")
	// 2. Regular price (if text contains "regular")
	// 3. Current price (if text contains "current")
	// 4. The first reasonable price (not too high, not too low)
	
	// Look for sale price first
	if strings.Contains(textLower, "sale") {
		for _, price := range prices {
			if price > 10 && price < 1000 { // Reasonable sale price range
				return price
			}
		}
	}
	
	// Look for regular price
	if strings.Contains(textLower, "regular") {
		for _, price := range prices {
			if price > 10 && price < 1000 {
				return price
			}
		}
	}
	
	// Look for current price
	if strings.Contains(textLower, "current") {
		for _, price := range prices {
			if price > 10 && price < 1000 {
				return price
			}
		}
	}
	
	// If no specific indicators, return the first reasonable price
	for _, price := range prices {
		if price > 5 && price < 2000 { // Wider reasonable range
			return price
		}
	}
	
	// Fallback to the first price
	return prices[0]
}

// isPriceResponse checks if a URL likely contains price data
func (ps *PriceScraper) isPriceResponse(url string) bool {
	priceKeywords := []string{
		"price", "pricing", "product", "item", "api", "graphql",
		"search", "catalog", "inventory", "stock", "availability",
	}

	urlLower := strings.ToLower(url)
	for _, keyword := range priceKeywords {
		if strings.Contains(urlLower, keyword) {
			return true
		}
	}

	return false
}

// extractPriceFromJSON recursively searches for price data in JSON
func (ps *PriceScraper) extractPriceFromJSON(data interface{}) *models.PriceData {
	switch v := data.(type) {
	case map[string]interface{}:
		// Look for common price field names
		priceFields := []string{"price", "current_price", "sale_price", "final_price", "amount"}
		originalPriceFields := []string{"original_price", "regular_price", "list_price", "msrp"}
		currencyFields := []string{"currency", "currency_code", "currency_symbol"}

		var currentPrice, originalPrice float64
		var currency string

		// Extract current price
		for _, field := range priceFields {
			if price, ok := ps.extractFloat(v[field]); ok && price > 0 {
				currentPrice = price
				break
			}
		}

		// Extract original price
		for _, field := range originalPriceFields {
			if price, ok := ps.extractFloat(v[field]); ok && price > 0 {
				originalPrice = price
				break
			}
		}

		// Extract currency
		for _, field := range currencyFields {
			if curr, ok := v[field].(string); ok && curr != "" {
				currency = curr
				break
			}
		}

		// If we found a price, return the data
		if currentPrice > 0 {
			if originalPrice == 0 {
				originalPrice = currentPrice
			}

			discountPercentage := 0.0
			if originalPrice > currentPrice {
				discountPercentage = ((originalPrice - currentPrice) / originalPrice) * 100
			}

			return &models.PriceData{
				CurrentPrice:       currentPrice,
				OriginalPrice:      originalPrice,
				Currency:           currency,
				DiscountPercentage: discountPercentage,
				IsOnSale:           discountPercentage > 0,
			}
		}

		// Recursively search nested objects
		for _, value := range v {
			if result := ps.extractPriceFromJSON(value); result != nil {
				return result
			}
		}

	case []interface{}:
		// Search through arrays
		for _, item := range v {
			if result := ps.extractPriceFromJSON(item); result != nil {
				return result
			}
		}
	}

	return nil
}

// extractFloat safely extracts a float value from various types
func (ps *PriceScraper) extractFloat(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		// Remove currency symbols and whitespace
		clean := regexp.MustCompile(`[^\d.,]`).ReplaceAllString(v, "")
		
		// Handle European decimal format (comma as decimal separator)
		if strings.Contains(clean, ",") && strings.Contains(clean, ".") {
			// If both comma and dot exist, assume comma is thousands separator
			clean = strings.ReplaceAll(clean, ",", "")
		} else if strings.Contains(clean, ",") {
			// If only comma exists, treat it as decimal separator (European format)
			clean = strings.ReplaceAll(clean, ",", ".")
		}
		
		// Additional validation for European prices
		if strings.Contains(v, "€") && !strings.Contains(v, "$") {
			// This is likely a European price, ensure proper decimal handling
			if strings.Contains(clean, ".") {
				parts := strings.Split(clean, ".")
				if len(parts) == 2 && len(parts[1]) == 2 {
					// This looks like a proper decimal format
				} else {
					// Might be a thousands separator, remove dots
					clean = strings.ReplaceAll(clean, ".", "")
				}
			}
		}
		
		if f, err := strconv.ParseFloat(clean, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// extractFromHTML falls back to HTML parsing when network interception fails
func (ps *PriceScraper) extractFromHTML(page *rod.Page) (*models.PriceData, error) {
	// Enhanced selectors for price elements - more comprehensive
	priceSelectors := []string{
		// Common price selectors
		"[data-price]", "[data-current-price]", ".price", ".current-price",
		".sale-price", ".final-price", ".product-price", ".item-price",
		"[class*='price']", "[class*='Price']", "[id*='price']", "[id*='Price']",
		
		// Additional selectors for different sites
		".price__current", ".price__value", ".product-price__current",
		".price-current", ".price-value", ".product-price-value",
		"[data-testid*='price']", "[data-testid*='Price']",
		".price-amount", ".price-currency", ".product-price-amount",
		
		// H&M specific selectors
		".price__value", ".price__current", ".product-price__value",
		"[data-testid='price']", "[data-testid='Price']",
		
		// Mango specific selectors
		".product-price", ".price-current", ".price-value",
		"[data-price]", "[data-current-price]",
		
		// Generic number patterns - removed invalid :contains() selectors
	}

	originalPriceSelectors := []string{
		// Common original price selectors
		"[data-original-price]", ".original-price", ".regular-price",
		".list-price", ".msrp", "[class*='original']", "[class*='regular']",
		
		// Additional selectors
		".price__original", ".price__regular", ".product-price__original",
		".price-original", ".price-regular", ".product-price-regular",
		"[data-testid*='original']", "[data-testid*='regular']",
	}

	var currentPrice, originalPrice float64
	var currency string

	// Debug: Log page title to see if we're on the right page
	title, err := page.Eval("document.title")
	if err == nil {
		log.Printf("Page title: %s", title.Value.Str())
	}

	// Try to find current price with enhanced debugging
	log.Printf("Searching for price elements...")
	for i, selector := range priceSelectors {
		elements := page.MustElements(selector)
		log.Printf("Selector %d (%s): found %d elements", i+1, selector, len(elements))
		
		for j, element := range elements {
			text := element.MustText()
			log.Printf("  Element %d text: '%s'", j+1, text)
			
			if price, ok := ps.extractFloat(text); ok && price > 0 {
				currentPrice = price
				log.Printf("Found current price: $%.2f from selector '%s'", currentPrice, selector)
				break
			}
		}
		if currentPrice > 0 {
			break
		}
	}

	// Try to find original price
	for i, selector := range originalPriceSelectors {
		elements := page.MustElements(selector)
		log.Printf("Original price selector %d (%s): found %d elements", i+1, selector, len(elements))
		
		for j, element := range elements {
			text := element.MustText()
			log.Printf("  Original price element %d text: '%s'", j+1, text)
			
			if price, ok := ps.extractFloat(text); ok && price > 0 {
				originalPrice = price
				log.Printf("Found original price: $%.2f from selector '%s'", originalPrice, selector)
				break
			}
		}
		if originalPrice > 0 {
			break
		}
	}

	// If still no price found, try a more aggressive approach
	if currentPrice == 0 {
		log.Printf("No price found with standard selectors, trying fallback methods...")
		
		// Try to find any element containing currency symbols
		fallbackSelectors := []string{"span", "div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "strong", "b"}
		for _, selector := range fallbackSelectors {
			elements := page.MustElements(selector)
			log.Printf("Fallback selector '%s': found %d elements", selector, len(elements))
			
			for j, element := range elements {
				text := element.MustText()
				// Look for patterns like $123.45, €123.45, or just numbers that could be prices
				if strings.Contains(text, "$") || strings.Contains(text, "€") || strings.Contains(text, "£") {
					log.Printf("  Fallback element %d text: '%s'", j+1, text)
					
					// Skip financing/payment text that might contain prices
					textLower := strings.ToLower(text)
					if strings.Contains(textLower, "/mo") || strings.Contains(textLower, "monthly") || 
					   strings.Contains(textLower, "apr") || strings.Contains(textLower, "affirm") ||
					   strings.Contains(textLower, "starting at") || strings.Contains(textLower, "from") {
						log.Printf("  Skipping financing text: '%s'", text)
						continue
					}
					
					// Skip text that contains multiple product names (likely related products)
					if strings.Contains(textLower, "upgrade your look") || 
					   strings.Contains(textLower, "related products") ||
					   strings.Contains(textLower, "you might also like") ||
					   strings.Contains(textLower, "recommended") ||
					   strings.Contains(textLower, "similar") {
						log.Printf("  Skipping related products text: '%s'", text)
						continue
					}
					
					// Extract all prices from this element and find the most likely main price
					prices := ps.extractAllPrices(text)
					if len(prices) > 0 {
						mainPrice := ps.selectMainPrice(prices, text)
						if mainPrice > 0 && mainPrice < 10000 {
							currentPrice = mainPrice
							log.Printf("Found main price via fallback: $%.2f (from %d candidates) from text: '%s'", currentPrice, len(prices), text)
							break
						}
					}
				}
			}
			if currentPrice > 0 {
				break
			}
		}
		
			// If still no price, try to get all text and search for price patterns
	if currentPrice == 0 {
		log.Printf("Trying to extract price from page text...")
		pageText, err := page.Eval("document.body.innerText")
		if err == nil {
			text := pageText.Value.Str()
			log.Printf("Page text length: %d characters", len(text))
			
			// Log first 500 characters to see what we're working with
			if len(text) > 500 {
				log.Printf("Page text preview: %s...", text[:500])
			} else {
				log.Printf("Page text: %s", text)
			}
			
			// Look for price patterns in the entire page text
			pricePattern := regexp.MustCompile(`[\$€£]?\s*(\d+(?:[.,]\d{2})?)`)
			matches := pricePattern.FindAllStringSubmatch(text, -1)
			
			log.Printf("Found %d potential price matches", len(matches))
			for i, match := range matches {
				log.Printf("  Match %d: '%s'", i+1, match[0])
				if len(match) > 1 {
					priceStr := strings.ReplaceAll(match[1], ",", "")
					if price, err := strconv.ParseFloat(priceStr, 64); err == nil && price > 0 && price < 10000 {
						currentPrice = price
						log.Printf("Found price via regex: $%.2f from match: '%s'", currentPrice, match[0])
						break
					}
				}
			}
		}
		
		// If still no price, try to wait for JavaScript to load and try again
		if currentPrice == 0 {
			log.Printf("No price found, waiting for JavaScript to load...")
			time.Sleep(3 * time.Second)
			
			// Try to find any element with numbers that could be prices
			allElements, err := page.Elements("*")
			if err == nil {
				log.Printf("Found %d total elements on page", len(allElements))
				for i, element := range allElements {
					if i > 100 { // Limit to first 100 elements to avoid spam
						break
					}
					text, err := element.Text()
					if err == nil && text != "" {
						// Look for any text containing numbers that could be prices
						if strings.Contains(text, "€") || strings.Contains(text, "$") || strings.Contains(text, "£") {
							log.Printf("  Element %d text: '%s'", i+1, text)
							if price, ok := ps.extractFloat(text); ok && price > 0 && price < 10000 {
								currentPrice = price
								log.Printf("Found price via element search: $%.2f from text: '%s'", currentPrice, text)
								break
							}
						}
					}
				}
			}
		}
	}
	}

	if currentPrice == 0 {
		return nil, fmt.Errorf("no price found in HTML after trying %d selectors", len(priceSelectors))
	}

	if originalPrice == 0 {
		originalPrice = currentPrice
	}

	discountPercentage := 0.0
	if originalPrice > currentPrice {
		discountPercentage = ((originalPrice - currentPrice) / originalPrice) * 100
	}

	return &models.PriceData{
		CurrentPrice:       currentPrice,
		OriginalPrice:      originalPrice,
		Currency:           currency,
		DiscountPercentage: discountPercentage,
		IsOnSale:           discountPercentage > 0,
	}, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
	}