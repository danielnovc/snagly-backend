# Flutter App Endpoint Configuration

This guide shows you exactly how to configure your Flutter app to connect to your deployed Snagly API.

## 🚀 **After Deployment - Get Your API Endpoint**

Once you deploy your infrastructure, you'll get a load balancer DNS name. This will be your API endpoint.

### Get Your Endpoint

```bash
# After deployment, get your load balancer DNS
aws cloudformation describe-stacks \
    --stack-name snagly-stack \
    --region eu-north-1 \
    --query 'Stacks[0].Outputs[?OutputKey==`LoadBalancerDNS`].OutputValue' \
    --output text
```

**Example output**: `snagly-alb-1234567890.eu-north-1.elb.amazonaws.com`

## 📱 **Flutter App Configuration**

### 1. Update Your API Service Class

Replace the base URL in your Flutter app:

```dart
class SnaglyApiService {
  // Replace this with your actual load balancer DNS
  static const String baseUrl = 'http://YOUR_LOAD_BALANCER_DNS';
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

### 2. Environment Configuration (Recommended)

Create a configuration file for different environments:

```dart
// lib/config/api_config.dart
class ApiConfig {
  static const String _baseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: 'http://YOUR_LOAD_BALANCER_DNS', // Replace with your actual DNS
  );
  
  static const String _apiKey = String.fromEnvironment(
    'API_KEY',
    defaultValue: 'test_pro_key_12345',
  );
  
  static String get baseUrl => _baseUrl;
  static String get apiKey => _apiKey;
}
```

Then use it in your API service:

```dart
class SnaglyApiService {
  static const String baseUrl = ApiConfig.baseUrl;
  static const String apiKey = ApiConfig.apiKey;
  
  // ... rest of your methods
}
```

### 3. Build with Environment Variables

```bash
# Development build
flutter run --dart-define=API_BASE_URL=http://YOUR_LOAD_BALANCER_DNS

# Production build
flutter build apk --dart-define=API_BASE_URL=http://YOUR_LOAD_BALANCER_DNS
```

## 🔧 **Complete Flutter Integration Example**

Here's a complete example of how to integrate with your Snagly API:

```dart
// lib/services/snagly_api_service.dart
import 'package:http/http.dart' as http;
import 'dart:convert';

class SnaglyApiService {
  // Replace with your actual load balancer DNS
  static const String baseUrl = 'http://YOUR_LOAD_BALANCER_DNS';
  static const String apiKey = 'test_pro_key_12345';
  
  static Map<String, String> get _headers => {
    'Authorization': 'Bearer $apiKey',
    'Content-Type': 'application/json',
  };

  // Test connection
  static Future<bool> testConnection() async {
    try {
      final response = await http.get(
        Uri.parse('$baseUrl/health'),
        headers: {'Content-Type': 'application/json'},
      );
      
      if (response.statusCode == 200) {
        final data = json.decode(response.body);
        print('✅ API is healthy: ${data['status']}');
        return true;
      }
      return false;
    } catch (e) {
      print('❌ API connection failed: $e');
      return false;
    }
  }

  // Add URL to track
  static Future<Map<String, dynamic>?> addUrlToTrack({
    required String url,
    required String name,
  }) async {
    try {
      print('📝 Adding URL to track: $name');
      final response = await http.post(
        Uri.parse('$baseUrl/api/v1/urls'),
        headers: _headers,
        body: json.encode({
          'url': url,
          'name': name,
        }),
      );
      
      if (response.statusCode == 201) {
        final result = json.decode(response.body);
        print('✅ URL added successfully: ID ${result['id']}');
        return result;
      } else {
        print('❌ Failed to add URL: ${response.statusCode} - ${response.body}');
        return null;
      }
    } catch (e) {
      print('❌ Error adding URL: $e');
      return null;
    }
  }

  // Get all tracked URLs
  static Future<List<dynamic>?> getTrackedUrls() async {
    try {
      print('📋 Fetching tracked URLs...');
      final response = await http.get(
        Uri.parse('$baseUrl/api/v1/urls'),
        headers: _headers,
      );
      
      if (response.statusCode == 200) {
        final urls = json.decode(response.body);
        print('✅ Found ${urls.length} tracked URLs');
        return urls;
      } else {
        print('❌ Failed to fetch URLs: ${response.statusCode}');
        return null;
      }
    } catch (e) {
      print('❌ Error fetching URLs: $e');
      return null;
    }
  }

  // Check price for a specific URL
  static Future<Map<String, dynamic>?> checkPriceNow(int urlId) async {
    try {
      print('💰 Checking price for URL ID: $urlId');
      final response = await http.post(
        Uri.parse('$baseUrl/api/v1/urls/$urlId/check'),
        headers: _headers,
      );
      
      if (response.statusCode == 200) {
        final result = json.decode(response.body);
        final priceData = result['price_data'];
        print('✅ Price check successful: \$${priceData['current_price']}');
        return result;
      } else {
        print('❌ Price check failed: ${response.statusCode}');
        return null;
      }
    } catch (e) {
      print('❌ Error checking price: $e');
      return null;
    }
  }

  // Get price history
  static Future<List<dynamic>?> getPriceHistory(int urlId) async {
    try {
      print('📊 Fetching price history for URL ID: $urlId');
      final response = await http.get(
        Uri.parse('$baseUrl/api/v1/urls/$urlId/history'),
        headers: _headers,
      );
      
      if (response.statusCode == 200) {
        final history = json.decode(response.body);
        print('✅ Found ${history.length} price history entries');
        return history;
      } else {
        print('❌ Failed to fetch price history: ${response.statusCode}');
        return null;
      }
    } catch (e) {
      print('❌ Error fetching price history: $e');
      return null;
    }
  }
}
```

## 🧪 **Testing Your Integration**

### 1. Test API Connection

```dart
// Test in your Flutter app
void testApiConnection() async {
  final isHealthy = await SnaglyApiService.testConnection();
  if (isHealthy) {
    print('🎉 API is working!');
  } else {
    print('❌ API connection failed');
  }
}
```

### 2. Test Adding a URL

```dart
void testAddUrl() async {
  final result = await SnaglyApiService.addUrlToTrack(
    url: 'https://www.amazon.com/dp/B08N5WRWNW',
    name: 'Test Product',
  );
  
  if (result != null) {
    print('✅ URL added: ${result['id']}');
  }
}
```

### 3. Test Getting URLs

```dart
void testGetUrls() async {
  final urls = await SnaglyApiService.getTrackedUrls();
  if (urls != null) {
    for (var url in urls) {
      print('📋 ${url['name']}: \$${url['current_price']}');
    }
  }
}
```

## 🔒 **Security Notes**

1. **API Key**: Store your API key securely in your Flutter app
2. **HTTPS**: Consider setting up SSL certificate for production
3. **Rate Limiting**: Be aware of API rate limits
4. **Error Handling**: Always handle network errors gracefully

## 📊 **Monitoring Your API**

### Check API Health
```bash
curl http://YOUR_LOAD_BALANCER_DNS/health
```

### Check API Status
```bash
curl http://YOUR_LOAD_BALANCER_DNS/status
```

### View Logs
```bash
aws logs tail /ecs/snagly --follow --region eu-north-1
```

## 🚀 **Next Steps**

1. **Deploy your infrastructure** using the deployment script
2. **Get your load balancer DNS** from CloudFormation outputs
3. **Update your Flutter app** with the correct base URL
4. **Test the integration** using the provided examples
5. **Monitor your API** using AWS CloudWatch logs

Your Flutter app will now be able to communicate with your deployed Snagly API! 🎉
