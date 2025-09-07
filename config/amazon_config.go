package config

import (
	"os"
)

// AmazonConfig holds Amazon API configuration
type AmazonConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	AssociateTag    string
	Region          string
	PartnerTag      string
	Enabled         bool
}

// LoadAmazonConfig loads Amazon configuration from environment variables
func LoadAmazonConfig() *AmazonConfig {
	return &AmazonConfig{
		AccessKeyID:     os.Getenv("AMAZON_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AMAZON_SECRET_ACCESS_KEY"),
		AssociateTag:    os.Getenv("AMAZON_ASSOCIATE_TAG"),
		Region:          getEnvOrDefault("AMAZON_REGION", "us-east-1"),
		PartnerTag:      getEnvOrDefault("AMAZON_PARTNER_TAG", "distrack-20"),
		Enabled:         getEnvOrDefault("AMAZON_API_ENABLED", "true") == "true",
	}
}

// IsValid checks if the Amazon configuration is valid
func (c *AmazonConfig) IsValid() bool {
	if !c.Enabled {
		return false
	}
	
	return c.AccessKeyID != "" && 
		   c.SecretAccessKey != "" && 
		   c.AssociateTag != "" && 
		   c.Region != ""
}

// getEnvOrDefault gets environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
