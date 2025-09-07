package scraper

import (
	"regexp"
	"strings"
)

// BotDetector detects bot walls and CAPTCHAs
type BotDetector struct {
	botPatterns    []*regexp.Regexp
	captchaPatterns []*regexp.Regexp
	blockPatterns  []*regexp.Regexp
}

// NewBotDetector creates a new bot detector
func NewBotDetector() *BotDetector {
	return &BotDetector{
		botPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)unfortunately we are unable`),
			regexp.MustCompile(`(?i)access denied`),
			regexp.MustCompile(`(?i)blocked`),
			regexp.MustCompile(`(?i)bot detected`),
			regexp.MustCompile(`(?i)please verify you are human`),
			regexp.MustCompile(`(?i)security check`),
			regexp.MustCompile(`(?i)cloudflare`),
			regexp.MustCompile(`(?i)distil networks`),
			regexp.MustCompile(`(?i)imperva`),
			regexp.MustCompile(`(?i)akamai`),
			regexp.MustCompile(`(?i)rate limit`),
			regexp.MustCompile(`(?i)too many requests`),
			regexp.MustCompile(`(?i)please wait`),
			regexp.MustCompile(`(?i)checking your browser`),
			regexp.MustCompile(`(?i)ddos protection`),
			regexp.MustCompile(`(?i)captcha`),
			regexp.MustCompile(`(?i)recaptcha`),
			regexp.MustCompile(`(?i)hcaptcha`),
			regexp.MustCompile(`(?i)turnstile`),
		},
		captchaPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)captcha`),
			regexp.MustCompile(`(?i)recaptcha`),
			regexp.MustCompile(`(?i)hcaptcha`),
			regexp.MustCompile(`(?i)turnstile`),
			regexp.MustCompile(`(?i)verify you are human`),
			regexp.MustCompile(`(?i)select all images`),
			regexp.MustCompile(`(?i)click the checkbox`),
		},
		blockPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)403 forbidden`),
			regexp.MustCompile(`(?i)429 too many requests`),
			regexp.MustCompile(`(?i)503 service unavailable`),
			regexp.MustCompile(`(?i)maintenance`),
			regexp.MustCompile(`(?i)under construction`),
			regexp.MustCompile(`(?i)site temporarily unavailable`),
		},
	}
}

// DetectBotWall checks if the page content indicates a bot wall
func (bd *BotDetector) DetectBotWall(pageContent, pageTitle string) (bool, string, float64) {
	content := strings.ToLower(pageContent + " " + pageTitle)
	
	// Check for bot patterns
	botScore := 0.0
	botReasons := []string{}
	
	for _, pattern := range bd.botPatterns {
		if pattern.MatchString(content) {
			botScore += 0.3
			botReasons = append(botReasons, pattern.String())
		}
	}
	
	// Check for CAPTCHA patterns (higher weight)
	for _, pattern := range bd.captchaPatterns {
		if pattern.MatchString(content) {
			botScore += 0.5
			botReasons = append(botReasons, "CAPTCHA detected: "+pattern.String())
		}
	}
	
	// Check for HTTP error patterns
	for _, pattern := range bd.blockPatterns {
		if pattern.MatchString(content) {
			botScore += 0.4
			botReasons = append(botReasons, "HTTP error: "+pattern.String())
		}
	}
	
	// Additional heuristics
	if strings.Contains(content, "javascript") && strings.Contains(content, "disabled") {
		botScore += 0.2
		botReasons = append(botReasons, "JavaScript disabled warning")
	}
	
	if len(content) < 1000 && botScore > 0 {
		botScore += 0.2
		botReasons = append(botReasons, "Very short content with bot indicators")
	}
	
	// Cap the score at 1.0
	if botScore > 1.0 {
		botScore = 1.0
	}
	
	isBotWall := botScore > 0.3
	reason := strings.Join(botReasons, "; ")
	
	return isBotWall, reason, botScore
}

// DetectCaptcha specifically checks for CAPTCHA challenges
func (bd *BotDetector) DetectCaptcha(pageContent, pageTitle string) (bool, string) {
	content := strings.ToLower(pageContent + " " + pageTitle)
	
	for _, pattern := range bd.captchaPatterns {
		if pattern.MatchString(content) {
			return true, "CAPTCHA detected: " + pattern.String()
		}
	}
	
	return false, ""
}

// GetBlockType determines the type of blocking
func (bd *BotDetector) GetBlockType(pageContent, pageTitle string) string {
	content := strings.ToLower(pageContent + " " + pageTitle)
	
	if isCaptcha, _ := bd.DetectCaptcha(pageContent, pageTitle); isCaptcha {
		return "captcha"
	}
	
	for _, pattern := range bd.blockPatterns {
		if pattern.MatchString(content) {
			return "http_error"
		}
	}
	
	return "bot_wall"
}
