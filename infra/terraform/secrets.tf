# ─────────────────────────────────────────────────────────────
# AWS Secrets Manager
# ─────────────────────────────────────────────────────────────
resource "aws_secretsmanager_secret" "app_secrets" {
  name                    = "${var.project_name}/${var.environment}/app-secrets"
  description             = "Production environment secrets and connection strings for ScanDrix"
  recovery_window_in_days = 7

  tags = {
    Name = "${var.project_name}-${var.environment}-secrets"
  }
}

resource "aws_secretsmanager_secret_version" "app_secrets_initial" {
  secret_id = aws_secretsmanager_secret.app_secrets.id

  # Initial skeleton - sensitive values will be managed safely via AWS console or Doppler/CLI
  secret_string = jsonencode({
    DATABASE_URL            = "postgresql://postgres:REPLACE_IN_AWS_SECRETS@db.scandrix.internal:5432/scandrix"
    RABBITMQ_URL            = "amqps://REPLACE_IN_AWS_SECRETS@puffin.rmq2.cloudamqp.com/vhost"
    REDIS_URL               = "redis://${aws_elasticache_replication_group.redis.primary_endpoint_address}:6379"
    JWT_SECRET              = "REPLACE_WITH_SECURE_RANDOM_SECRET_KEY"
    SENTRY_DSN              = ""
    RESEND_API_KEY          = ""
    ZOHO_CRM_CLIENT_ID      = ""
    ZOHO_CRM_CLIENT_SECRET  = ""
    ZOHO_CRM_REFRESH_TOKEN  = ""
    POSTHOG_API_KEY         = ""
  })

  lifecycle {
    ignore_changes = [
      secret_string
    ]
  }
}
