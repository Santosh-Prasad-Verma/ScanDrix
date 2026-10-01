variable "aws_region" {
  type        = string
  description = "AWS region for primary infrastructure deployment"
  default     = "ap-south-1"
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
}

variable "project_name" {
  type        = string
  description = "Top-level project namespace for resource naming and tagging"
  default     = "scandrix"
}

variable "domain_name" {
  type        = string
  description = "Primary apex domain name"
  default     = "scandrix.dev"
}

variable "vpc_cidr" {
  type        = string
  description = "CIDR block for the VPC"
  default     = "10.0.0.0/16"
}

variable "public_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for the 2 public subnets"
  default     = ["10.0.1.0/24", "10.0.2.0/24"]
}

variable "private_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for the 2 private subnets"
  default     = ["10.0.10.0/24", "10.0.11.0/24"]
}

variable "api_cpu" {
  type        = number
  description = "CPU units for the API container (256 = 0.25 vCPU, 512 = 0.5 vCPU, 1024 = 1 vCPU)"
  default     = 512
}

variable "api_memory" {
  type        = number
  description = "Memory for the API container in MB"
  default     = 1024
}

variable "api_desired_count" {
  type        = number
  description = "Number of API task instances to run in ECS"
  default     = 2
}

variable "worker_cpu" {
  type        = number
  description = "CPU units for the background worker container"
  default     = 512
}

variable "worker_memory" {
  type        = number
  description = "Memory for the background worker container in MB"
  default     = 1024
}

variable "worker_desired_count" {
  type        = number
  description = "Number of worker task instances to run in ECS"
  default     = 1
}

variable "worker_min_capacity" {
  type        = number
  description = "Minimum number of worker tasks in the auto-scaling group"
  default     = 1
}

variable "worker_max_capacity" {
  type        = number
  description = "Maximum number of worker tasks in the auto-scaling group"
  default     = 4
}

variable "webhooks_cpu" {
  type        = number
  description = "CPU units for the webhooks receiver container"
  default     = 256
}

variable "webhooks_memory" {
  type        = number
  description = "Memory for the webhooks receiver container in MB"
  default     = 512
}

variable "webhooks_desired_count" {
  type        = number
  description = "Number of webhooks receiver task instances to run in ECS"
  default     = 1
}

variable "redis_node_type" {
  type        = string
  description = "ElastiCache Redis node type (t4g.micro for budget lean tier)"
  default     = "cache.t4g.micro"
}

variable "api_image_tag" {
  type        = string
  description = "Docker image tag for the API container"
  default     = "latest"
}

variable "worker_image_tag" {
  type        = string
  description = "Docker image tag for the worker container"
  default     = "latest"
}

variable "webhooks_image_tag" {
  type        = string
  description = "Docker image tag for the webhooks container"
  default     = "latest"
}
