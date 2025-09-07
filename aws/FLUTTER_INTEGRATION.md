# Flutter App Integration with Distrack API

This guide explains how to integrate your Flutter app with the deployed Distrack API service.

## API Endpoints

Your Flutter app can connect to the Distrack API using these endpoints:

### Base URL
```
https://YOUR_LOAD_BALANCER_DNS
```

### Key Endpoints

#### 1. Health Check (No Auth Required)
```dart
GET /health
```

#### 2. Add URL to Track
```dart
POST /api/v1/urls
Headers: {
  'Authorization': 'Bearer YOUR_API_KEY',
  'Content-Type': 'application/json'
}
Body: {
  'url': 'https://example.com/product',
  'name': 'Product Name'
}
```

#### 3. Get Tracked URLs
```dart
GET /api/v1/urls
Headers: {
  'Authorization': 'Bearer YOUR_API_KEY'
}
```

#### 4. Check Price Now
```dart
POST /api/v1/urls/{id}/check
Headers: {
  'Authorization': 'Bearer YOUR_API_KEY'
}
```

#### 5. Get Price History
```dart
GET /api/v1/urls/{id}/history
Headers: {
  'Authorization': 'Bearer YOUR_API_KEY'
}
```

## Flutter HTTP Client Setup

### 1. Add Dependencies

Add to your `pubspec.yaml`:

```yaml
dependencies:
  http: ^1.1.0
  dio: ^5.3.2  # Alternative to http package
```

### 2. API Service Class

Create an API service class:

```dart
import 'package:http/http.dart' as http;
import 'dart:convert';

class DistrackApiService {
  static const String baseUrl = 'https://YOUR_LOAD_BALANCER_DNS';
  static const String apiKey = 'test_pro_key_12345'; // Use your API key
  
  static Map<String, String> get _headers => {
    'Authorization': 'Bearer $apiKey',
    'Content-Type': 'application/json',
  };

  // Health check
  static Future<bool> checkHealth() async {
    try {
      final response = await http.get(
        Uri.parse('$baseUrl/health'),
        headers: {'Content-Type': 'application/json'},
      );
      return response.statusCode == 200;
    } catch (e) {
      return false;
    }
  }

  // Add URL to track
  static Future<Map<String, dynamic>?> addUrlToTrack({
    required String url,
    required String name,
  }) async {
    try {
      final response = await http.post(
        Uri.parse('$baseUrl/api/v1/urls'),
        headers: _headers,
        body: json.encode({
          'url': url,
          'name': name,
        }),
      );
      
      if (response.statusCode == 201) {
        return json.decode(response.body);
      }
      return null;
    } catch (e) {
      return null;
    }
  }

  // Get tracked URLs
  static Future<List<dynamic>?> getTrackedUrls() async {
    try {
      final response = await http.get(
        Uri.parse('$baseUrl/api/v1/urls'),
        headers: _headers,
      );
      
      if (response.statusCode == 200) {
        return json.decode(response.body);
      }
      return null;
    } catch (e) {
      return null;
    }
  }

  // Check price now
  static Future<Map<String, dynamic>?> checkPriceNow(int urlId) async {
    try {
      final response = await http.post(
        Uri.parse('$baseUrl/api/v1/urls/$urlId/check'),
        headers: _headers,
      );
      
      if (response.statusCode == 200) {
        return json.decode(response.body);
      }
      return null;
    } catch (e) {
      return null;
    }
  }

  // Get price history
  static Future<List<dynamic>?> getPriceHistory(int urlId) async {
    try {
      final response = await http.get(
        Uri.parse('$baseUrl/api/v1/urls/$urlId/history'),
        headers: _headers,
      );
      
      if (response.statusCode == 200) {
        return json.decode(response.body);
      }
      return null;
    } catch (e) {
      return null;
    }
  }
}
```

### 3. Usage Example

