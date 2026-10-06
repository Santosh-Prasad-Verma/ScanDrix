variable "aws_region" {
  type        = string
  description = "AWS region for primary infrastructure deployment"
  default     = "ap-south-1"

  validation {
    condition     = can(regex("^[a-z]{2}-[a-z]+-[0-9]$", var.aws_region))
    error_message = "aws_region must look like a region id, e.g. ap-south-1."
  }

}

variable "use_localstack" {
  type        = bool
  description = "Enable LocalStack endpoints and dummy credentials for local AWS emulation"
  default     = false
}

variable "environment" {
  type        = string
  description = "Deployment environment (e.g., production, staging)"
  default     = "production"

  validation {
    condition     = contains(["production", "staging", "development", "local"], var.environment)
    error_message = "environment must be one of production, staging, development, local."
  }

}

variable "project_name" {
  type        = string
  description = "Top-level project namespace for resource naming and tagging"
  default     = "scandrix"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,30}$", var.project_name))
    error_message = "project_name must be lowercase alphanumeric with dashes, 2-31 chars."
  }

}

variable "jwt_secret" {
  type        = string
  description = <<-EOT
    HS256 signing secret for access tokens. There is deliberately no default:
    a default would let an environment be created with a publicly known
    signing key, which makes every token forgeable.

    Supply it out of band (never commit it):
      export TF_VAR_jwt_secret="$(openssl rand -base64 48 | tr -d '\n')"

    Rotating it invalidates outstanding access tokens only. Refresh tokens are
    opaque random bytes hashed in auth.tokenHash and are NOT signed with this
    value, so they survive; access tokens live for JWT_EXPIRES_IN (15m), so the
    rotation blip is bounded and self-healing via /api/v1/auth/refresh.
  EOT
  sensitive   = true

  validation {
    condition     = length(var.jwt_secret) >= 32
    error_message = "jwt_secret must be at least 32 characters. Generate one with: openssl rand -base64 48 | tr -d '\\n'"
  }

  validation {
    condition     = lower(var.jwt_secret) != "replace_with_secure_random_secret_key"
    error_message = "jwt_secret is still the committed placeholder. An environment must never be created with a known signing key."
  }
}

variable "domain_name" {
  type        = string
  description = "Primary apex domain name"
  default     = "scandrix.dev"

  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$", var.domain_name))
    error_message = "domain_name must be a valid lowercase DNS name, e.g. scandrix.dev."
  }

}

variable "vpc_cidr" {
  type        = string
  description = "CIDR block for the VPC"
  default     = "10.0.0.0/16"

  validation {
    condition     = can(cidrnetmask(var.vpc_cidr))
    error_message = "vpc_cidr must be valid CIDR notation."
  }

  validation {
    condition     = tonumber(split("/", var.vpc_cidr)[1]) >= 16
    error_message = "vpc_cidr must be a /16 or larger."
  }

}

variable "public_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for the 2 public subnets"
  default     = ["10.0.1.0/24", "10.0.2.0/24"]

  validation {
    condition     = length(var.public_subnet_cidrs) >= 2
    error_message = "at least 2 public subnets are required for an ALB."
  }

  validation {
    condition     = alltrue([for c in var.public_subnet_cidrs : can(cidrnetmask(c))])
    error_message = "every public_subnet_cidrs entry must be valid CIDR."
  }

  validation {
    condition     = alltrue([for c in var.public_subnet_cidrs : c != var.vpc_cidr])
    error_message = "a subnet CIDR must not equal the VPC CIDR."
  }

}

variable "private_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for the 2 private subnets"
  default     = ["10.0.10.0/24", "10.0.11.0/24"]

  validation {
    condition     = length(var.private_subnet_cidrs) >= 2
    error_message = "at least 2 private subnets are required."
  }

  validation {
    condition     = alltrue([for c in var.private_subnet_cidrs : can(cidrnetmask(c))])
    error_message = "every private_subnet_cidrs entry must be valid CIDR."
  }

  validation {
    condition     = length(setintersection(toset(var.public_subnet_cidrs), toset(var.private_subnet_cidrs))) == 0
    error_message = "public and private subnet CIDRs must not overlap."
  }

}

