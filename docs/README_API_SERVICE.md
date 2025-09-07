# Distrack API Service

This document explains how to use Distrack as an API service with versioning, rate limiting, and comprehensive documentation.

## Features

- **API Versioning**: All endpoints are now under `/api/v1/` with backward compatibility
- **Rate Limiting**: Tiered rate limiting based on subscription plans
- **API Key Authentication**: Secure API key-based authentication
- **Comprehensive Documentation**: Full API documentation with examples
- **Subscription Plans**: Free, Basic, Pro, and Enterprise tiers
- **Usage Tracking**: Monitor API usage and limits

## Quick Start

### 1. Test API Keys

The service comes with pre-configured test API keys for each plan:

```
Free Plan:     test_free_key_12345
Basic Plan:    test_basic_key_12345  
Pro Plan:      test_pro_key_12345
Enterprise:    test_enterprise_key_12345
```

### 2. Make Your First API Call

```bash
# Health check (no API key required)
curl http://localhost:8080/health

# Get tracked URLs (API key required)
curl -H "Authorization: Bearer test_pro_key_12345" \
     http://localhost:8080/api/v1/urls
```

### 3. Add a URL to Track

```bash
curl -X POST "http://localhost:8080/api/v1/urls" \
  -H "Authorization: Bearer test_pro_key_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/product",
    "name": "Test Product"
  }'
```

## API Endpoints

### Base URLs

- **Current Version**: `/api/v1/`
- **Legacy (Deprecated)**: `/api/` (redirects to v1)
- **Documentation**: `/docs`
- **Health Check**: `/health`

### Authentication

Include your API key in one of these ways:

```bash
# Header (Recommended)
Authorization: Bearer YOUR_API_KEY

# Alternative header
X-API-Key: YOUR_API_KEY

# Query parameter
?api_key=YOUR_API_KEY
```

### Rate Limits

| Plan | Rate Limit | Daily Limit | Max URLs | Max Alerts |
|------|------------|-------------|----------|------------|
| Free | 60 req/min | 1,000 req/day | 10 | 5 |
| Basic | 300 req/min | 50,000 req/day | 100 | 50 |
| Pro | 1,000 req/min | 200,000 req/day | 1,000 | 500 |
| Enterprise | 5,000 req/min | 1,000,000 req/day | 10,000 | 5,000 |

## Subscription Plans

### Free Plan
- Basic price tracking
- Price alerts
- Price history
- Email notifications
- **Price**: $0/month

### Basic Plan ($29.99/month)
- Everything in Free
- Webhook support
- Priority support
- Advanced analytics
- Bulk operations

### Pro Plan ($99.99/month)
- Everything in Basic
- Priority processing
- Custom webhook endpoints
- Advanced filtering
- Data export
- API usage analytics

### Enterprise Plan ($299.99/month)
- Everything in Pro
- Custom rate limits
- Dedicated support
- Custom integrations
- White-label options
- SLA guarantees

## Environment Variables

Configure the API service using these environment variables:

```bash
# API Configuration
API_BASE_URL=https://api.distrack.com
API_REQUIRE_KEY=true
API_RATE_LIMIT_ENABLED=true
API_LOGGING_ENABLED=true
API_CORS_ENABLED=true
API_MAX_REQUEST_SIZE=10485760  # 10MB
API_REQUEST_TIMEOUT=30s

# CORS
ALLOWED_ORIGINS=http://localhost:3000,https://yourdomain.com

# Server
PORT=8080
HOST=0.0.0.0
```

## Usage Examples

### Python

```python
import requests

API_KEY = "test_pro_key_12345"
BASE_URL = "http://localhost:8080/api/v1"

headers = {
    "Authorization": f"Bearer {API_KEY}",
    "Content-Type": "application/json"
}

# Add URL to track
response = requests.post(
    f"{BASE_URL}/urls",
    headers=headers,
    json={
        "url": "https://example.com/product",
        "name": "Product Name"
    }
)

# Check price
response = requests.post(
    f"{BASE_URL}/urls/123/check",
    headers=headers
)
```

