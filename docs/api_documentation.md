# Distrack API Documentation

## Overview

Distrack is a price tracking and monitoring service that allows you to track product prices across various e-commerce platforms. This API provides endpoints for managing tracked URLs, monitoring prices, and setting up price alerts.

## Base URL

```
https://api.distrack.com/v1
```

## Authentication

All API requests require authentication using an API key. You can include your API key in one of the following ways:

### Header Authentication (Recommended)
```
Authorization: Bearer YOUR_API_KEY
```
or
```
X-API-Key: YOUR_API_KEY
```

### Query Parameter
```
?api_key=YOUR_API_KEY
```

## Rate Limiting

Rate limits are applied per API key and subscription plan:

| Plan | Rate Limit | Daily Limit | Description |
|------|------------|-------------|-------------|
| Free | 60 req/min | 1,000 req/day | Basic usage |
| Basic | 300 req/min | 50,000 req/day | Small business |
| Pro | 1,000 req/min | 200,000 req/day | Growing business |
| Enterprise | 5,000 req/min | 1,000,000 req/day | High volume |

Rate limit headers are included in all responses:
- `X-RateLimit-Limit`: Maximum requests per window
- `X-RateLimit-Remaining`: Remaining requests in current window
- `X-RateLimit-Reset`: Time when the rate limit resets

## Error Handling

The API uses standard HTTP status codes and returns error details in JSON format:

```json
{
  "error": "Error message description",
  "code": "ERROR_CODE",
  "details": "Additional error details"
}
```

Common HTTP status codes:
- `200` - Success
- `201` - Created
- `400` - Bad Request
- `401` - Unauthorized
- `403` - Forbidden
- `404` - Not Found
- `429` - Rate Limit Exceeded
- `500` - Internal Server Error

## Endpoints

### Health Check

#### GET /health
Check the health status of the API service.

**Response:**
```json
{
  "service": "distrack",
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "version": "2.0.0",
  "api_version": "v1"
}
```

### URL Management

#### POST /v1/urls
Add a new URL to track.

**Request Body:**
```json
{
  "url": "https://example.com/product",
  "name": "Product Name"
}
```

**Response:**
```json
{
  "id": 123,
  "url": "https://example.com/product",
  "name": "Product Name",
  "created_at": "2024-01-15T10:30:00Z",
  "current_price": 99.99,
  "currency": "USD"
}
```

#### GET /v1/urls
Get all tracked URLs.

**Query Parameters:**
- `limit` (optional): Maximum number of URLs to return (default: 50, max: 100)

**Response:**
```json
[
  {
    "id": 123,
    "url": "https://example.com/product",
    "name": "Product Name",
    "current_price": 99.99,
    "currency": "USD",
    "last_checked": "2024-01-15T10:30:00Z",
    "status": "active"
  }
]
```

#### GET /v1/urls/{id}
Get details for a specific tracked URL.

**Response:**
```json
{
  "id": 123,
  "url": "https://example.com/product",
  "name": "Product Name",
  "current_price": 99.99,
  "original_price": 129.99,
  "currency": "USD",
  "discount_percentage": 23.1,
  "is_on_sale": true,
  "last_checked": "2024-01-15T10:30:00Z",
  "status": "active"
}
```

#### DELETE /v1/urls/{id}
Delete a tracked URL.

**Response:**
```json
{
  "message": "URL deleted successfully"
}
```

### Price Checking

#### POST /v1/urls/{id}/check
Check the current price for a tracked URL.

**Response:**
```json
{
  "price_data": {
    "current_price": 99.99,
    "original_price": 129.99,
    "currency": "USD",
    "discount_percentage": 23.1,
    "is_on_sale": true,
    "extraction_method": "hybrid",
    "confidence": 0.95
  },
  "checked_at": "2024-01-15T10:30:00Z",
  "price_validation": {
    "is_realistic": true,
    "change_reason": "price_drop",
    "old_price": 129.99,
    "price_change_percent": -23.1
  }
}
```

#### POST /v1/urls/{id}/check-async
Start an asynchronous price check.

