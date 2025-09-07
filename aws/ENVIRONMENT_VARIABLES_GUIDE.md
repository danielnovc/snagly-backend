# Environment Variables Management in AWS

This guide explains how environment variables and secrets are managed in your Snagly AWS deployment.

## 🔐 **Security Strategy**

### **Sensitive Data (Secrets)**
- **Storage**: AWS Systems Manager Parameter Store (encrypted)
- **Access**: ECS Task Execution Role with IAM permissions
- **Encryption**: AWS KMS (automatic for SecureString parameters)

### **Non-sensitive Configuration**
- **Storage**: ECS Task Definition Environment Variables
- **Access**: Direct container access
- **Encryption**: Not needed (non-sensitive)

## 📋 **Environment Variables Breakdown**

### **🔒 Secrets (SSM Parameter Store)**

| Variable | Parameter Path | Type | Description |
|----------|----------------|------|-------------|
| `DATABASE_URL` | `/snagly/database-url` | SecureString | PostgreSQL connection string |
| `SUPABASE_URL` | `/snagly/supabase-url` | String | Supabase project URL |
| `SUPABASE_ANON_KEY` | `/snagly/supabase-anon-key` | SecureString | Supabase anonymous key |

### **⚙️ Configuration (ECS Task Definition)**

| Variable | Value | Description |
|----------|-------|-------------|
| `OCR_SERVICE_URL` | `http://localhost:5000` | Internal OCR service URL |
| `YOLO_SERVICE_URL` | `http://localhost:8000` | Internal YOLO service URL |
| `HOST` | `0.0.0.0` | Server bind address |
| `PORT` | `8080` | Server port |
| `ALLOWED_ORIGINS` | `*` | CORS allowed origins |
| `AWS_REGION` | `eu-north-1` | AWS region |
| `ECS_CLUSTER_NAME` | `snagly-cluster` | ECS cluster name |
| `ECS_SERVICE_NAME` | `snagly-service` | ECS service name |
| `LOG_LEVEL` | `info` | Logging level |
| `LOG_FORMAT` | `json` | Log format |
| `MAX_WORKERS` | `5` | Maximum worker threads |
| `REQUEST_TIMEOUT` | `300` | Request timeout in seconds |
| `HEALTH_CHECK_INTERVAL` | `30` | Health check interval |
| `PRICE_CHECK_INTERVAL` | `0 */12 * * *` | Cron expression for price checks |
| `MAX_CONCURRENT_CHECKS` | `5` | Maximum concurrent price checks |

## 🚀 **Deployment Process**

### **Step 1: Create SSM Parameters**

```bash
# Run the environment variables management script
cd backend/aws
./manage-env-vars.sh create
```

This creates all the required SSM parameters with your actual values.

### **Step 2: Deploy Infrastructure**

```bash
# Deploy the CloudFormation stack
./deploy-snagly.sh
```

The CloudFormation template will:
- Create SSM parameters (if they don't exist)
- Set up IAM roles with proper permissions
- Configure ECS task definition with environment variables and secrets

### **Step 3: Verify Deployment**

```bash
# Check if parameters are accessible
./manage-env-vars.sh test

# List all parameters
./manage-env-vars.sh list
```

## 🔧 **Managing Environment Variables**

### **View Current Configuration**

```bash
./manage-env-vars.sh show-config
```

### **List All Parameters**

```bash
./manage-env-vars.sh list
```

### **Get Parameter Value**

```bash
# Get non-sensitive parameter
./manage-env-vars.sh get-param supabase-url

# For sensitive parameters, use AWS CLI directly
aws ssm get-parameter --name "/snagly/database-url" --with-decryption --region eu-north-1
```

### **Update Parameter**

```bash
# Update non-sensitive parameter
./manage-env-vars.sh update-param supabase-url "https://new-url.supabase.co"

# Update sensitive parameter
./manage-env-vars.sh update-param database-url "postgresql://new-connection-string" SecureString
```

### **Delete Parameter**

```bash
./manage-env-vars.sh delete-param parameter-name
```

## 🔑 **IAM Permissions**

### **ECS Task Execution Role**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameters",
        "ssm:GetParameter"
      ],
      "Resource": "arn:aws:ssm:eu-north-1:797926358985:parameter/snagly/*"
    }
  ]
}
```

### **ECS Task Role**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:eu-north-1:797926358985:log-group:/ecs/snagly:*"
    }
  ]
}
```

