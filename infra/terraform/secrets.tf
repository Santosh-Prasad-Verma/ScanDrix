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
  #
  # jwt_secret is the only value with no placeholder: it signs access tokens, so
  # a literal here would mean a fresh environment boots with a publicly known
  # signing key and every token is forgeable. It is a required sensitive
  # variable instead, validated for length and for the old placeholder
  # (AUDIT_REMEDIATION.md F-62).
  #
  # The remaining placeholders are intentionally left for an operator because
  # they fail loudly at connect time, unlike a signing key which fails silently
  # and only at exploit time.
  secret_string = jsonencode({
    DATABASE_URL           = "postgresql://postgres:REPLACE_IN_AWS_SECRETS@db.scandrix.internal:5432/scandrix"
    RABBITMQ_URL           = "amqps://REPLACE_IN_AWS_SECRETS@puffin.rmq2.cloudamqp.com/vhost"
    REDIS_URL              = "redis://${aws_elasticache_replication_group.redis.primary_endpoint_address}:6379"
    JWT_SECRET             = var.jwt_secret
    SENTRY_DSN             = ""
    RESEND_API_KEY         = ""
    ZOHO_CRM_CLIENT_ID     = ""
    ZOHO_CRM_CLIENT_SECRET = ""
    ZOHO_CRM_REFRESH_TOKEN = ""
    POSTHOG_API_KEY        = ""
  })

  lifecycle {
    ignore_changes = [
      secret_string
    ]
  }
}
