# ─────────────────────────────────────────────────────────────
# Alarm notification target
#
# The three alarms below previously had no alarm_actions, so they transitioned
# to ALARM and notified nobody. AUDIT_REMEDIATION.md F-65.
# ─────────────────────────────────────────────────────────────
resource "aws_sns_topic" "alerts" {
  name = "${var.project_name}-${var.environment}-alerts"

  tags = {
    Name = "${var.project_name}-${var.environment}-alerts"
  }
}

# Optional. An email subscription needs an out-of-band confirmation click, so
# the address is a variable defaulting to empty rather than a hardcoded
# mailbox. Leave it unset to keep the topic created but un-subscribed.
variable "alert_email" {
  type        = string
  description = "Optional address subscribed to CloudWatch alarms. Requires confirming the AWS subscription email."
  default     = ""
}

resource "aws_sns_topic_subscription" "alert_email" {
  count = var.alert_email == "" ? 0 : 1

  topic_arn = aws_sns_topic.alerts.arn
  protocol  = "email"
  endpoint  = var.alert_email
}

# ─────────────────────────────────────────────────────────────
# CloudWatch Log Groups
#
# Retention was hardcoded to 14 days, which is often shorter than the interval
# between an incident and the investigation of it. It is now a variable, and
# the groups are protected from accidental deletion.
# AUDIT_REMEDIATION.md F-65.
# ─────────────────────────────────────────────────────────────
resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${var.project_name}-${var.environment}-api"
  retention_in_days = var.log_retention_days

  tags = {
    Name = "${var.project_name}-${var.environment}-api-logs"
  }

  # Logs are the only record of what a task did before it was replaced.
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_cloudwatch_log_group" "worker" {
  name              = "/ecs/${var.project_name}-${var.environment}-worker"
  retention_in_days = var.log_retention_days

  tags = {
    Name = "${var.project_name}-${var.environment}-worker-logs"
  }

  # Logs are the only record of what a task did before it was replaced.
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_cloudwatch_log_group" "webhooks" {
  name              = "/ecs/${var.project_name}-${var.environment}-webhooks"
  retention_in_days = var.log_retention_days

  tags = {
    Name = "${var.project_name}-${var.environment}-webhooks-logs"
  }

  # Logs are the only record of what a task did before it was replaced.
  lifecycle {
    prevent_destroy = true
  }
}

# ─────────────────────────────────────────────────────────────
# Observability Alarms
# ─────────────────────────────────────────────────────────────
resource "aws_cloudwatch_metric_alarm" "api_cpu_high" {
  alarm_name          = "${var.project_name}-${var.environment}-api-cpu-high"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 2
  metric_name         = "CPUUtilization"
  namespace           = "AWS/ECS"
  period              = 60
  statistic           = "Average"
  threshold           = 85
  alarm_description   = "Triggered when API container CPU exceeds 85% for 2 minutes"
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]

  dimensions = {
    ClusterName = aws_ecs_cluster.main.name
    ServiceName = aws_ecs_service.api.name
  }
}

resource "aws_cloudwatch_metric_alarm" "api_memory_high" {
  alarm_name          = "${var.project_name}-${var.environment}-api-memory-high"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 2
  metric_name         = "MemoryUtilization"
  namespace           = "AWS/ECS"
  period              = 60
  statistic           = "Average"
  threshold           = 90
  alarm_description   = "Triggered when API container memory exceeds 90%"
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]

  dimensions = {
    ClusterName = aws_ecs_cluster.main.name
    ServiceName = aws_ecs_service.api.name
  }
}

resource "aws_cloudwatch_metric_alarm" "alb_5xx" {
  count               = var.enable_alb ? 1 : 0
  alarm_name          = "${var.project_name}-${var.environment}-alb-5xx-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "HTTPCode_Target_5XX_Count"
  namespace           = "AWS/ApplicationELB"
  period              = 300
  statistic           = "Sum"
  threshold           = 10
  alarm_description   = "Triggered when ALB target 5XX error count exceeds 10 in 5 minutes"
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]

  dimensions = {
    LoadBalancer = aws_lb.main[0].arn_suffix
  }
}

# ─────────────────────────────────────────────────────────────
# Cloudflare Tunnel Sidecar Monitoring
# ─────────────────────────────────────────────────────────────
resource "aws_cloudwatch_log_metric_filter" "cloudflared_errors" {
  count          = local.cloudflare_enabled ? 1 : 0
  name           = "${var.project_name}-${var.environment}-cloudflared-errors"
  log_group_name = aws_cloudwatch_log_group.api.name
  pattern        = "?ERR ?\"ERR \" ?\"error\" ?\"failed to connect\""

  metric_transformation {
    name          = "CloudflaredErrorCount"
    namespace     = "${var.project_name}/CloudflareTunnel"
    value         = "1"
    default_value = 0
  }
}

resource "aws_cloudwatch_metric_alarm" "cloudflared_errors" {
  count               = local.cloudflare_enabled ? 1 : 0
  alarm_name          = "${var.project_name}-${var.environment}-cloudflared-errors-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "CloudflaredErrorCount"
  namespace           = "${var.project_name}/CloudflareTunnel"
  period              = 300
  statistic           = "Sum"
  threshold           = 5
  alarm_description   = "Triggered when cloudflared tunnel logs 5+ connection/tunnel errors within 5 minutes"
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]
}