### JavaScript

```javascript
const API_KEY = "test_pro_key_12345";
const BASE_URL = "http://localhost:8080/api/v1";

const headers = {
    "Authorization": `Bearer ${API_KEY}`,
    "Content-Type": "application/json"
};

// Add URL to track
fetch(`${BASE_URL}/urls`, {
    method: "POST",
    headers,
    body: JSON.stringify({
        url: "https://example.com/product",
        name: "Product Name"
    })
});

// Check price
fetch(`${BASE_URL}/urls/123/check`, {
    method: "POST",
    headers
});
```

### cURL

```bash
# Get all tracked URLs
curl -H "Authorization: Bearer test_pro_key_12345" \
     http://localhost:8080/api/v1/urls

# Check price for a specific URL
curl -X POST \
     -H "Authorization: Bearer test_pro_key_12345" \
     http://localhost:8080/api/v1/urls/123/check

# Get price history
curl -H "Authorization: Bearer test_pro_key_12345" \
     http://localhost:8080/api/v1/urls/123/history
```

## Error Handling

The API returns standard HTTP status codes and detailed error messages:

```json
{
  "error": "Rate limit exceeded",
  "code": "RATE_LIMIT_EXCEEDED",
  "details": "You have exceeded your rate limit of 60 requests per minute"
}
```

Common status codes:
- `200` - Success
- `201` - Created
- `400` - Bad Request
- `401` - Unauthorized
- `429` - Rate Limit Exceeded
- `500` - Internal Server Error

## Rate Limit Headers

All responses include rate limit information:

```
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 45
X-RateLimit-Reset: 2024-01-15T11:00:00Z
```

## Migration from Legacy API

If you're using the old `/api/*` endpoints, they will automatically redirect to `/api/v1/*`. You'll see deprecation warnings in the response headers:

```
X-API-Deprecation-Warning: This endpoint is deprecated. Please use /api/v1 endpoints instead.
X-API-Version: v1
```

## Monitoring and Health

### Health Check

```bash
curl http://localhost:8080/health
```

Returns service status, API version, and rate limit information.

### Metrics

```bash
curl http://localhost:8080/metrics
```

Returns system metrics including memory usage, goroutines, and active URLs.

### Status

```bash
curl http://localhost:8080/status
```

Returns detailed system status including URL counts and success rates.

## Development

### Running the Service

```bash
cd backend
go mod tidy  # Install new dependencies
go run main.go
```

### Testing API Keys

Use the test keys provided above to test different subscription tiers and rate limits.

### Adding New Endpoints

1. Add the handler function to `handlers/handlers.go`
2. Add the route to `main.go` under the appropriate version
3. Update the API documentation in `docs/api_documentation.md`

## Production Deployment

### Security Considerations

1. **API Key Storage**: Store API keys securely in a database
2. **Rate Limiting**: Implement proper rate limiting per user/plan
3. **Logging**: Log all API requests for monitoring and debugging
4. **CORS**: Configure CORS properly for your domain
5. **HTTPS**: Always use HTTPS in production

### Scaling

1. **Database**: Use a production database (PostgreSQL, MySQL)
2. **Caching**: Implement Redis for rate limiting and caching
3. **Load Balancing**: Use a load balancer for multiple instances
4. **Monitoring**: Implement proper monitoring and alerting

### Environment Variables

Set these in your production environment:

```bash
API_BASE_URL=https://api.yourdomain.com
API_REQUIRE_KEY=true
API_RATE_LIMIT_ENABLED=true
ALLOWED_ORIGINS=https://yourdomain.com
```

## Support

For questions and support:
- Check the API documentation at `/docs`
- Review the health endpoint for service status
- Check the logs for detailed error information

## Changelog

### v1.0.0 (2024-01-15)
- Initial API service release
- API versioning with v1 endpoints
- Rate limiting based on subscription plans
- API key authentication
- Comprehensive documentation
- Backward compatibility with legacy endpoints

