# ─────────────────────────────────────────────────────────────
# CloudWatch Log Groups (14-day retention for cost optimization)
# ─────────────────────────────────────────────────────────────
resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${var.project_name}-${var.environment}-api"
  retention_in_days = 14

  tags = {
    Name = "${var.project_name}-${var.environment}-api-logs"
  }
}

resource "aws_cloudwatch_log_group" "worker" {
  name              = "/ecs/${var.project_name}-${var.environment}-worker"
  retention_in_days = 14

  tags = {
    Name = "${var.project_name}-${var.environment}-worker-logs"
  }
}

resource "aws_cloudwatch_log_group" "webhooks" {
  name              = "/ecs/${var.project_name}-${var.environment}-webhooks"
  retention_in_days = 14

  tags = {
    Name = "${var.project_name}-${var.environment}-webhooks-logs"
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

  dimensions = {
    ClusterName = aws_ecs_cluster.main.name
    ServiceName = aws_ecs_service.api.name
  }
}

resource "aws_cloudwatch_metric_alarm" "alb_5xx" {
  alarm_name          = "${var.project_name}-${var.environment}-alb-5xx-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "HTTPCode_Target_5XX_Count"
  namespace           = "AWS/ApplicationELB"
  period              = 300
  statistic           = "Sum"
  threshold           = 10
  alarm_description   = "Triggered when ALB target 5XX error count exceeds 10 in 5 minutes"

  dimensions = {
    LoadBalancer = aws_lb.main.arn_suffix
  }
}
