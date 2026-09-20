#!/usr/bin/env bash

set -eo pipefail

echo "==> [LocalStack Init] Initializing ScanDrix mock AWS infrastructure..."

REGION="${AWS_DEFAULT_REGION:-us-east-1}"

# 1. Create S3 Storage Bucket for code artifacts & review diffs
echo "==> [LocalStack Init] Creating S3 bucket: scandrix-artifacts..."
awslocal --region "${REGION}" s3 mb s3://scandrix-artifacts || true
awslocal --region "${REGION}" s3api put-bucket-versioning \
    --bucket scandrix-artifacts \
    --versioning-configuration Status=Enabled || true

# 2. Create Master KMS Key for envelope encryption
echo "==> [LocalStack Init] Provisioning KMS master key..."
KEY_ID=$(awslocal --region "${REGION}" kms create-key \
    --description "ScanDrix Local Envelope Encryption Key" \
    --query "KeyMetadata.KeyId" --output text 2>/dev/null || echo "")

if [ -n "${KEY_ID}" ] && [ "${KEY_ID}" != "None" ]; then
    awslocal --region "${REGION}" kms create-alias \
        --alias-name "alias/scandrix-master" \
        --target-key-id "${KEY_ID}" || true
    echo "==> [LocalStack Init] KMS Key created: ${KEY_ID} (alias/scandrix-master)"
fi

# 3. Create SQS Review Queues & DLQ
echo "==> [LocalStack Init] Provisioning SQS queues..."
awslocal --region "${REGION}" sqs create-queue --queue-name scandrix-dlq || true
awslocal --region "${REGION}" sqs create-queue --queue-name scandrix-tasks || true

echo "==> [LocalStack Init] ScanDrix mock AWS resources initialized successfully."
