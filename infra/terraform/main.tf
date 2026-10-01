terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.5"
    }
  }

  # Uncomment and configure with your S3 bucket & DynamoDB table for remote state locking:
  # Backend arguments cannot come from variables - they are resolved during
  # `terraform init`, before variables exist. So the block declares the type
  # only and the arguments are supplied at init time:
  #
  #   cp backend.hcl.example backend.hcl && $EDITOR backend.hcl
  #   terraform init -backend-config=backend.hcl
  #
  # AUDIT_REMEDIATION.md F-59: state was local-only, so it was neither shared
  # across operators nor versioned, and a lost workstation lost the state.
  backend "s3" {}
}

provider "aws" {
  region = var.aws_region

  access_key                  = var.use_localstack ? "test" : null
  secret_key                  = var.use_localstack ? "test" : null
  skip_credentials_validation = var.use_localstack
  skip_metadata_api_check     = var.use_localstack
  skip_requesting_account_id  = var.use_localstack

  dynamic "endpoints" {
    for_each = var.use_localstack ? [1] : []
    content {
      acm                    = "http://localhost:4566"
      apigateway             = "http://localhost:4566"
      applicationautoscaling = "http://localhost:4566"
      autoscaling            = "http://localhost:4566"
      cloudformation         = "http://localhost:4566"
      cloudfront             = "http://localhost:4566"
      cloudwatch             = "http://localhost:4566"
      dynamodb               = "http://localhost:4566"
      ec2                    = "http://localhost:4566"
      ecr                    = "http://localhost:4566"
      ecs                    = "http://localhost:4566"
      elasticache            = "http://localhost:4566"
      elb                    = "http://localhost:4566"
      elbv2                  = "http://localhost:4566"
      iam                    = "http://localhost:4566"
      logs                   = "http://localhost:4566"
      route53                = "http://localhost:4566"
      s3                     = "http://localhost:4566"
      secretsmanager         = "http://localhost:4566"
      sts                    = "http://localhost:4566"
    }
  }

  default_tags {
    tags = {
      Project     = var.project_name
      Environment = var.environment
      ManagedBy   = "Terraform"
    }
  }
}

# Optional second provider for CloudFront ACM certificates (which must reside in us-east-1)
provider "aws" {
  alias  = "us_east_1"
  region = "us-east-1"

  access_key                  = var.use_localstack ? "test" : null
  secret_key                  = var.use_localstack ? "test" : null
  skip_credentials_validation = var.use_localstack
  skip_metadata_api_check     = var.use_localstack
  skip_requesting_account_id  = var.use_localstack

  dynamic "endpoints" {
    for_each = var.use_localstack ? [1] : []
    content {
      acm        = "http://localhost:4566"
      cloudfront = "http://localhost:4566"
      route53    = "http://localhost:4566"
      sts        = "http://localhost:4566"
    }
  }

  default_tags {
    tags = {
      Project     = var.project_name
      Environment = var.environment
      ManagedBy   = "Terraform"
    }
  }
}
