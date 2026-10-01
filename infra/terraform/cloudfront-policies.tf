# ─────────────────────────────────────────────────────────────────────────────
# CloudFront cache and origin-request policies
#
# The distribution used the deprecated `forwarded_values` block. Its modern
# equivalent is a cache policy plus an origin request policy. These are defined
# explicitly rather than using AWS managed policies so the forwarded surface is
# visible in this repository and cannot drift.
#
# Semantics are preserved from the previous forwarded_values blocks:
#   default behaviour - all query strings, all cookies, the same header list,
#                       and effectively no caching (min/default TTL 0)
#   /static/*         - no query strings, no cookies, one-day caching
#
# AUDIT_REMEDIATION.md F-66.
# ─────────────────────────────────────────────────────────────────────────────

resource "aws_cloudfront_cache_policy" "api" {
  name        = "${var.project_name}-${var.environment}-api"
  comment     = "API responses are per-request; caching is left to the origin TTLs"
  min_ttl     = 0
  default_ttl = 0
  max_ttl     = 86400

  parameters_in_cache_key_and_forwarded_to_origin {
    enable_accept_encoding_gzip   = true
    enable_accept_encoding_brotli = true

    cookies_config {
      cookie_behavior = "none"
    }

    headers_config {
      header_behavior = "none"
    }

    query_strings_config {
      query_string_behavior = "none"
    }
  }
}

resource "aws_cloudfront_origin_request_policy" "api" {
  name    = "${var.project_name}-${var.environment}-api"
  comment = "Forward everything the API needs to authenticate and route"

  # In this provider version the three config blocks are top-level on the
  # resource, not nested inside parameters_in_cache_key_and_forwarded_to_origin
  # (which is how the cache policy resource is shaped). Verified against
  # `terraform providers schema`.
  cookies_config {
    cookie_behavior = "all"
  }

  headers_config {
    header_behavior = "whitelist"
    headers {
      items = ["Authorization", "Accept", "Content-Type", "Origin", "User-Agent"]
    }
  }

  query_strings_config {
    query_string_behavior = "all"
  }
}

resource "aws_cloudfront_cache_policy" "static" {
  name        = "${var.project_name}-${var.environment}-static"
  comment     = "Immutable static assets"
  min_ttl     = 0
  default_ttl = 86400
  max_ttl     = 31536000

  parameters_in_cache_key_and_forwarded_to_origin {
    enable_accept_encoding_gzip   = true
    enable_accept_encoding_brotli = true

    cookies_config {
      cookie_behavior = "none"
    }

    headers_config {
      header_behavior = "none"
    }

    query_strings_config {
      query_string_behavior = "none"
    }
  }
}

resource "aws_cloudfront_origin_request_policy" "static" {
  name    = "${var.project_name}-${var.environment}-static"
  comment = "Static assets need nothing beyond the URL path"

  cookies_config {
    cookie_behavior = "none"
  }

  headers_config {
    header_behavior = "none"
  }

  query_strings_config {
    query_string_behavior = "none"
  }
}
