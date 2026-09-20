---
name: scandrix-sec-infra
description: Container hardening, Dockerfiles, Kubernetes, cloud posture, and network exposure
---

# ScanDrix Infrastructure & Cloud Hardening

### Container & Dockerfile Standards
- **Non-Root Execution**: Explicitly define a non-privileged user: `USER nonroot:nonroot` or `USER 10001`.
- **Minimal Images**: Use `distroless` (e.g., `gcr.io/distroless/static-debian12`) or Alpine. Avoid full OS base images.
- **Read-Only Root Filesystem**: Set `readOnlyRootFilesystem: true` in Kubernetes security contexts.
- **Drop Linux Capabilities**: `securityContext.capabilities.drop: ["ALL"]`.

### Network & Exposure
- **Interface Binding**: Bind microservices strictly to `127.0.0.1` unless external ingress is explicitly required.
- **CORS Configuration**: Never use `Access-Control-Allow-Origin: *` when `Access-Control-Allow-Credentials` is true. Pin exact origin allowlists.
- **Essential Security Headers**:
  - `Content-Security-Policy: default-src 'self'`
  - `Strict-Transport-Security: max-age=63072000; includeSubDomains; preload`
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`