**Response:**
```json
{
  "task_id": "task_abc123",
  "status": "queued",
  "message": "Price check queued for processing",
  "url_id": 123,
  "url_name": "Product Name"
}
```

#### GET /v1/tasks/{taskId}
Get the status of an asynchronous task.

**Response:**
```json
{
  "id": "task_abc123",
  "status": "completed",
  "result": {
    "price_data": {
      "current_price": 99.99,
      "currency": "USD"
    }
  },
  "created_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:31:00Z"
}
```

### Price History

#### GET /v1/urls/{id}/history
Get price history for a tracked URL.

**Query Parameters:**
- `limit` (optional): Maximum number of history entries (default: 50, max: 100)

**Response:**
```json
[
  {
    "id": 456,
    "url_id": 123,
    "price": 99.99,
    "currency": "USD",
    "checked_at": "2024-01-15T10:30:00Z",
    "extraction_method": "hybrid",
    "confidence": 0.95
  }
]
```

### Price Alerts

#### POST /v1/urls/{id}/alerts
Create a new price alert.

**Request Body:**
```json
{
  "alert_type": "price_drop",
  "target_price": 89.99,
  "percentage": 10.0
}
```

**Alert Types:**
- `price_drop`: Alert when price drops below target price
- `percentage_drop`: Alert when price drops by specified percentage

**Response:**
```json
{
  "id": 789,
  "url_id": 123,
  "alert_type": "price_drop",
  "target_price": 89.99,
  "percentage": 10.0,
  "is_active": true,
  "created_at": "2024-01-15T10:30:00Z"
}
```

#### GET /v1/urls/{id}/alerts
Get all price alerts for a tracked URL.

**Response:**
```json
[
  {
    "id": 789,
    "url_id": 123,
    "alert_type": "price_drop",
    "target_price": 89.99,
    "percentage": 10.0,
    "is_active": true,
    "created_at": "2024-01-15T10:30:00Z"
  }
]
```

#### DELETE /v1/urls/{id}/alerts/{alertId}
Delete a price alert.

**Response:**
```json
{
  "message": "Alert deleted successfully"
}
```

### Task Management

#### GET /v1/tasks/stats
Get statistics about the task manager.

**Response:**
```json
{
  "stats": {
    "total_tasks": 150,
    "completed_tasks": 120,
    "failed_tasks": 5,
    "queued_tasks": 25,
    "active_workers": 3
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Debug Endpoints

#### POST /v1/debug/screenshot
Take a screenshot of a URL for debugging purposes.

**Request Body:**
```json
{
  "url": "https://example.com/product"
}
```

**Response:**
```json
{
  "success": true,
  "screenshot": "base64_encoded_image_data",
  "url": "https://example.com/product",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

## SDKs and Libraries

### cURL Examples

#### Add a URL to track
```bash
curl -X POST "https://api.distrack.com/v1/urls" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/product",
    "name": "Product Name"
  }'
```

#### Check price
```bash
curl -X POST "https://api.distrack.com/v1/urls/123/check" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

#### Get tracked URLs
```bash
curl -X GET "https://api.distrack.com/v1/urls" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

### Python Example
```python
import requests

API_KEY = "YOUR_API_KEY"
BASE_URL = "https://api.distrack.com/v1"

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

### JavaScript Example
```javascript
const API_KEY = "YOUR_API_KEY";
const BASE_URL = "https://api.distrack.com/v1";

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

## Webhooks

Webhooks allow you to receive real-time notifications when price alerts are triggered. Configure webhooks in your account settings.

**Webhook Payload:**
```json
{
  "event": "price_alert_triggered",
  "url_id": 123,
  "url_name": "Product Name",
  "current_price": 89.99,
  "target_price": 89.99,
  "alert_type": "price_drop",
  "triggered_at": "2024-01-15T10:30:00Z"
}
```

## Support

For API support and questions:
- Email: api-support@distrack.com
- Documentation: https://docs.distrack.com
- Status page: https://status.distrack.com

## Changelog

### v1.0.0 (2024-01-15)
- Initial API release
- URL tracking and management
- Price checking and monitoring
- Price alerts and notifications
- Asynchronous task processing

