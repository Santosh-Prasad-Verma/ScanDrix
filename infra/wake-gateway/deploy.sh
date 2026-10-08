#!/usr/bin/env bash
set -euo pipefail

# ScanDrix Wake-on-Request Gateway Deployment Script
REGION="${AWS_REGION:-ap-southeast-2}"
STACK_NAME="scandrix-wake-gateway"
INSTANCE_ID="i-07a8af27e8bc5380c"
IP="3.107.153.4"

echo "Packaging Lambda zip..."
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

zip -q -9 wake-gateway.zip handler.py

echo "Deploying CloudFormation stack: $STACK_NAME in $REGION..."
aws cloudformation deploy \
    --template-file template.yaml \
    --stack-name "$STACK_NAME" \
    --region "$REGION" \
    --capabilities CAPABILITY_NAMED_IAM \
    --parameter-overrides Ec2InstanceId="$INSTANCE_ID" Ec2Host="$IP"

echo "Updating Lambda code with handler.py..."
aws lambda update-function-code \
    --function-name scandrix-wake-gateway \
    --zip-file fileb://wake-gateway.zip \
    --region "$REGION" > /dev/null

echo "Retrieving public Function URL..."
URL=$(aws lambda get-function-url-config --function-name scandrix-wake-gateway --region "$REGION" --query FunctionUrl --output text)

echo "=========================================================="
echo "SUCCESS! Wake-on-Request Gateway is deployed."
echo "Public Gateway URL: $URL"
echo "Use this URL in GitHub Webhook Settings or frontend proxy."
echo "=========================================================="
