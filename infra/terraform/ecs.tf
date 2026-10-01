# ─────────────────────────────────────────────────────────────
# ECS Cluster
# ─────────────────────────────────────────────────────────────
resource "aws_ecs_cluster" "main" {
  name = "${var.project_name}-${var.environment}-cluster"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = {
    Name = "${var.project_name}-${var.environment}-ecs"
  }
}

# ─────────────────────────────────────────────────────────────
# ECR Repositories with Lifecycle Cleanup
# ─────────────────────────────────────────────────────────────
resource "aws_ecr_repository" "api" {
  name                 = "${var.project_name}-api"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_repository" "worker" {
  name                 = "${var.project_name}-worker"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_repository" "webhooks" {
  name                 = "${var.project_name}-webhooks"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "api" {
  repository = aws_ecr_repository.api.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep last 10 images to conserve storage costs"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = 10
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}

resource "aws_ecr_lifecycle_policy" "worker" {
  repository = aws_ecr_repository.worker.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep last 10 images to conserve storage costs"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = 10
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}

resource "aws_ecr_lifecycle_policy" "webhooks" {
  repository = aws_ecr_repository.webhooks.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep last 10 images to conserve storage costs"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = 10
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}

# ─────────────────────────────────────────────────────────────
# Task Definitions (Fargate awsvpc)
# ─────────────────────────────────────────────────────────────
resource "aws_ecs_task_definition" "api" {
  family                   = "${var.project_name}-${var.environment}-api"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.api_cpu
  memory                   = var.api_memory
  execution_role_arn       = aws_iam_role.ecs_execution_role.arn
  task_role_arn            = aws_iam_role.ecs_task_role.arn

  container_definitions = jsonencode([
    {
      name      = "api"
      image     = "${aws_ecr_repository.api.repository_url}:${var.api_image_tag}"
      essential = true

      portMappings = [
        {
          containerPort = 8080
          hostPort      = 8080
          protocol      = "tcp"
        }
      ]

      environment = [
        { name = "ENVIRONMENT", value = var.environment },
        { name = "PORT", value = "8080" },
        { name = "API_PORT", value = "8080" }
      ]

      secrets = [
        { name = "DATABASE_URL", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:DATABASE_URL::" },
        { name = "RABBITMQ_URL", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:RABBITMQ_URL::" },
        { name = "REDIS_URL", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:REDIS_URL::" },
        { name = "JWT_SECRET", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:JWT_SECRET::" },
        { name = "SENTRY_DSN", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:SENTRY_DSN::" },
        { name = "RESEND_API_KEY", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:RESEND_API_KEY::" },
        { name = "ZOHO_CRM_CLIENT_ID", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:ZOHO_CRM_CLIENT_ID::" },
        { name = "ZOHO_CRM_CLIENT_SECRET", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:ZOHO_CRM_CLIENT_SECRET::" },
        { name = "ZOHO_CRM_REFRESH_TOKEN", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:ZOHO_CRM_REFRESH_TOKEN::" },
        { name = "POSTHOG_API_KEY", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:POSTHOG_API_KEY::" }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.api.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "api"
        }
      }
    }
  ])
}

resource "aws_ecs_task_definition" "webhooks" {
  family                   = "${var.project_name}-${var.environment}-webhooks"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.webhooks_cpu
  memory                   = var.webhooks_memory
  execution_role_arn       = aws_iam_role.ecs_execution_role.arn
  task_role_arn            = aws_iam_role.ecs_task_role.arn

  container_definitions = jsonencode([
    {
      name      = "webhooks"
      image     = "${aws_ecr_repository.webhooks.repository_url}:${var.webhooks_image_tag}"
      essential = true

      portMappings = [
        {
          containerPort = 8081
          hostPort      = 8081
          protocol      = "tcp"
        }
      ]

      environment = [
        { name = "ENVIRONMENT", value = var.environment },
        { name = "PORT", value = "8081" },
        { name = "WEBHOOKS_PORT", value = "8081" }
      ]

      secrets = [
        { name = "DATABASE_URL", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:DATABASE_URL::" },
        { name = "RABBITMQ_URL", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:RABBITMQ_URL::" },
        { name = "SENTRY_DSN", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:SENTRY_DSN::" }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.webhooks.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "webhooks"
        }
      }
    }
  ])
}

resource "aws_ecs_task_definition" "worker" {
  family                   = "${var.project_name}-${var.environment}-worker"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.worker_cpu
  memory                   = var.worker_memory
  execution_role_arn       = aws_iam_role.ecs_execution_role.arn
  task_role_arn            = aws_iam_role.ecs_task_role.arn

  container_definitions = jsonencode([
    {
      name      = "worker"
      image     = "${aws_ecr_repository.worker.repository_url}:${var.worker_image_tag}"
      essential = true

      environment = [
        { name = "ENVIRONMENT", value = var.environment }
      ]

      secrets = [
        { name = "DATABASE_URL", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:DATABASE_URL::" },
        { name = "RABBITMQ_URL", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:RABBITMQ_URL::" },
        { name = "REDIS_URL", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:REDIS_URL::" },
        { name = "SENTRY_DSN", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:SENTRY_DSN::" },
        { name = "RESEND_API_KEY", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:RESEND_API_KEY::" },
        { name = "ZOHO_CRM_CLIENT_ID", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:ZOHO_CRM_CLIENT_ID::" },
        { name = "ZOHO_CRM_CLIENT_SECRET", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:ZOHO_CRM_CLIENT_SECRET::" },
        { name = "ZOHO_CRM_REFRESH_TOKEN", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:ZOHO_CRM_REFRESH_TOKEN::" },
        { name = "POSTHOG_API_KEY", valueFrom = "${aws_secretsmanager_secret.app_secrets.arn}:POSTHOG_API_KEY::" }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.worker.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "worker"
        }
      }
    }
  ])
}

# ─────────────────────────────────────────────────────────────
# ECS Services
# ─────────────────────────────────────────────────────────────
resource "aws_ecs_service" "api" {
  name            = "${var.project_name}-${var.environment}-api"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.api_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }

  deployment_controller {
    type = "ECS"
  }

  depends_on = [aws_lb_listener.https]
}

resource "aws_ecs_service" "webhooks" {
  name            = "${var.project_name}-${var.environment}-webhooks"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.webhooks.arn
  desired_count   = var.webhooks_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.webhooks.arn
    container_name   = "webhooks"
    container_port   = 8081
  }

  deployment_controller {
    type = "ECS"
  }

  depends_on = [aws_lb_listener.https]
}

resource "aws_ecs_service" "worker" {
  name            = "${var.project_name}-${var.environment}-worker"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.worker.arn
  desired_count   = var.worker_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  deployment_controller {
    type = "ECS"
  }
}

# ─────────────────────────────────────────────────────────────
# Worker Auto Scaling (1-4 tasks based on CPU utilization)
# ─────────────────────────────────────────────────────────────
resource "aws_appautoscaling_target" "worker" {
  max_capacity       = var.worker_max_capacity
  min_capacity       = var.worker_min_capacity
  resource_id        = "service/${aws_ecs_cluster.main.name}/${aws_ecs_service.worker.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "worker_cpu" {
  name               = "${var.project_name}-${var.environment}-worker-cpu-scaling"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.worker.resource_id
  scalable_dimension = aws_appautoscaling_target.worker.scalable_dimension
  service_namespace  = aws_appautoscaling_target.worker.service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = 70.0
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}