```dart
import 'package:flutter/material.dart';
import 'distrack_api_service.dart';

class PriceTrackerScreen extends StatefulWidget {
  @override
  _PriceTrackerScreenState createState() => _PriceTrackerScreenState();
}

class _PriceTrackerScreenState extends State<PriceTrackerScreen> {
  List<dynamic> trackedUrls = [];
  bool isLoading = false;

  @override
  void initState() {
    super.initState();
    _loadTrackedUrls();
  }

  Future<void> _loadTrackedUrls() async {
    setState(() => isLoading = true);
    
    final urls = await DistrackApiService.getTrackedUrls();
    if (urls != null) {
      setState(() {
        trackedUrls = urls;
        isLoading = false;
      });
    }
  }

  Future<void> _addUrlToTrack() async {
    final result = await DistrackApiService.addUrlToTrack(
      url: 'https://example.com/product',
      name: 'Test Product',
    );
    
    if (result != null) {
      _loadTrackedUrls(); // Refresh the list
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('URL added successfully!')),
      );
    }
  }

  Future<void> _checkPrice(int urlId) async {
    final result = await DistrackApiService.checkPriceNow(urlId);
    if (result != null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Price checked: \$${result['price_data']['current_price']}')),
      );
      _loadTrackedUrls(); // Refresh to get updated price
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('Price Tracker')),
      body: isLoading
          ? Center(child: CircularProgressIndicator())
          : ListView.builder(
              itemCount: trackedUrls.length,
              itemBuilder: (context, index) {
                final url = trackedUrls[index];
                return ListTile(
                  title: Text(url['name']),
                  subtitle: Text('Current: \$${url['current_price']}'),
                  trailing: ElevatedButton(
                    onPressed: () => _checkPrice(url['id']),
                    child: Text('Check Price'),
                  ),
                );
              },
            ),
      floatingActionButton: FloatingActionButton(
        onPressed: _addUrlToTrack,
        child: Icon(Icons.add),
      ),
    );
  }
}
```

## API Key Management

### Test API Keys (for development)
```
Free Plan:     test_free_key_12345
Basic Plan:    test_basic_key_12345  
Pro Plan:      test_pro_key_12345
Enterprise:    test_enterprise_key_12345
```

### Generate New API Key
```dart
static Future<String?> generateApiKey({
  required String userId,
  String plan = 'free',
}) async {
  try {
    final response = await http.post(
      Uri.parse('$baseUrl/api/v1/api-keys/generate'),
      headers: _headers,
      body: json.encode({
        'user_id': userId,
        'plan': plan,
      }),
    );
    
    if (response.statusCode == 201) {
      final data = json.decode(response.body);
      return data['api_key'];
    }
    return null;
  } catch (e) {
    return null;
  }
}
```

## Error Handling

```dart
class ApiException implements Exception {
  final String message;
  final int? statusCode;
  
  ApiException(this.message, [this.statusCode]);
  
  @override
  String toString() => 'ApiException: $message (Status: $statusCode)';
}

// Usage in API calls
static Future<Map<String, dynamic>?> addUrlToTrack({
  required String url,
  required String name,
}) async {
  try {
    final response = await http.post(
      Uri.parse('$baseUrl/api/v1/urls'),
      headers: _headers,
      body: json.encode({
        'url': url,
        'name': name,
      }),
    );
    
    if (response.statusCode == 201) {
      return json.decode(response.body);
    } else if (response.statusCode == 401) {
      throw ApiException('Invalid API key', 401);
    } else if (response.statusCode == 400) {
      throw ApiException('Invalid request data', 400);
    } else {
      throw ApiException('Server error', response.statusCode);
    }
  } catch (e) {
    if (e is ApiException) rethrow;
    throw ApiException('Network error: ${e.toString()}');
  }
}
```

## CORS Configuration

The API is configured with `ALLOWED_ORIGINS=*` to allow requests from:
- Flutter web apps
- Flutter mobile apps (via HTTP client)
- Flutter desktop apps
- Any other client

## Rate Limits

Based on your API key plan:

| Plan | Rate Limit | Daily Limit |
|------|------------|-------------|
| Free | 60 req/min | 1,000 req/day |
| Basic | 300 req/min | 50,000 req/day |
| Pro | 1,000 req/min | 200,000 req/day |
| Enterprise | 5,000 req/min | 1,000,000 req/day |

## Testing the Integration

1. **Health Check Test**:
```dart
final isHealthy = await DistrackApiService.checkHealth();
print('API is healthy: $isHealthy');
```

2. **Add URL Test**:
```dart
final result = await DistrackApiService.addUrlToTrack(
  url: 'https://www.amazon.com/dp/B08N5WRWNW',
  name: 'Test Product',
);
print('Added URL: $result');
```

3. **Get URLs Test**:
```dart
final urls = await DistrackApiService.getTrackedUrls();
print('Tracked URLs: $urls');
```

## Security Considerations

1. **API Key Storage**: Store API keys securely using Flutter's secure storage
2. **HTTPS**: Always use HTTPS in production
3. **Input Validation**: Validate URLs and names before sending to API
4. **Error Handling**: Implement proper error handling for network issues

## Troubleshooting

### Common Issues

1. **CORS Errors**: The API is configured to allow all origins (`*`)
2. **Authentication Errors**: Check your API key is correct
3. **Network Errors**: Ensure the load balancer DNS is accessible
4. **Rate Limiting**: Check if you've exceeded your plan limits

### Debug Mode

Enable debug logging in your Flutter app:

```dart
import 'package:http/http.dart' as http;

// Enable debug logging
http.Client().interceptors.add(LogInterceptor(
  requestBody: true,
  responseBody: true,
));
```
