output "vpc_id" {
  description = "ID of the VPC"
  value       = aws_vpc.main.id
}

output "alb_dns_name" {
  description = "Public DNS name of the Application Load Balancer"
  value       = aws_lb.main.dns_name
}

output "alb_zone_id" {
  description = "Canonical hosted zone ID of the Application Load Balancer"
  value       = aws_lb.main.zone_id
}

output "cloudfront_domain_name" {
  description = "Domain name of the CloudFront distribution (empty when enable_cloudfront=false, Cloudflare CDN is used instead)"
  value       = var.enable_cloudfront ? aws_cloudfront_distribution.main[0].domain_name : ""
}

output "ecs_cluster_name" {
  description = "Name of the ECS cluster"
  value       = aws_ecs_cluster.main.name
}

output "ecr_api_repository_url" {
  description = "ECR repository URL for the API image"
  value       = aws_ecr_repository.api.repository_url
}

output "ecr_worker_repository_url" {
  description = "ECR repository URL for the background worker image"
  value       = aws_ecr_repository.worker.repository_url
}

output "ecr_webhooks_repository_url" {
  description = "ECR repository URL for the webhooks receiver image"
  value       = aws_ecr_repository.webhooks.repository_url
}

output "redis_endpoint" {
  description = "Primary endpoint address for ElastiCache Redis (empty when enable_elasticache=false, external REDIS_URL is used)"
  value       = var.enable_elasticache ? aws_elasticache_replication_group.redis[0].primary_endpoint_address : ""
}

output "secrets_manager_arn" {
  description = "ARN of the AWS Secrets Manager secret"
  value       = aws_secretsmanager_secret.app_secrets.arn
}
