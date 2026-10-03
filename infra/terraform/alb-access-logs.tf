# ─────────────────────────────────────────────────────────────────────────────
# ALB access logs
#
# The ALB had no access_logs block, so there was no record of which requests
# reached it. Combined with ALB access logging disabled at the account level,
# an investigation had nothing to work from. AUDIT_REMEDIATION.md F-66.
#
# NOTE: ALB access logs are delivered by the regional ELB log-delivery account,
# so the bucket policy must name it. That account differs per region. If this
# value is wrong, log delivery fails SILENTLY - the bucket policy denies the
# writer and the ALB reports success. Confirm it against the AWS documentation
# for var.aws_region before relying on these logs.
# ─────────────────────────────────────────────────────────────────────────────
# Resolved rather than hardcoded, so the bucket policy path always matches the
# account actually running it.
data "aws_caller_identity" "current" {}

variable "elb_log_delivery_account_id" {
  type        = string
  description = "Regional ELB log-delivery account id, used by the access-log bucket policy. Wrong values fail silently."
  default     = "718504428378" # ap-south-1
}

variable "alb_log_retention_days" {
  type        = number
  description = "Days to keep ALB access logs before expiry."
  default     = 30
}

resource "aws_s3_bucket" "alb_logs" {
  bucket        = "${var.project_name}-${var.environment}-alb-logs"
  force_destroy = false

  tags = {
    Name = "${var.project_name}-${var.environment}-alb-logs"
  }
}

resource "aws_s3_bucket_public_access_block" "alb_logs" {
  bucket                  = aws_s3_bucket.alb_logs.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "alb_logs" {
  bucket = aws_s3_bucket.alb_logs.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_ownership_controls" "alb_logs" {
  bucket = aws_s3_bucket.alb_logs.id

  rule {
    object_ownership = "BucketOwnerPreferred"
  }
}

# Access logs are high-volume and low-value after a few weeks.
resource "aws_s3_bucket_lifecycle_configuration" "alb_logs" {
  bucket = aws_s3_bucket.alb_logs.id

  rule {
    id     = "expire-alb-access-logs"
    status = "Enabled"

    filter {}

    expiration {
      days = var.alb_log_retention_days
    }

    noncurrent_version_expiration {
      noncurrent_days = 7
    }
  }
}

data "aws_iam_policy_document" "alb_logs" {
  statement {
    sid     = "AllowELBAccountWrite"
    effect  = "Allow"
    actions = ["s3:PutObject"]

    principals {
      type        = "AWS"
      identifiers = [var.elb_log_delivery_account_id]
    }

    resources = ["${aws_s3_bucket.alb_logs.arn}/AWSLogs/${data.aws_caller_identity.current.account_id}/*"]
  }

  statement {
    sid       = "DenyUnencryptedTransport"
    effect    = "Deny"
    actions   = ["s3:*"]
    resources = [aws_s3_bucket.alb_logs.arn, "${aws_s3_bucket.alb_logs.arn}/*"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "alb_logs" {
  bucket = aws_s3_bucket.alb_logs.id
  policy = data.aws_iam_policy_document.alb_logs.json

  depends_on = [aws_s3_bucket_public_access_block.alb_logs]
}