variable "api_cpu" {
  type        = number
  description = "CPU units for the API container (256 = 0.25 vCPU, 512 = 0.5 vCPU, 1024 = 1 vCPU)"
  default     = 512

  validation {
    condition     = var.api_cpu >= 256 && var.api_cpu <= 8192
    error_message = "api_cpu must be between 256 and 8192 (current default 1024)."
  }

}

variable "api_memory" {
  type        = number
  description = "Memory for the API container in MB"
  default     = 1024

  validation {
    condition     = var.api_memory >= 512 && var.api_memory <= 32768
    error_message = "api_memory must be between 512 and 32768 (current default 2048)."
  }

}

variable "api_desired_count" {
  type        = number
  description = "Number of API task instances to run in ECS"
  default     = 1

  validation {
    condition     = var.api_desired_count >= 0 && var.api_desired_count <= 50
    error_message = "api_desired_count must be between 0 and 50 (current default 1)."
  }

}

variable "worker_cpu" {
  type        = number
  description = "CPU units for the background worker container"
  default     = 256

  validation {
    condition     = var.worker_cpu >= 256 && var.worker_cpu <= 8192
    error_message = "worker_cpu must be between 256 and 8192 (current default 1024)."
  }

}

variable "worker_memory" {
  type        = number
  description = "Memory for the background worker container in MB"
  default     = 512

  validation {
    condition     = var.worker_memory >= 512 && var.worker_memory <= 32768
    error_message = "worker_memory must be between 512 and 32768 (current default 2048)."
  }

}

variable "worker_desired_count" {
  type        = number
  description = "Number of worker task instances to run in ECS"
  default     = 1

  validation {
    condition     = var.worker_desired_count >= 0 && var.worker_desired_count <= 50
    error_message = "worker_desired_count must be between 0 and 50 (current default 1)."
  }

}

variable "worker_min_capacity" {
  type        = number
  description = "Minimum number of worker tasks in the auto-scaling group"
  default     = 1

  validation {
    condition     = var.worker_min_capacity >= 0 && var.worker_min_capacity <= 50
    error_message = "worker_min_capacity must be between 0 and 50 (current default 1)."
  }

}

variable "worker_max_capacity" {
  type        = number
  description = "Maximum number of worker tasks in the auto-scaling group"
  default     = 4
  validation {
    condition     = var.worker_max_capacity >= var.worker_min_capacity
    error_message = "worker_max_capacity must be greater than or equal to worker_min_capacity."
  }

}

variable "webhooks_cpu" {
  type        = number
  description = "CPU units for the webhooks receiver container"
  default     = 256

  validation {
    condition     = var.webhooks_cpu >= 128 && var.webhooks_cpu <= 4096
    error_message = "webhooks_cpu must be between 128 and 4096 (current default 512)."
  }

}

variable "webhooks_memory" {
  type        = number
  description = "Memory for the webhooks receiver container in MB"
  default     = 512

  validation {
    condition     = var.webhooks_memory >= 256 && var.webhooks_memory <= 16384
    error_message = "webhooks_memory must be between 256 and 16384 (current default 1024)."
  }

}

variable "webhooks_desired_count" {
  type        = number
  description = "Number of webhooks receiver task instances to run in ECS"
  default     = 0

  validation {
    condition     = var.webhooks_desired_count >= 0 && var.webhooks_desired_count <= 50
    error_message = "webhooks_desired_count must be between 0 and 50 (current default 0)."
  }

}

variable "redis_node_type" {
  type        = string
  description = "ElastiCache Redis node type (t4g.micro for budget lean tier)"
  default     = "cache.t4g.micro"

  validation {
    condition     = can(regex("^cache\\.[a-z0-9]+\\.[a-z0-9]+$", var.redis_node_type))
    error_message = "redis_node_type must look like cache.t4g.micro."
  }

}

