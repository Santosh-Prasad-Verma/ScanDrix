# ─────────────────────────────────────────────────────────────
# AWS ElastiCache (Redis) - Budget Lean Single-Node Tier
# ─────────────────────────────────────────────────────────────
resource "aws_elasticache_subnet_group" "redis" {
  name       = "${var.project_name}-${var.environment}-redis-subnet-group"
  subnet_ids = aws_subnet.private[*].id

  tags = {
    Name = "${var.project_name}-${var.environment}-redis-subnets"
  }
}

resource "aws_elasticache_parameter_group" "redis" {
  name   = "${var.project_name}-${var.environment}-redis7-params"
  family = "redis7"

  parameter {
    name  = "maxmemory-policy"
    value = "allkeys-lru"
  }
}

resource "aws_elasticache_replication_group" "redis" {
  count                      = var.enable_elasticache ? 1 : 0
  replication_group_id       = "${var.project_name}-${var.environment}-redis"
  description                = "Managed Redis cluster for ScanDrix rate-limiting and session cache"
  node_type                  = var.redis_node_type
  port                       = 6379
  parameter_group_name       = aws_elasticache_parameter_group.redis.name
  subnet_group_name          = aws_elasticache_subnet_group.redis.name
  security_group_ids         = [aws_security_group.redis.id]
  num_cache_clusters         = 1
  automatic_failover_enabled = false
  at_rest_encryption_enabled = true
  transit_encryption_enabled = false
  apply_immediately          = true

  tags = {
    Name = "${var.project_name}-${var.environment}-redis"
  }
}
