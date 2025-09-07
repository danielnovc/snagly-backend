# Distrack AWS Deployment Guide

This guide will help you deploy the Distrack price monitoring service to AWS using ECS Fargate.

## Architecture Overview

The deployment uses:
- **AWS ECS Fargate** for container orchestration
- **Application Load Balancer** for traffic routing
- **CloudWatch** for logging and monitoring
- **ECR** for container image storage
- **CloudFormation** for infrastructure as code

## Prerequisites

1. **AWS CLI** installed and configured
2. **Docker** installed
3. **AWS Account** with appropriate permissions
4. **VPC and Subnets** (default VPC is used if available)

## Quick Start

### 1. Initial Setup

```bash
# Navigate to the aws directory
cd backend/aws

# Run the setup script
./setup.sh
```

This script will:
- Check AWS CLI configuration
- Get VPC and subnet information
- Create CloudWatch log group
- Generate configuration files
- Make deployment scripts executable

### 2. Configure Environment

Edit `aws/.env.production` and update:
- `ALLOWED_ORIGINS` with your frontend domains
- Any other environment-specific settings

### 3. Deploy to AWS

```bash
# Run the deployment script
./deploy.sh
```

This script will:
- Build and push Docker images to ECR
- Deploy infrastructure using CloudFormation
- Update ECS service
- Provide service status

## Manual Deployment Steps

If you prefer to run the deployment manually:

### 1. Build and Push Images

```bash
# Set environment variables
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export AWS_REGION=$(aws configure get region)
export ECR_REGISTRY="${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

# Login to ECR
aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin $ECR_REGISTRY

# Create ECR repositories
aws ecr create-repository --repository-name distrack-backend --region $AWS_REGION
aws ecr create-repository --repository-name distrack-yolo --region $AWS_REGION
aws ecr create-repository --repository-name distrack-ocr --region $AWS_REGION

# Build and push images
docker build -t distrack-backend:latest .
docker tag distrack-backend:latest $ECR_REGISTRY/distrack-backend:latest
docker push $ECR_REGISTRY/distrack-backend:latest

docker build -t distrack-yolo:latest ../yolo-training/
docker tag distrack-yolo:latest $ECR_REGISTRY/distrack-yolo:latest
docker push $ECR_REGISTRY/distrack-yolo:latest

docker build -f Dockerfile.ocr -t distrack-ocr:latest .
docker tag distrack-ocr:latest $ECR_REGISTRY/distrack-ocr:latest
docker push $ECR_REGISTRY/distrack-ocr:latest
```

### 2. Deploy Infrastructure

```bash
# Deploy CloudFormation stack
aws cloudformation create-stack \
    --stack-name distrack-stack \
    --template-body file://cloudformation-template.yaml \
    --parameters file://cloudformation-parameters.json \
    --capabilities CAPABILITY_NAMED_IAM \
    --region $AWS_REGION

# Wait for stack creation
aws cloudformation wait stack-create-complete --stack-name distrack-stack --region $AWS_REGION
```

### 3. Update ECS Service

```bash
# Force new deployment
aws ecs update-service \
    --cluster distrack-cluster \
    --service distrack-service \
    --force-new-deployment \
    --region $AWS_REGION
```

## Monitoring and Logs

### Check Service Status

```bash
aws ecs describe-services \
    --cluster distrack-cluster \
    --services distrack-service \
    --region $AWS_REGION
```

### View Logs

```bash
# View all logs
aws logs tail /ecs/distrack --follow

# View specific service logs
aws logs tail /ecs/distrack --filter-pattern "backend" --follow
aws logs tail /ecs/distrack --filter-pattern "yolo" --follow
aws logs tail /ecs/distrack --filter-pattern "ocr" --follow
```

### Get Load Balancer URL

```bash
aws cloudformation describe-stacks \
    --stack-name distrack-stack \
    --region $AWS_REGION \
    --query 'Stacks[0].Outputs[?OutputKey==`LoadBalancerDNS`].OutputValue' \
    --output text
```

## Flutter App Integration

The API is configured to work with Flutter apps:
- **CORS**: Set to `*` to allow all origins (Flutter web, mobile, desktop)
- **API Keys**: Use test keys for development, generate production keys as needed
- **Integration Guide**: See `FLUTTER_INTEGRATION.md` for complete Flutter setup

## Configuration Files

### Environment Variables

Key environment variables in `aws/.env.production`:

- `AWS_REGION`: AWS region for deployment
- `AWS_ACCOUNT_ID`: Your AWS account ID
- `DATABASE_URL`: Supabase database connection string
- `SUPABASE_URL`: Supabase project URL
- `SUPABASE_ANON_KEY`: Supabase anonymous key
- `ALLOWED_ORIGINS`: CORS allowed origins (set to `*` for Flutter compatibility)
- `PRICE_CHECK_INTERVAL`: Cron expression for price checks (default: every 12 hours)

### CloudFormation Parameters

Update `aws/cloudformation-parameters.json` with your specific values:

- `VpcId`: Your VPC ID
- `SubnetIds`: Comma-separated list of subnet IDs
- `DatabaseUrl`: Database connection string
- `SupabaseUrl`: Supabase project URL
- `SupabaseAnonKey`: Supabase anonymous key
- `AllowedOrigins`: CORS allowed origins

## Cost Optimization

For a background scraping service running every 12 hours:

1. **Use Fargate Spot** for cost savings (add to task definition)
2. **Set appropriate CPU/Memory** (current: 2 vCPU, 4GB RAM)
3. **Use CloudWatch Logs retention** (30 days default)
4. **Consider scheduled scaling** for predictable workloads

## Troubleshooting

### Common Issues

1. **Service fails to start**
   - Check CloudWatch logs
   - Verify environment variables
   - Check security group rules

2. **Images not found**
   - Verify ECR repositories exist
   - Check image tags
   - Ensure ECR login is successful

3. **Database connection issues**
   - Verify DATABASE_URL is correct
   - Check security group allows outbound connections
   - Verify Supabase credentials

### Health Checks

The service provides health check endpoints:

- `GET /health` - Basic health check
- `GET /status` - Detailed status information
- `GET /metrics` - System metrics

### Scaling

To scale the service:

```bash
aws ecs update-service \
    --cluster distrack-cluster \
    --service distrack-service \
    --desired-count 2 \
    --region $AWS_REGION
```

## Security Considerations

1. **Secrets Management**: Database credentials are stored in SSM Parameter Store
2. **Network Security**: Services communicate internally within the task
3. **IAM Roles**: Minimal permissions for ECS tasks
4. **HTTPS**: Consider adding SSL certificate for production

## Cleanup

To remove all resources:

```bash
# Delete CloudFormation stack
aws cloudformation delete-stack --stack-name distrack-stack --region $AWS_REGION

# Delete ECR repositories (optional)
aws ecr delete-repository --repository-name distrack-backend --force --region $AWS_REGION
aws ecr delete-repository --repository-name distrack-yolo --force --region $AWS_REGION
aws ecr delete-repository --repository-name distrack-ocr --force --region $AWS_REGION

# Delete log group (optional)
aws logs delete-log-group --log-group-name /ecs/distrack --region $AWS_REGION
```

## Support

For issues or questions:
1. Check CloudWatch logs
2. Review ECS service events
3. Verify all configuration files
4. Check AWS service limits and quotas