variable "api_image_tag" {
  type        = string
  description = "Docker image tag for the API container"

  # No default. The previous default was "latest", a floating tag, so a
  # `terraform apply` with no explicit tag deployed whatever happened to be last
  # pushed. A missing tag is now a hard error instead of an implicit "latest"
  # (AUDIT_REMEDIATION.md F-61).
  validation {
    condition     = length(trimspace(var.api_image_tag)) > 0 && !startswith(trimspace(var.api_image_tag), "latest")
    error_message = "api_image_tag must be an explicit immutable tag (release version or commit sha), not the latest tag."
  }
}

variable "worker_image_tag" {
  type        = string
  description = "Docker image tag for the worker container"

  # See api_image_tag: an explicit immutable tag is required (F-61).
  validation {
    condition     = length(trimspace(var.worker_image_tag)) > 0 && !startswith(trimspace(var.worker_image_tag), "latest")
    error_message = "worker_image_tag must be an explicit immutable tag (release version or commit sha), not \"latest\"."
  }
}

variable "webhooks_image_tag" {
  type        = string
  description = "Docker image tag for the webhooks container"

  # See api_image_tag: an explicit immutable tag is required (F-61).
  validation {
    condition     = length(trimspace(var.webhooks_image_tag)) > 0 && !startswith(trimspace(var.webhooks_image_tag), "latest")
    error_message = "webhooks_image_tag must be an explicit immutable tag (release version or commit sha), not \"latest\"."
  }
}

variable "log_retention_days" {
  type        = number
  description = "CloudWatch Logs retention. Was hardcoded to 14 days, which can be shorter than the gap between an incident and its investigation."
  default     = 30

  validation {
    condition     = contains([1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 3653], var.log_retention_days)
    error_message = "log_retention_days must be one of the values CloudWatch Logs accepts."
  }
}

variable "alb_deletion_protection" {
  type        = bool
  description = "Block deletion of the ALB. Was hardcoded to false, so a stray apply could remove the load balancer in front of production."
  default     = true
}

variable "enable_alb" {
  type        = bool
  description = "Create Application Load Balancer ($22/mo). False = use Cloudflare Tunnel (cloudflared sidecar) for 100% free ingress with zero open ports."
  default     = false
}

variable "cloudflare_api_token" {
  type        = string
  description = "Cloudflare API token (DNS:Edit, SSL:Edit, Cache Rules:Edit, Firewall:Edit, Tunnel:Edit scoped to scandrix.dev). Export TF_VAR_cloudflare_api_token from local .env. Empty disables Cloudflare resources."
  default     = ""
  sensitive   = true
}

variable "cloudflare_zone_id" {
  type        = string
  description = "Cloudflare Zone ID for scandrix.dev (Domain Overview, right sidebar)."
  default     = ""
}

variable "cloudflare_account_id" {
  type        = string
  description = "Cloudflare Account ID for Tunnel + WAF."
  default     = ""
}

variable "enable_nat_gateway" {
  type        = bool
  description = "Create NAT Gateway ($33/mo). False = public subnets + VPC endpoints, for $100-credit path with Supabase/CloudAMQP external."
  default     = false
}

variable "enable_elasticache" {
  type        = bool
  description = "Create ElastiCache Redis ($14.6/mo). False = use external REDIS_URL (Upstash free / self-host)."
  default     = false
}

variable "enable_cloudfront" {
  type        = bool
  description = "Create CloudFront distribution. False = Cloudflare CDN free instead."
  default     = false
}

variable "enable_webhooks_service" {
  type        = bool
  description = "Run standalone webhooks Fargate service. False = merged into api (api handles /webhooks*), saves ~$11/mo."
  default     = false
}

variable "enable_cloudflare_tunnel" {
  type        = bool
  description = "Create Cloudflare Tunnel + DNS + WAF. Requires cloudflare_api_token/zone/account IDs."
  default     = true
}