## 🔍 **How It Works in Your Application**

### **Go Backend (main.go)**
```go
// Environment variables are automatically injected by ECS
// Secrets are retrieved from SSM Parameter Store at runtime

func main() {
    // These are available as environment variables
    port := os.Getenv("PORT")
    host := os.Getenv("HOST")
    logLevel := os.Getenv("LOG_LEVEL")
    
    // These are retrieved from SSM Parameter Store
    databaseURL := os.Getenv("DATABASE_URL")  // Injected by ECS
    supabaseURL := os.Getenv("SUPABASE_URL")  // Injected by ECS
    supabaseKey := os.Getenv("SUPABASE_ANON_KEY")  // Injected by ECS
    
    // Your application code...
}
```

### **Python Services (YOLO/OCR)**
```python
import os

# Environment variables are available directly
model_path = os.getenv('MODEL_PATH', '/app/models/price_detection_model.pt')
confidence_threshold = float(os.getenv('CONFIDENCE_THRESHOLD', '0.5'))
device = os.getenv('DEVICE', 'cpu')

# Your service code...
```

## 🛡️ **Security Best Practices**

### **✅ What We're Doing Right**

1. **Secrets Encryption**: Sensitive data is encrypted using AWS KMS
2. **IAM Permissions**: Least privilege access to SSM parameters
3. **Parameter Store**: Centralized secret management
4. **No Hardcoded Secrets**: All secrets are externalized
5. **Secure Communication**: Internal service communication over localhost

### **🔒 Additional Security Recommendations**

1. **Rotate Secrets**: Regularly rotate database passwords and API keys
2. **Monitor Access**: Use CloudTrail to monitor parameter access
3. **Environment Separation**: Use different parameters for dev/staging/prod
4. **VPC Endpoints**: Consider VPC endpoints for SSM in production
5. **Secrets Manager**: For high-rotation secrets, consider AWS Secrets Manager

## 🚨 **Troubleshooting**

### **Common Issues**

#### **1. Parameter Not Found**
```bash
# Check if parameter exists
aws ssm get-parameter --name "/snagly/database-url" --region eu-north-1

# Create missing parameter
./manage-env-vars.sh create
```

#### **2. Permission Denied**
```bash
# Check IAM role permissions
aws iam get-role-policy --role-name snagly-task-execution-role --policy-name SSMParameterAccess

# Verify parameter path in IAM policy
aws iam list-role-policies --role-name snagly-task-execution-role
```

#### **3. ECS Task Failing to Start**
```bash
# Check ECS task logs
aws logs tail /ecs/snagly --follow --region eu-north-1

# Check ECS service events
aws ecs describe-services --cluster snagly-cluster --services snagly-service --region eu-north-1
```

### **Debug Commands**

```bash
# Test parameter access
./manage-env-vars.sh test

# Check ECS task definition
aws ecs describe-task-definition --task-definition snagly-task --region eu-north-1

# Check ECS service status
aws ecs describe-services --cluster snagly-cluster --services snagly-service --region eu-north-1
```

## 📊 **Monitoring**

### **CloudWatch Metrics**
- Parameter Store API calls
- ECS task health
- Application logs

### **Alerts**
Set up CloudWatch alarms for:
- Parameter Store access failures
- ECS task failures
- Application errors

## 🔄 **Updating Environment Variables**

### **For Non-sensitive Variables**
Update the CloudFormation template and redeploy:
```bash
# Edit cloudformation-template.yaml
# Update the Environment section
# Redeploy
./deploy-snagly.sh
```

### **For Sensitive Variables**
Update SSM parameters directly:
```bash
# Update parameter
./manage-env-vars.sh update-param database-url "new-connection-string" SecureString

# Restart ECS service to pick up new values
aws ecs update-service --cluster snagly-cluster --service snagly-service --force-new-deployment --region eu-north-1
```

## 🎯 **Summary**

Your Snagly application uses a secure, scalable approach to environment variable management:

- **Secrets** → AWS Systems Manager Parameter Store (encrypted)
- **Configuration** → ECS Task Definition (environment variables)
- **Access** → IAM roles with least privilege permissions
- **Management** → Automated scripts for easy administration

This approach ensures your sensitive data is secure while keeping configuration flexible and manageable! 🚀
