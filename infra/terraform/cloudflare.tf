# ─────────────────────────────────────────────────────────────
# Cloudflare Free (DNS + CDN + WAF + Tunnel, no R2)
# DB (Supabase) + MQ (CloudAMQP) + storage (Appwrite) stay external free-tier.
# Enable with TF_VAR_cloudflare_api_token + zone/account IDs. No card needed.
# ─────────────────────────────────────────────────────────────
locals {
  cloudflare_enabled = var.enable_cloudflare_tunnel && var.cloudflare_api_token != "" && var.cloudflare_zone_id != "" && var.cloudflare_account_id != ""
}

resource "random_id" "tunnel_secret" {
  count       = local.cloudflare_enabled ? 1 : 0
  byte_length = 32
}

resource "cloudflare_zero_trust_tunnel_cloudflared" "main" {
  count      = local.cloudflare_enabled ? 1 : 0
  account_id = var.cloudflare_account_id
  name       = "${var.project_name}-${var.environment}"
  secret     = random_id.tunnel_secret[0].b64_std
}

# Ingress points at the Fargate task localhost: the cloudflared sidecar in
# ecs.tf shares the task network with the api container (:8080).
# /webhooks* goes to api too (enable_webhooks_service=false merged path).
resource "cloudflare_zero_trust_tunnel_cloudflared_config" "main" {
  count      = local.cloudflare_enabled ? 1 : 0
  account_id = var.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.main[0].id

  config {
    ingress_rule {
      hostname = "api.${var.domain_name}"
      service  = "http://localhost:8080"
    }
    ingress_rule {
      hostname = "webhooks.${var.domain_name}"
      service  = "http://localhost:8080"
    }
    ingress_rule {
      service = "http_status:404"
    }
  }
}

resource "cloudflare_record" "api" {
  count   = local.cloudflare_enabled ? 1 : 0
  zone_id = var.cloudflare_zone_id
  name    = "api"
  type    = "CNAME"
  value   = "${cloudflare_zero_trust_tunnel_cloudflared.main[0].id}.cfargotunnel.com"
  proxied = true
}

resource "cloudflare_record" "webhooks" {
  count   = local.cloudflare_enabled ? 1 : 0
  zone_id = var.cloudflare_zone_id
  name    = "webhooks"
  type    = "CNAME"
  value   = "${cloudflare_zero_trust_tunnel_cloudflared.main[0].id}.cfargotunnel.com"
  proxied = true
}

# WAF + rate limits at edge so Redis-backed limiters only see clean traffic.
resource "cloudflare_ruleset" "edge_guard" {
  count       = local.cloudflare_enabled ? 1 : 0
  zone_id     = var.cloudflare_zone_id
  name        = "${var.project_name}-${var.environment}-edge-guard"
  description = "ScanDrix edge guard: webhook/auth rate limits"
  kind        = "zone"
  phase       = "http_ratelimit"

  rules {
    action = "block"
    ratelimit {
      characteristics     = ["cf.colo.id", "cf.ip.src"]
      period              = 60
      requests_per_period = 100
      mitigation_timeout  = 60
    }
    expression  = "(http.request.uri.path contains \"/webhooks\")"
    description = "webhook ingress 100r/m per IP"
    enabled     = true
  }

  rules {
    action = "block"
    ratelimit {
      characteristics     = ["cf.colo.id", "cf.ip.src"]
      period              = 60
      requests_per_period = 20
      mitigation_timeout  = 300
    }
    expression  = "(http.request.uri.path contains \"/auth\")"
    description = "auth 20r/m per IP"
    enabled     = true
  }
}
