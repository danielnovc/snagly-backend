#!/bin/bash

# Quick script to push containers to AWS ECR
# Run this after AWS CLI is configured

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Get AWS account info
log_info "Getting AWS account information..."
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION=$(aws configure get region)

if [ -z "$AWS_ACCOUNT_ID" ] || [ -z "$AWS_REGION" ]; then
    log_error "AWS CLI not configured properly. Please run 'aws configure' first."
    exit 1
fi

ECR_REGISTRY="${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

log_info "AWS Account ID: $AWS_ACCOUNT_ID"
log_info "AWS Region: $AWS_REGION"
log_info "ECR Registry: $ECR_REGISTRY"

# Login to ECR
log_info "Logging in to Amazon ECR..."
aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin $ECR_REGISTRY

# Create ECR repositories if they don't exist
log_info "Creating ECR repositories..."
repositories=("distrack-backend" "distrack-yolo" "distrack-ocr")

for repo in "${repositories[@]}"; do
    if ! aws ecr describe-repositories --repository-names $repo --region $AWS_REGION &> /dev/null; then
        log_info "Creating ECR repository: $repo"
        aws ecr create-repository --repository-name $repo --region $AWS_REGION
    else
        log_info "ECR repository $repo already exists"
    fi
done

# Build and push backend image
log_info "Building and pushing backend image..."
docker build -t distrack-backend:latest .
docker tag distrack-backend:latest $ECR_REGISTRY/distrack-backend:latest
docker push $ECR_REGISTRY/distrack-backend:latest

# Build and push YOLO image
log_info "Building and pushing YOLO image..."
docker build -t distrack-yolo:latest ../yolo-training/
docker tag distrack-yolo:latest $ECR_REGISTRY/distrack-yolo:latest
docker push $ECR_REGISTRY/distrack-yolo:latest

# Build and push OCR image
log_info "Building and pushing OCR image..."
docker build -f Dockerfile.ocr -t distrack-ocr:latest .
docker tag distrack-ocr:latest $ECR_REGISTRY/distrack-ocr:latest
docker push $ECR_REGISTRY/distrack-ocr:latest

log_info "All images pushed successfully!"
log_info "Images are now available at:"
log_info "  - $ECR_REGISTRY/distrack-backend:latest"
log_info "  - $ECR_REGISTRY/distrack-yolo:latest"
log_info "  - $ECR_REGISTRY/distrack-ocr:latest"
